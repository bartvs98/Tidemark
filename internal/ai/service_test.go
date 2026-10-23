package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/store"

	_ "modernc.org/sqlite"
)

type fakeProvider struct {
	calls   atomic.Int32
	answer  string
	err     error
	usage   Usage
	release chan struct{} // when set, Complete waits for it
	onCall  func()        // when set, runs during the call (e.g. advances the clock)
	mu      sync.Mutex
	last    Prompt // the last prompt sent
}

func (f *fakeProvider) Name() string   { return "anthropic" }
func (f *fakeProvider) Model() string  { return "m1" }
func (f *fakeProvider) Effort() string { return "low" }
func (f *fakeProvider) Complete(ctx context.Context, p Prompt) (json.RawMessage, Usage, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.last = p
	f.mu.Unlock()
	if f.release != nil {
		<-f.release
	}
	if f.onCall != nil {
		f.onCall()
	}
	if f.err != nil {
		return nil, f.usage, f.err
	}
	return json.RawMessage(f.answer), f.usage, nil
}

func newService(t *testing.T) (*Service, *store.Store, *time.Time) {
	s, st, now, _ := newServiceAt(t)
	return s, st, now
}

// newServiceAt also returns the database path, for reading tables the store has no getter for.
func newServiceAt(t *testing.T) (*Service, *store.Store, *time.Time, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := New(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	return s, st, &now, path
}

var (
	key = Key{"github.com/a/b", "1.0.0", "1.2.0"}
	req = Request{Image: "a/b", Repo: "a/b", Running: "1.0.0", Latest: "1.2.0", Releases: []Notes{
		{Tag: "v1.2.0", Body: "- removed foo"}, {Tag: "v1.1.0", Body: "- fix"}}}
	good = `{"changes":["Foo is removed"],"actions":["Drop foo from config"],"verdict":"breaking","reasons":[{"text":"foo removed","tag":"v1.2.0"}]}`
)

func TestGenerateStoresAndLoads(t *testing.T) {
	s, st, now := newService(t)
	ctx := context.Background()
	if _, err := s.Generate(ctx, key, req, false); !errors.Is(err, ErrOff) {
		t.Errorf("off: %v", err)
	}
	f := &fakeProvider{answer: good}
	s.Configure(f, 0)
	if _, err := s.Generate(ctx, key, Request{Latest: "1", Releases: []Notes{{Tag: "v1", Body: " "}}}, false); !errors.Is(err, ErrNoNotes) {
		t.Errorf("no notes: %v", err)
	}
	if f.calls.Load() != 0 {
		t.Fatal("provider called without notes")
	}

	e, err := s.Generate(ctx, key, req, false)
	if err != nil {
		t.Fatal(err)
	}
	sum := e.Summary
	if sum == nil || sum.Verdict != analysis.Breaking || sum.Provider != "anthropic" || sum.Model != "m1" ||
		sum.Releases != 2 || sum.Omitted != 0 || sum.NotesHash != NotesHash(req) || !sum.CreatedAt.Equal(*now) ||
		len(sum.Reasons) != 1 || sum.Reasons[0].Tag != "v1.2.0" || sum.Actions[0] != "Drop foo from config" {
		t.Fatalf("summary = %+v", sum)
	}
	if got, ok := s.Lookup(key); !ok || got.Summary != sum {
		t.Error("lookup after generate")
	}

	// A failure keeps the summary and records the error; it survives a reload.
	*now = now.Add(time.Hour)
	f.err = errors.New("rate limited by anthropic")
	e, err = s.Generate(ctx, key, req, false)
	if err == nil || err.Error() != "rate limited by anthropic" || e.Summary == nil || e.Error != err.Error() || !e.FailedAt.Equal(*now) {
		t.Errorf("failure: %+v %v", e, err)
	}
	s2 := New(st, s.log)
	if err := s2.Load(ctx); err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Lookup(key)
	if !ok || got.Summary == nil || got.Summary.Verdict != analysis.Breaking || got.Summary.Reasons[0] != (Reason{"foo removed", "v1.2.0"}) ||
		got.Error != "rate limited by anthropic" || !got.FailedAt.Equal(*now) || got.Summary.NotesHash != NotesHash(req) {
		t.Errorf("reloaded = %+v %+v", got, got.Summary)
	}

	// Success clears the error.
	f.err = nil
	e, _ = s.Generate(ctx, key, req, false)
	if e.Error != "" || !e.FailedAt.IsZero() {
		t.Errorf("error not cleared: %+v", e)
	}

	// An invalid answer is a failure.
	f.answer = `{"changes":[],"actions":[],"verdict":"maybe","reasons":[]}`
	if _, err := s.Generate(ctx, Key{"r", "1", "2"}, req, false); err == nil {
		t.Error("invalid answer accepted")
	}
	if e, _ := s.Lookup(Key{"r", "1", "2"}); e.Summary != nil || e.Error == "" {
		t.Errorf("invalid answer entry: %+v", e)
	}
}

func TestGenerateSharesConcurrentCalls(t *testing.T) {
	s, _, _ := newService(t)
	f := &fakeProvider{answer: good, release: make(chan struct{})}
	s.Configure(f, 0)
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			if _, err := s.Generate(context.Background(), key, req, false); err != nil {
				t.Error(err)
			}
		})
	}
	deadline := time.Now().Add(5 * time.Second)
	for !s.Generating(key) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !s.Generating(key) {
		t.Fatal("not generating")
	}
	time.Sleep(20 * time.Millisecond) // let the other callers join
	close(f.release)
	wg.Wait()
	if n := f.calls.Load(); n != 1 {
		t.Errorf("provider calls = %d, want 1", n)
	}
	if s.Generating(key) {
		t.Error("still generating")
	}
}

func TestAuto(t *testing.T) {
	s, _, now := newService(t)
	ctx := context.Background()
	f := &fakeProvider{answer: good}

	s.Auto(ctx, key, req) // off
	s.Configure(f, 0)
	s.Auto(ctx, key, req) // auto off
	s.Configure(f, 3)
	s.Auto(ctx, key, req) // 2 releases < 3
	if f.calls.Load() != 0 {
		t.Fatal("auto ran below its conditions")
	}

	s.Configure(f, 2)
	s.Auto(ctx, key, req)
	s.Auto(ctx, key, req) // fresh summary: no new call
	if f.calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", f.calls.Load())
	}

	// The notes changed: stale, regenerated.
	stale := req
	stale.Releases = append([]Notes(nil), req.Releases...)
	stale.Releases[0].Body += "\n- more"
	f.err = errors.New("anthropic error (status 500)")
	s.Auto(ctx, key, stale) // fails
	if f.calls.Load() != 2 {
		t.Fatalf("stale: calls = %d, want 2", f.calls.Load())
	}
	if e, _ := s.Lookup(key); e.Summary == nil || e.Summary.NotesHash != NotesHash(req) || e.Error == "" {
		t.Errorf("after failure: %+v", e)
	}
	// Back-off: no retry within 6 h, one after.
	*now = now.Add(5 * time.Hour)
	s.Auto(ctx, key, stale)
	if f.calls.Load() != 2 {
		t.Error("retried within the back-off")
	}
	f.err = nil
	*now = now.Add(2 * time.Hour)
	s.Auto(ctx, key, stale)
	if f.calls.Load() != 3 {
		t.Error("no retry after the back-off")
	}
	if e, _ := s.Lookup(key); e.Summary == nil || e.Summary.NotesHash != NotesHash(stale) || e.Error != "" {
		t.Errorf("after retry: %+v", e)
	}
}

func TestStatusAndConfigure(t *testing.T) {
	s, _, _ := newService(t)
	if p, n := s.Status(); p != nil || n != 0 {
		t.Error("new service not off")
	}
	f := &fakeProvider{}
	s.Configure(f, 5)
	if p, n := s.Status(); p != f || n != 5 {
		t.Error("configure")
	}
	s.Configure(nil, 0)
	if p, _ := s.Status(); p != nil {
		t.Error("configure off")
	}
}

// aiCalls reads the usage records straight from the database.
func aiCalls(t *testing.T, path string) []store.AICall {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT at, provider, model, trigger, mode, repo, from_version, to_version, releases,
		input_tokens, output_tokens, duration_ms, error FROM ai_calls ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []store.AICall
	for rows.Next() {
		var c store.AICall
		var at string
		var ms int64
		if err := rows.Scan(&at, &c.Provider, &c.Model, &c.Trigger, &c.Mode, &c.Repo, &c.From, &c.To, &c.Releases,
			&c.InputTokens, &c.OutputTokens, &ms, &c.Error); err != nil {
			t.Fatal(err)
		}
		c.At, _ = time.Parse(time.RFC3339Nano, at)
		c.Duration = time.Duration(ms) * time.Millisecond
		out = append(out, c)
	}
	return out
}

func TestCallRecordsAndUsage(t *testing.T) {
	s, st, now, path := newServiceAt(t)
	ctx := context.Background()
	start := *now
	f := &fakeProvider{answer: good, usage: Usage{InputTokens: 1500, OutputTokens: 200},
		onCall: func() { *now = now.Add(4200 * time.Millisecond) }}

	// No call, no record: off, or nothing to summarise.
	s.Generate(ctx, key, req, false)
	s.Configure(f, 2)
	s.Generate(ctx, key, Request{Latest: "1", Releases: []Notes{{Tag: "v1"}}}, false)
	if got := aiCalls(t, path); len(got) != 0 {
		t.Fatalf("records without a call: %+v", got)
	}

	e, err := s.Generate(ctx, key, req, false)
	if err != nil {
		t.Fatal(err)
	}
	if sm := e.Summary; sm.InputTokens != 1500 || sm.OutputTokens != 200 || sm.Duration != 4200*time.Millisecond {
		t.Errorf("summary usage: %+v", sm)
	}
	s2 := New(st, s.log)
	s2.Load(ctx)
	if got, _ := s2.Lookup(key); got.Summary == nil || got.Summary.InputTokens != 1500 || got.Summary.Duration != 4200*time.Millisecond {
		t.Errorf("reloaded usage: %+v", got.Summary)
	}

	// A failed automatic call is recorded too, with what the provider reported.
	f.err, f.usage = errors.New("the answer was cut off"), Usage{InputTokens: 1500, OutputTokens: 4096}
	k2 := Key{"github.com/a/b", "1.0.0", "1.3.0"}
	s.Auto(ctx, k2, req)

	got := aiCalls(t, path)
	want := []store.AICall{
		{At: start, Provider: "anthropic", Model: "m1", Trigger: "manual", Mode: "full", Repo: key.Repo, From: key.From, To: key.To,
			Releases: 2, InputTokens: 1500, OutputTokens: 200, Duration: 4200 * time.Millisecond},
		{At: start.Add(4200 * time.Millisecond), Provider: "anthropic", Model: "m1", Trigger: "auto", Mode: "full", Repo: k2.Repo, From: k2.From, To: k2.To,
			Releases: 2, InputTokens: 1500, OutputTokens: 4096, Duration: 4200 * time.Millisecond, Error: "the answer was cut off"},
	}
	if len(got) != len(want) {
		t.Fatalf("records = %+v", got)
	}
	for i := range want {
		if !got[i].At.Equal(want[i].At) {
			t.Errorf("record %d at %v, want %v", i, got[i].At, want[i].At)
		}
		got[i].At = want[i].At
		if got[i] != want[i] {
			t.Errorf("record %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}

	u, err := s.Usage(ctx, start)
	if err != nil || u != (store.AIUsage{Calls: 2, Failed: 1, InputTokens: 3000, OutputTokens: 4296}) {
		t.Errorf("usage = %+v, %v", u, err)
	}
}

func TestPrune(t *testing.T) {
	s, st, _ := newService(t)
	ctx := context.Background()
	f := &fakeProvider{answer: good}
	s.Configure(f, 0)
	keep, drop, busy := Key{"r", "1", "2"}, Key{"r", "0", "3"}, Key{"r", "2", "4"} // other running versions: no bases of keep
	for _, k := range []Key{keep, drop, busy} {
		if _, err := s.Generate(ctx, k, req, false); err != nil {
			t.Fatal(err)
		}
	}
	s.setGenerating(busy, true) // a regeneration is running

	n, err := s.Prune(ctx, map[Key]bool{keep: true})
	if err != nil || n != 1 {
		t.Fatalf("pruned %d, %v", n, err)
	}
	for k, want := range map[Key]bool{keep: true, drop: false, busy: true} {
		if _, ok := s.Lookup(k); ok != want {
			t.Errorf("memory %v: present %v, want %v", k, ok, want)
		}
	}
	rows, _ := st.AISummaries(ctx)
	if len(rows) != 2 || rows[0].To != "2" || rows[1].To != "4" {
		t.Errorf("store: %+v", rows)
	}

	s.setGenerating(busy, false)
	if n, err := s.Prune(ctx, nil); err != nil || n != 2 {
		t.Errorf("prune all: %d %v", n, err)
	}
	if rows, _ := st.AISummaries(ctx); len(rows) != 0 {
		t.Errorf("left in store: %+v", rows)
	}
	if _, ok := s.Lookup(keep); ok {
		t.Error("left in memory")
	}
}

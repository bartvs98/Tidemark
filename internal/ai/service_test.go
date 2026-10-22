package ai

import (
	"context"
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
)

type fakeProvider struct {
	calls   atomic.Int32
	answer  string
	err     error
	release chan struct{} // when set, Complete waits for it
}

func (f *fakeProvider) Name() string  { return "anthropic" }
func (f *fakeProvider) Model() string { return "m1" }
func (f *fakeProvider) Complete(ctx context.Context, p Prompt) (json.RawMessage, error) {
	f.calls.Add(1)
	if f.release != nil {
		<-f.release
	}
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(f.answer), nil
}

func newService(t *testing.T) (*Service, *store.Store, *time.Time) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := New(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	return s, st, &now
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
	if _, err := s.Generate(ctx, key, req); !errors.Is(err, ErrOff) {
		t.Errorf("off: %v", err)
	}
	f := &fakeProvider{answer: good}
	s.Configure(f, 0)
	if _, err := s.Generate(ctx, key, Request{Latest: "1", Releases: []Notes{{Tag: "v1", Body: " "}}}); !errors.Is(err, ErrNoNotes) {
		t.Errorf("no notes: %v", err)
	}
	if f.calls.Load() != 0 {
		t.Fatal("provider called without notes")
	}

	e, err := s.Generate(ctx, key, req)
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
	e, err = s.Generate(ctx, key, req)
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
	e, _ = s.Generate(ctx, key, req)
	if e.Error != "" || !e.FailedAt.IsZero() {
		t.Errorf("error not cleared: %+v", e)
	}

	// An invalid answer is a failure.
	f.answer = `{"changes":[],"actions":[],"verdict":"maybe","reasons":[]}`
	if _, err := s.Generate(ctx, Key{"r", "1", "2"}, req); err == nil {
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
			if _, err := s.Generate(context.Background(), key, req); err != nil {
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

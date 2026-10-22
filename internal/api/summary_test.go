package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bartvs98/tidemark/internal/ai"
	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/store"
)

// fakeProvider answers every completion with out, or fails with err.
type fakeProvider struct {
	mu    sync.Mutex
	out   string
	err   error
	usage ai.Usage
	calls int
}

func (p *fakeProvider) Name() string  { return "anthropic" }
func (p *fakeProvider) Model() string { return "claude-test" }
func (p *fakeProvider) Complete(context.Context, ai.Prompt) (json.RawMessage, ai.Usage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.err != nil {
		return nil, p.usage, p.err
	}
	return json.RawMessage(p.out), p.usage, nil
}

func (p *fakeProvider) answer(out string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.out, p.err = out, err
}

type summaryFixture struct {
	h   http.Handler
	srv *Server
	svc *ai.Service
	db  *store.Store
	p   *fakeProvider
}

// newSummaryFixture serves the fixture snapshot after one check run (jellyfin: 10.9.11 →
// 10.10.7) with a fake GitHub whose newest release has notes newest, and AI on.
func newSummaryFixture(t *testing.T, newest string) summaryFixture {
	t.Helper()
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/repos/jellyfin/jellyfin-packaging/releases") {
			http.NotFound(w, r)
			return
		}
		body, _ := json.Marshal(newest)
		_, _ = io.WriteString(w, `[
			{"tag_name":"v10.10.7","name":"10.10.7","body":`+string(body)+`,"html_url":"u","published_at":"2026-10-01T00:00:00Z"},
			{"tag_name":"v10.10.0","name":"10.10.0","body":"big one","html_url":"u","published_at":"2026-09-01T00:00:00Z"},
			{"tag_name":"v10.9.11","name":"10.9.11","body":"old","html_url":"u","published_at":"2026-08-01T00:00:00Z"}
		]`)
	}))
	t.Cleanup(gh.Close)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	src := docker.NewSnapshot("../docker/testdata/snapshot.json")
	checks := check.New(src, tagsRegistry{}, db, log)
	svc := ai.New(db, log)
	if err := svc.Load(t.Context()); err != nil {
		t.Fatal(err)
	}
	p := &fakeProvider{}
	svc.Configure(p, 0)
	srv := New(Deps{
		Version: "t", Log: log, Source: src, Checks: checks, Store: db, AI: svc,
		Changelogs: changelog.New(db, "", log, changelog.WithBaseURLs(gh.URL, gh.URL, gh.URL)),
		Background: t.Context(),
	})
	h := srv.Handler(fstest.MapFS{})

	checks.Start(t.Context(), check.TriggerManual)
	for deadline := time.Now().Add(5 * time.Second); checks.Status().Running && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	return summaryFixture{h: h, srv: srv, svc: svc, db: db, p: p}
}

func (f summaryFixture) detail(t *testing.T) detailJSON {
	t.Helper()
	rec := do(t, f.h, "/api/containers/jellyfin")
	if rec.Code != http.StatusOK {
		t.Fatalf("detail = %d: %s", rec.Code, rec.Body)
	}
	var d detailJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func aiReasons(rs []analysis.Reason) []analysis.Reason {
	var out []analysis.Reason
	for _, r := range rs {
		if r.Source == "ai" {
			out = append(out, r)
		}
	}
	return out
}

func TestSummaryLifecycle(t *testing.T) {
	f := newSummaryFixture(t, "fixes")

	// The detail request stores the notes; nothing summarised yet.
	d := f.detail(t)
	if d.AI == nil || d.AI.Provider != "anthropic" || d.AI.Model != "claude-test" || d.AI.Releases != 2 ||
		d.AI.Summary != nil || d.AI.Generating || d.AI.Error != "" {
		t.Fatalf("ai before = %+v", d.AI)
	}
	if u := d.Container.Update; u.Level != analysis.None {
		t.Errorf("heuristic level = %v", u.Level)
	}

	f.p.answer(`{"changes":["New player"],"actions":[],"verdict":"attention","reasons":[{"text":"Config moved","tag":"v10.10.7"}]}`, nil)
	f.p.usage = ai.Usage{InputTokens: 12300, OutputTokens: 640}
	rec := send(t, f.h, http.MethodPost, "/api/containers/jellyfin/summary", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("POST = %d: %s", rec.Code, rec.Body)
	}
	var state aiJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	sm := state.Summary
	if sm == nil || sm.Verdict != analysis.Attention || len(sm.Changes) != 1 || sm.Actions == nil || len(sm.Actions) != 0 ||
		len(sm.Reasons) != 1 || sm.Reasons[0].Tag != "v10.10.7" || sm.Releases != 2 || sm.Stale || sm.CreatedAt.IsZero() {
		t.Errorf("state = %s", rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"actions":[]`) {
		t.Errorf("actions not an array: %s", rec.Body)
	}
	if sm != nil && (sm.InputTokens != 12300 || sm.OutputTokens != 640 || sm.DurationMs < 0) ||
		!strings.Contains(rec.Body.String(), `"inputTokens":12300,"outputTokens":640,"durationMs":`) {
		t.Errorf("usage fields: %s", rec.Body)
	}
	if a := f.detail(t).AI; a == nil || a.Summary == nil || a.Summary.InputTokens != 12300 || a.Summary.OutputTokens != 640 {
		t.Errorf("detail usage: %+v", a)
	}
	if u, err := f.svc.Usage(t.Context(), time.Time{}); err != nil || u != (store.AIUsage{Calls: 1, InputTokens: 12300, OutputTokens: 640}) {
		t.Errorf("recorded usage = %+v, %v", u, err)
	}

	// The verdict raises the level, on the list and the detail alike, with the AI's reasons last.
	for name, u := range map[string]updateJSON{"list": listUpdate(t, f.h, "jellyfin"), "detail": *f.detail(t).Container.Update} {
		ar := aiReasons(u.Reasons)
		if u.Level != analysis.Attention || len(ar) != 1 || ar[0] != (analysis.Reason{Text: "Config moved", Tag: "v10.10.7", Source: "ai"}) ||
			u.Reasons[len(u.Reasons)-1].Source != "ai" {
			t.Errorf("%s: level %v reasons %+v", name, u.Level, u.Reasons)
		}
	}

	// A failed attempt keeps the summary and reports the (scrubbed) error.
	f.p.answer("", errors.New("rate limited by anthropic"))
	rec = send(t, f.h, http.MethodPost, "/api/containers/jellyfin/summary", "")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "rate limited by anthropic") {
		t.Errorf("failing POST = %d: %s", rec.Code, rec.Body)
	}
	if a := f.detail(t).AI; a == nil || a.Summary == nil || a.Error == "" || a.FailedAt == nil {
		t.Errorf("after failure = %+v", a)
	}

	// AI off: no block, verdicts no longer apply, the button is refused.
	f.svc.Configure(nil, 0)
	if d := f.detail(t); d.AI != nil || d.Container.Update.Level != analysis.None || aiReasons(d.Container.Update.Reasons) != nil {
		t.Errorf("off: ai %+v, update %+v", d.AI, d.Container.Update)
	}
	if u := listUpdate(t, f.h, "jellyfin"); u.Level != analysis.None {
		t.Errorf("off: list level %v", u.Level)
	}
	if rec := send(t, f.h, http.MethodPost, "/api/containers/jellyfin/summary", ""); rec.Code != http.StatusConflict {
		t.Errorf("off POST = %d: %s", rec.Code, rec.Body)
	}
}

// The AI verdict can only raise the heuristic level, never lower it.
func TestSummaryNeverLowers(t *testing.T) {
	f := newSummaryFixture(t, "## Breaking\nnew config")
	f.detail(t) // stores the notes
	f.p.answer(`{"changes":["x"],"actions":[],"verdict":"none","reasons":[]}`, nil)
	if rec := send(t, f.h, http.MethodPost, "/api/containers/jellyfin/summary", ""); rec.Code != http.StatusOK {
		t.Fatalf("POST = %d: %s", rec.Code, rec.Body)
	}
	d := f.detail(t)
	if u := d.Container.Update; u.Level != analysis.Breaking || aiReasons(u.Reasons) != nil {
		t.Errorf("update = %+v", u)
	}
	if d.AI == nil || d.AI.Summary == nil || d.AI.Summary.Verdict != analysis.None {
		t.Errorf("ai = %+v", d.AI)
	}
}

func TestSummaryErrors(t *testing.T) {
	f := newSummaryFixture(t, "fixes")
	tests := []struct {
		name string
		code int
	}{
		{"ghost", http.StatusNotFound},
		{"orphan", http.StatusUnprocessableEntity},   // no update to summarise
		{"jellyfin", http.StatusUnprocessableEntity}, // notes not stored yet: nothing in range
	}
	for _, tt := range tests {
		if rec := send(t, f.h, http.MethodPost, "/api/containers/"+tt.name+"/summary", ""); rec.Code != tt.code {
			t.Errorf("%s: %d %s", tt.name, rec.Code, rec.Body)
		}
	}
	if f.p.calls != 0 {
		t.Errorf("provider called %d times", f.p.calls)
	}

	// Writes need the custom header.
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/containers/jellyfin/summary", nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("no header: %d", rec.Code)
	}

	// Without an AI service.
	h := New(Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Source: docker.NewSnapshot("../docker/testdata/snapshot.json")}).Handler(fstest.MapFS{})
	if rec := send(t, h, http.MethodPost, "/api/containers/jellyfin/summary", ""); rec.Code != http.StatusNotImplemented {
		t.Errorf("no service: %d", rec.Code)
	}
}

// AIKeys names exactly the summaries the list applies, so pruning to it keeps them all.
func TestAIKeysAndPrune(t *testing.T) {
	f := newSummaryFixture(t, "fixes")
	f.detail(t) // stores the notes
	f.p.answer(`{"changes":["x"],"actions":[],"verdict":"breaking","reasons":[{"text":"From AI","tag":""}]}`, nil)
	if rec := send(t, f.h, http.MethodPost, "/api/containers/jellyfin/summary", ""); rec.Code != http.StatusOK {
		t.Fatalf("POST = %d: %s", rec.Code, rec.Body)
	}
	rows, _ := f.db.AISummaries(t.Context())
	if len(rows) != 1 {
		t.Fatalf("stored = %+v", rows)
	}
	jellyfin := ai.Key{Repo: rows[0].Repo, From: rows[0].From, To: rows[0].To}

	keys, err := f.srv.AIKeys(t.Context())
	if err != nil || !keys[jellyfin] {
		t.Fatalf("keys = %v, %v (want %v in it)", keys, err, jellyfin)
	}
	// A summary for every key, plus one nothing needs: each key's summary shows on the list.
	notes := ai.Request{Latest: "x", Releases: []ai.Notes{{Tag: "v", Body: "notes"}}}
	for k := range keys {
		if _, err := f.svc.Generate(t.Context(), k, notes, false); err != nil {
			t.Fatal(err)
		}
	}
	orphan := ai.Key{Repo: jellyfin.Repo, From: "1.0.0", To: "2.0.0"}
	if _, err := f.svc.Generate(t.Context(), orphan, notes, false); err != nil {
		t.Fatal(err)
	}
	applied := func() int {
		var body containerList
		if err := json.Unmarshal(do(t, f.h, "/api/containers").Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, c := range body.Containers {
			if c.Update != nil && aiReasons(c.Update.Reasons) != nil {
				n++
			}
		}
		return n
	}
	before := applied()
	if before < len(keys) {
		t.Errorf("%d containers show an AI verdict, want at least one per key (%d)", before, len(keys))
	}

	n, err := f.svc.Prune(t.Context(), keys)
	if err != nil || n != 1 {
		t.Errorf("pruned %d, %v; want only the orphan", n, err)
	}
	if _, ok := f.svc.Lookup(orphan); ok {
		t.Error("orphan still in memory")
	}
	if got := applied(); got != before {
		t.Errorf("after prune %d containers show an AI verdict, want %d", got, before)
	}

	// Docker failing is an error, not an empty set (which would prune everything).
	broken := New(Deps{Log: f.srv.log, Source: docker.NewSnapshot("missing.json"), AI: f.svc})
	if _, err := broken.AIKeys(t.Context()); err == nil {
		t.Error("AIKeys without Docker: no error")
	}
}

// The POST body picks the mode: empty or {} extends an earlier summary when possible,
// {"mode":"full"} (Regenerate) always summarises the whole range.
func TestSummaryModes(t *testing.T) {
	f := newSummaryFixture(t, "fixes")
	f.detail(t) // stores the notes
	f.p.answer(`{"changes":["x"],"actions":[],"verdict":"none","reasons":[]}`, nil)

	for _, body := range []string{`{"mode":"nope"}`, `{"mode":1}`, `[]`, `nonsense`, `{"mod":"full"}`} {
		if rec := send(t, f.h, http.MethodPost, "/api/containers/jellyfin/summary", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
	if f.p.calls != 0 {
		t.Fatalf("provider called %d times for bad bodies", f.p.calls)
	}

	if rec := send(t, f.h, http.MethodPost, "/api/containers/jellyfin/summary", `{"mode":"full"}`); rec.Code != http.StatusOK ||
		strings.Contains(rec.Body.String(), "extendedFrom") {
		t.Fatalf("full POST = %d: %s", rec.Code, rec.Body)
	}
	rows, _ := f.db.AISummaries(t.Context())
	if len(rows) != 1 {
		t.Fatalf("stored = %+v", rows)
	}
	// An earlier summary up to 10.10.0, with the notes as stored: the next one extends it.
	base := ai.Key{Repo: rows[0].Repo, From: rows[0].From, To: "10.10.0"}
	f.p.answer(`{"changes":["big"],"actions":[],"verdict":"attention","reasons":[{"text":"db migration","tag":"v10.10.0"}]}`, nil)
	if _, err := f.svc.Generate(t.Context(), base, ai.Request{Latest: "10.10.0",
		Releases: []ai.Notes{{Tag: "v10.10.0", Body: "big one"}}}, true); err != nil {
		t.Fatal(err)
	}

	f.p.answer(`{"changes":["fixes"],"actions":[],"verdict":"none","reasons":[]}`, nil)
	for _, body := range []string{"", "{}"} {
		rec := send(t, f.h, http.MethodPost, "/api/containers/jellyfin/summary", body)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"extendedFrom":"10.10.0"`) {
			t.Errorf("%q POST = %d: %s", body, rec.Code, rec.Body)
		}
	}
	d := f.detail(t)
	if d.AI == nil || d.AI.Summary == nil || d.AI.Summary.ExtendedFrom != "10.10.0" || d.AI.Summary.Verdict != analysis.Attention ||
		d.AI.Summary.Releases != 2 || d.AI.Summary.Stale {
		t.Errorf("detail ai = %+v", d.AI)
	}
	if u := d.Container.Update; u.Level != analysis.Attention || len(aiReasons(u.Reasons)) != 1 {
		t.Errorf("update = %+v", u)
	}

	if rec := send(t, f.h, http.MethodPost, "/api/containers/jellyfin/summary", `{"mode":"full"}`); rec.Code != http.StatusOK ||
		strings.Contains(rec.Body.String(), "extendedFrom") {
		t.Errorf("regenerate = %d: %s", rec.Code, rec.Body)
	}
	if u, _ := f.svc.Usage(t.Context(), time.Time{}); u.Calls != 5 || u.Extended != 2 {
		t.Errorf("usage = %+v", u)
	}
}

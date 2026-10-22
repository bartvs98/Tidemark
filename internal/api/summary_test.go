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
	calls int
}

func (p *fakeProvider) Name() string  { return "anthropic" }
func (p *fakeProvider) Model() string { return "claude-test" }
func (p *fakeProvider) Complete(context.Context, ai.Prompt) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	return json.RawMessage(p.out), nil
}

func (p *fakeProvider) answer(out string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.out, p.err = out, err
}

type summaryFixture struct {
	h   http.Handler
	svc *ai.Service
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
	h := New(Deps{
		Version: "t", Log: log, Source: src, Checks: checks, Store: db, AI: svc,
		Changelogs: changelog.New(db, "", log, changelog.WithBaseURLs(gh.URL, gh.URL, gh.URL)),
		Background: t.Context(),
	}).Handler(fstest.MapFS{})

	checks.Start(t.Context(), check.TriggerManual)
	for deadline := time.Now().Add(5 * time.Second); checks.Status().Running && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	return summaryFixture{h: h, svc: svc, p: p}
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

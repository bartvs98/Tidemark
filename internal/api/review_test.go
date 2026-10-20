package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/store"
)

func send(t *testing.T, h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("X-Tidemark", "1")
	h.ServeHTTP(rec, req)
	return rec
}

// reviewHandler serves the fixture snapshot with one finished check run (jellyfin:
// 10.9.11 → 10.10.7, orphan: image missing) and a store.
func reviewHandler(t *testing.T) http.Handler {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	src := docker.NewSnapshot("../docker/testdata/snapshot.json")
	checks := check.New(src, tagsRegistry{}, db, log)
	checks.Start(t.Context(), check.TriggerManual)
	for deadline := time.Now().Add(5 * time.Second); checks.Status().Running && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	return New(Deps{Version: "t", Log: log, Source: src, Checks: checks, Store: db}).Handler(fstest.MapFS{})
}

// listReviews returns each container's review from the list endpoint (nil when absent).
func listReviews(t *testing.T, h http.Handler) map[string]*reviewJSON {
	t.Helper()
	var body containerList
	if err := json.Unmarshal(do(t, h, "/api/containers").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	out := map[string]*reviewJSON{}
	for _, c := range body.Containers {
		out[c.Name] = c.Review
	}
	return out
}

func TestReviewLifecycle(t *testing.T) {
	h := reviewHandler(t)
	if got := listReviews(t, h); got["jellyfin"] != nil {
		t.Fatalf("review before any was set: %+v", got["jellyfin"])
	}

	// Reviewed up to the latest version: shown, as stored.
	rec := send(t, h, http.MethodPut, "/api/containers/jellyfin/review", `{"kind":"reviewed","version":"10.10.7"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT reviewed = %d: %s", rec.Code, rec.Body)
	}
	var stored reviewJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Kind != "reviewed" || stored.Version != "10.10.7" || stored.Until != nil || stored.At.IsZero() {
		t.Errorf("stored = %+v", stored)
	}
	if r := listReviews(t, h)["jellyfin"]; r == nil || r.Kind != "reviewed" || r.Version != "10.10.7" || !r.At.Equal(stored.At) {
		t.Errorf("listed = %+v", r)
	}
	var d detailJSON
	if err := json.Unmarshal(do(t, h, "/api/containers/jellyfin").Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.Container.Review == nil || d.Container.Review.Kind != "reviewed" {
		t.Errorf("detail review = %+v", d.Container.Review)
	}

	// Reviewed up to an older version: a newer latest ends it.
	send(t, h, http.MethodPut, "/api/containers/jellyfin/review", `{"kind":"reviewed","version":"10.10.6"}`)
	if r := listReviews(t, h)["jellyfin"]; r != nil {
		t.Errorf("outdated review shown: %+v", r)
	}

	// Snoozed by time replaces it.
	until := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	rec = send(t, h, http.MethodPut, "/api/containers/jellyfin/review", `{"kind":"snoozed","until":"`+until.Format(time.RFC3339)+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT snoozed = %d: %s", rec.Code, rec.Body)
	}
	if r := listReviews(t, h)["jellyfin"]; r == nil || r.Kind != "snoozed" || r.Version != "" || r.Until == nil || !r.Until.Equal(until) {
		t.Errorf("snoozed by time = %+v", r)
	}
	// Snoozed until a newer version.
	send(t, h, http.MethodPut, "/api/containers/jellyfin/review", `{"kind":"snoozed","version":"10.10.7"}`)
	if r := listReviews(t, h)["jellyfin"]; r == nil || r.Kind != "snoozed" || r.Version != "10.10.7" || r.Until != nil {
		t.Errorf("snoozed by version = %+v", r)
	}

	// A container without a successful result (image missing) shows no review.
	if rec := send(t, h, http.MethodPut, "/api/containers/orphan/review", `{"kind":"snoozed","version":"1.0"}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT orphan = %d", rec.Code)
	}
	if r := listReviews(t, h)["orphan"]; r != nil {
		t.Errorf("orphan review shown without a result: %+v", r)
	}

	// Undo, twice: both 204.
	for range 2 {
		if rec := send(t, h, http.MethodDelete, "/api/containers/jellyfin/review", ""); rec.Code != http.StatusNoContent {
			t.Errorf("DELETE = %d: %s", rec.Code, rec.Body)
		}
	}
	if r := listReviews(t, h)["jellyfin"]; r != nil {
		t.Errorf("review after delete: %+v", r)
	}
}

func TestReviewRequests(t *testing.T) {
	h := reviewHandler(t)
	past := time.Now().Add(-time.Hour).Format(time.RFC3339)
	future := time.Now().Add(time.Hour).Format(time.RFC3339)
	bad := []string{
		``,
		`not json`,
		`{"kind":"later"}`,
		`{"kind":"reviewed"}`,
		`{"kind":"reviewed","version":"  "}`,
		`{"kind":"reviewed","version":"1.0","until":"` + future + `"}`,
		`{"kind":"snoozed"}`,
		`{"kind":"snoozed","version":"1.0","until":"` + future + `"}`,
		`{"kind":"snoozed","until":"` + past + `"}`,
		`{"kind":"snoozed","until":"tomorrow"}`,
		`{"kind":"reviewed","version":"1.0","extra":true}`,
		`{"kind":"reviewed","version":"1.0"} {}`,
	}
	for _, body := range bad {
		rec := send(t, h, http.MethodPut, "/api/containers/jellyfin/review", body)
		var e map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		if rec.Code != http.StatusBadRequest || e["error"] == "" {
			t.Errorf("%s: got %d %v, want 400 with an error", body, rec.Code, e)
		}
	}

	for _, rec := range []*httptest.ResponseRecorder{
		send(t, h, http.MethodPut, "/api/containers/nope/review", `{"kind":"reviewed","version":"1.0"}`),
		send(t, h, http.MethodDelete, "/api/containers/nope/review", ""),
	} {
		var e map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		if rec.Code != http.StatusNotFound || e["error"] == "" {
			t.Errorf("unknown container: got %d %v", rec.Code, e)
		}
	}

	// Docker unreachable.
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	down := New(Deps{Log: log, Source: failingSource{}, Store: db}).Handler(fstest.MapFS{})
	if rec := send(t, down, http.MethodPut, "/api/containers/jellyfin/review", `{"kind":"reviewed","version":"1.0"}`); rec.Code != http.StatusBadGateway {
		t.Errorf("Docker down: PUT = %d", rec.Code)
	}
}

func TestReviewsWithoutStore(t *testing.T) {
	h := handler(builtSPA)
	if rec := send(t, h, http.MethodPut, "/api/containers/jellyfin/review", `{"kind":"reviewed","version":"1.0"}`); rec.Code != http.StatusNotImplemented {
		t.Errorf("PUT = %d", rec.Code)
	}
	if rec := send(t, h, http.MethodDelete, "/api/containers/jellyfin/review", ""); rec.Code != http.StatusNotImplemented {
		t.Errorf("DELETE = %d", rec.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(do(t, h, "/api/containers/jellyfin").Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["history"]) != "[]" {
		t.Errorf("history = %s, want []", raw["history"])
	}
}

func TestDetailHistory(t *testing.T) {
	h := reviewHandler(t)
	var raw struct {
		History []map[string]any `json:"history"`
	}
	if err := json.Unmarshal(do(t, h, "/api/containers/jellyfin").Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	// The first check: tracking started, so no prev fields.
	if len(raw.History) != 1 {
		t.Fatalf("history = %v", raw.History)
	}
	row := raw.History[0]
	if row["imageId"] != "sha256:aaa" || row["running"] != "10.9.11" || row["at"] == nil || row["prevImageId"] != nil || row["prevRunning"] != nil {
		t.Errorf("first row = %v", row)
	}
	// A container whose checks failed has none.
	var orphan detailJSON
	if err := json.Unmarshal(do(t, h, "/api/containers/orphan").Body.Bytes(), &orphan); err != nil {
		t.Fatal(err)
	}
	if orphan.History == nil || len(orphan.History) != 0 {
		t.Errorf("orphan history = %v", orphan.History)
	}
}

// A channel tag or an unknown latest ties a review to the build (the tag's digest); a
// version review stays on the latest version.
func TestReviewKeyAndBuildReviews(t *testing.T) {
	now := time.Now()
	checked := now
	mk := func(channel, latest, digest string) *updateJSON {
		u := &updateJSON{Channel: channel, Latest: latest, TagDigest: digest, Class: "digest", CheckedAt: &checked}
		u.ReviewKey = reviewKey(u)
		return u
	}
	if k := mk("nightly", "1.2.0", "sha256:aaaa").ReviewKey; k != "sha256:aaaa" {
		t.Errorf("channel key = %q", k)
	}
	if k := mk("", "", "sha256:aaaa").ReviewKey; k != "sha256:aaaa" {
		t.Errorf("unknown-latest key = %q", k)
	}
	if k := mk("", "1.2.0", "sha256:aaaa").ReviewKey; k != "1.2.0" {
		t.Errorf("version key = %q", k)
	}
	if k := mk("", "", "").ReviewKey; k != "" {
		t.Errorf("no key = %q", k)
	}

	reviews := map[string]store.Review{"c": {Kind: "reviewed", Version: "sha256:aaaa"}}
	for _, channel := range []string{"nightly", ""} { // channel tag, then floating tag with unknown latest
		latest := ""
		if channel != "" {
			latest = "1.2.0"
		}
		if reviewFor(reviews, "c", mk(channel, latest, "sha256:aaaa"), now) == nil {
			t.Errorf("%q: same build should stay reviewed", channel)
		}
		if reviewFor(reviews, "c", mk(channel, latest, "sha256:bbbb"), now) != nil {
			t.Errorf("%q: a new build should end the review", channel)
		}
	}
	if reviewFor(reviews, "c", mk("", "", ""), now) != nil {
		t.Error("a build review without a known digest should not show")
	}
	// A version review is unchanged by the digest.
	vr := map[string]store.Review{"c": {Kind: "reviewed", Version: "1.2.0"}}
	if reviewFor(vr, "c", mk("", "1.2.0", "sha256:bbbb"), now) == nil {
		t.Error("version review should survive a rebuild")
	}
	if reviewFor(vr, "c", mk("", "1.3.0", "sha256:bbbb"), now) != nil {
		t.Error("version review should end on a newer version")
	}
}

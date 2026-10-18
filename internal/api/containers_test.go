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
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/store"

	"github.com/bartvs98/tidemark/internal/docker"
)

func TestContainers(t *testing.T) {
	rec := do(t, handler(builtSPA), "/api/containers")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var body containerList
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}

	if body.Source.Kind != "snapshot" || body.Source.CapturedAt == nil {
		t.Errorf("source = %+v", body.Source)
	}

	// Sorted by stack (ungrouped first, the UI moves them last), then name.
	var names []string
	for _, c := range body.Containers {
		names = append(names, c.Name)
	}
	want := []string{"orphan", "retagged", "jellyfin", "uptime-kuma"}
	if len(names) != len(want) {
		t.Fatalf("names = %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names = %v, want %v", names, want)
		}
	}

	uptime-kuma := body.Containers[3]
	if uptime-kuma.Stack != "monitoring" || uptime-kuma.Image.Display != "louislam/uptime-kuma:latest" ||
		uptime-kuma.Image.Registry != "docker.io" || uptime-kuma.Image.Tag != "latest" || uptime-kuma.Image.Platform != "linux/arm64/v8" {
		t.Errorf("uptime-kuma = %+v", uptime-kuma)
	}
	if orphan := body.Containers[0]; !orphan.Image.Missing || orphan.Image.Created != nil {
		t.Errorf("orphan image = %+v", orphan.Image)
	}
}

type failingSource struct{}

func (failingSource) Containers(context.Context) ([]docker.Container, error) {
	return nil, errors.New("can't reach Docker at unix:///var/run/docker.sock: permission denied")
}
func (failingSource) Info() docker.SourceInfo { return docker.SourceInfo{Kind: "docker"} }

func TestContainersSourceError(t *testing.T) {
	h := New(Deps{Version: "1.2.3", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Source: failingSource{}}).Handler(fstest.MapFS{})
	rec := do(t, h, "/api/containers")
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusBadGateway || body["error"] == "" {
		t.Errorf("got %d %v", rec.Code, body)
	}
}

type tagsRegistry struct{}

func (tagsRegistry) Tags(context.Context, imageref.Ref) ([]string, error) {
	return []string{"10.9.11", "10.10.7", "latest"}, nil
}
func (tagsRegistry) Digest(context.Context, imageref.Ref, string) (string, error) {
	return "", errors.New("unused")
}

func TestChecksAndUpdates(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	src := docker.NewSnapshot("../docker/testdata/snapshot.json")
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	checks := check.New(src, tagsRegistry{}, db, log)
	h := New(Deps{Version: "1.2.3", Log: log, Source: src, Checks: checks}).Handler(fstest.MapFS{})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/checks", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("POST /api/checks = %d", rec.Code)
	}
	deadline := time.Now().Add(5 * time.Second)
	for checks.Status().Running && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	var body containerList
	_ = json.Unmarshal(do(t, h, "/api/containers").Body.Bytes(), &body)
	if body.Check == nil || body.Check.Running || body.Check.Done != body.Check.Total || body.Check.FinishedAt == nil || body.Check.Trigger != "manual" {
		t.Errorf("check = %+v", body.Check)
	}
	for _, c := range body.Containers {
		if c.Name != "jellyfin" {
			continue
		}
		u := c.Update
		if u == nil || u.Running != "10.9.11" || u.Source != "tag" || u.Latest != "10.10.7" || u.Class != "minor" {
			t.Errorf("jellyfin update = %+v", u)
		}
	}
}

// flakyRegistry is tagsRegistry that can lose the network.
type flakyRegistry struct {
	tagsRegistry
	down *atomic.Bool
}

func (r flakyRegistry) Tags(ctx context.Context, ref imageref.Ref) ([]string, error) {
	if r.down.Load() {
		return nil, errors.New("dial tcp: lookup index.docker.io: no such host")
	}
	return r.tagsRegistry.Tags(ctx, ref)
}

// A failed check keeps the last good result: the update describes that one and says
// when refreshing it failed. Without one, it carries the error alone.
func TestUpdateAfterFailedCheck(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `[{"tag_name":"v10.10.7","body":"## Breaking\nnew config","published_at":"2026-10-01T00:00:00Z"},
			{"tag_name":"v10.9.11","body":"old","published_at":"2026-08-01T00:00:00Z"}]`)
	}))
	defer gh.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	src := docker.NewSnapshot("../docker/testdata/snapshot.json")
	down := new(atomic.Bool)
	checks := check.New(src, flakyRegistry{down: down}, db, log)
	h := New(Deps{
		Version: "t", Log: log, Source: src, Checks: checks,
		Changelogs: changelog.New(db, "", log, changelog.WithBaseURLs(gh.URL, gh.URL, gh.URL)),
	}).Handler(fstest.MapFS{})
	run := func(offline bool) {
		t.Helper()
		down.Store(offline)
		checks.Start(t.Context(), check.TriggerManual)
		for deadline := time.Now().Add(5 * time.Second); checks.Status().Running && time.Now().Before(deadline); {
			time.Sleep(5 * time.Millisecond)
		}
	}
	// The update as raw JSON, to see which fields are absent.
	raw := func() map[string]any {
		t.Helper()
		var body struct {
			Containers []struct {
				Name   string         `json:"name"`
				Update map[string]any `json:"update"`
			} `json:"containers"`
		}
		if err := json.Unmarshal(do(t, h, "/api/containers").Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		for _, c := range body.Containers {
			if c.Name == "jellyfin" {
				return c.Update
			}
		}
		t.Fatal("no jellyfin")
		return nil
	}

	// Never succeeded.
	run(true)
	u := raw()
	if u["source"] != "unknown" || u["class"] != "none" || u["level"] != "none" || u["error"] == nil || u["failedAt"] == nil || u["checkedAt"] != nil || u["latest"] != nil {
		t.Errorf("never succeeded: %v", u)
	}

	// Succeeded; the detail request stores the release notes.
	run(false)
	do(t, h, "/api/containers/jellyfin")
	ok := raw()
	if ok["latest"] != "10.10.7" || ok["class"] != "minor" || ok["level"] != "breaking" || ok["checkedAt"] == nil || ok["error"] != nil || ok["failedAt"] != nil {
		t.Errorf("succeeded: %v", ok)
	}

	// Failed with a kept result: same result, level and checkedAt, plus the failure.
	run(true)
	u = raw()
	if u["latest"] != "10.10.7" || u["class"] != "minor" || u["source"] != "tag" || u["level"] != "breaking" || u["reasons"] == nil ||
		u["checkedAt"] != ok["checkedAt"] || u["error"] == nil || u["failedAt"] == nil {
		t.Errorf("kept: %v", u)
	}
	// The detail page assesses the kept result too.
	var d detailJSON
	if err := json.Unmarshal(do(t, h, "/api/containers/jellyfin").Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if du := d.Container.Update; du == nil || du.Level != analysis.Breaking || du.Error == "" || du.CheckedAt == nil || du.FailedAt == nil {
		t.Errorf("detail of kept result = %+v", du)
	}
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/imageref"

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
	checks := check.New(src, tagsRegistry{}, log)
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
	if body.Check == nil || body.Check.Running || body.Check.Done != body.Check.Total || body.Check.FinishedAt == nil {
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

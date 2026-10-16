package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"testing/fstest"

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
	h := New("1.2.3", slog.New(slog.NewTextHandler(io.Discard, nil)), failingSource{}).Handler(fstest.MapFS{})
	rec := do(t, h, "/api/containers")
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusBadGateway || body["error"] == "" {
		t.Errorf("got %d %v", rec.Code, body)
	}
}

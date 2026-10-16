package docker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func byName(t *testing.T, list []Container) map[string]Container {
	t.Helper()
	m := map[string]Container{}
	for _, c := range list {
		m[c.Name] = c
	}
	return m
}

// checkFixture asserts the decoded testdata/snapshot.json, shared by snapshot and live client tests.
func checkFixture(t *testing.T, list []Container) {
	t.Helper()
	if len(list) != 4 {
		t.Fatalf("got %d containers, want 4", len(list))
	}
	c := byName(t, list)

	j := c["jellyfin"]
	if j.Project != "media" || j.Service != "jellyfin" || j.State != "running" || j.Status != "Up 3 days" {
		t.Errorf("jellyfin = %+v", j)
	}
	if j.Image.Platform != "linux/amd64" || j.Image.RepoDigests[0] != "jellyfin/jellyfin@sha256:111" {
		t.Errorf("jellyfin image = %+v", j.Image)
	}
	if j.Image.Labels["org.opencontainers.image.source"] == "" || j.Image.Created.Year() != 2024 {
		t.Errorf("jellyfin image labels/created = %+v", j.Image)
	}
	if !j.Created.Equal(time.Unix(1759305600, 0)) {
		t.Errorf("jellyfin created = %v", j.Created)
	}

	if b := c["uptime-kuma"]; b.ImageRef != "louislam/uptime-kuma" || b.Image.Platform != "linux/arm64/v8" || b.Image.Labels == nil {
		t.Errorf("uptime-kuma = %+v", b)
	}
	if r := c["retagged"]; r.ImageRef != "example/retagged:2.0" || r.Project != "" || r.Labels == nil {
		t.Errorf("retagged: bare image ID should fall back to the image's tag, got %+v", r)
	}
	if o := c["orphan"]; !o.Image.Missing || o.Image.ID != "sha256:ddd" {
		t.Errorf("orphan: image should be marked missing, got %+v", o.Image)
	}
}

func TestSnapshot(t *testing.T) {
	s := NewSnapshot("testdata/snapshot.json")
	list, err := s.Containers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	checkFixture(t, list)

	info := s.Info()
	if info.Kind != "snapshot" || info.CapturedAt == nil || info.CapturedAt.Day() != 1 {
		t.Errorf("info = %+v", info)
	}
}

func TestSnapshotMissingFile(t *testing.T) {
	if _, err := NewSnapshot("testdata/nope.json").Containers(context.Background()); err == nil {
		t.Error("expected error")
	}
}

// fakeEngine serves the fixture through Engine API endpoints and fails the test
// if anything asks for container inspect (which exposes environment variables).
func fakeEngine(t *testing.T) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile("testdata/snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Containers json.RawMessage
		Images     []json.RawMessage
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	images := map[string]json.RawMessage{}
	for _, raw := range f.Images {
		var id struct{ Id string }
		_ = json.Unmarshal(raw, &id)
		images[id.Id] = raw
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("non-GET request: %s %s", r.Method, r.URL)
		}
		switch {
		case r.URL.Path == "/containers/json":
			if r.URL.Query().Get("all") != "1" {
				t.Errorf("containers list without all=1")
			}
			_, _ = w.Write(f.Containers)
		case strings.HasPrefix(r.URL.Path, "/containers/"):
			t.Errorf("container inspect must never be called: %s", r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
		case strings.HasPrefix(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/images/"), "/json")
			if raw, ok := images[id]; ok {
				_, _ = w.Write(raw)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"No such image: ` + id + `"}`))
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestClient(t *testing.T) {
	srv := fakeEngine(t)
	defer srv.Close()

	c, err := NewClient("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := c.Containers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	checkFixture(t, list)
}

func TestClientEngineError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Request forbidden by administrative rules"}`))
	}))
	defer srv.Close()

	c, _ := NewClient("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	_, err := c.Containers(context.Background())
	if err == nil || !strings.Contains(err.Error(), "forbidden by administrative rules") {
		t.Errorf("err = %v", err)
	}
}

func TestClientUnreachable(t *testing.T) {
	c, _ := NewClient("tcp://127.0.0.1:1")
	_, err := c.Containers(context.Background())
	if err == nil || !strings.Contains(err.Error(), "can't reach Docker") {
		t.Errorf("err = %v", err)
	}
}

func TestNewClientHosts(t *testing.T) {
	for _, ok := range []string{"unix:///var/run/docker.sock", "tcp://socket-proxy:2375", "http://10.0.0.2:2375"} {
		if _, err := NewClient(ok); err != nil {
			t.Errorf("NewClient(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{"ssh://me@server", "npipe:////./pipe/docker_engine", "tcp://", "::"} {
		if _, err := NewClient(bad); err == nil {
			t.Errorf("NewClient(%q): expected error", bad)
		}
	}
}

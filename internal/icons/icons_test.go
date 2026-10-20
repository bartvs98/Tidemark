package icons

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bartvs98/tidemark/internal/imageref"
)

const testIndex = `[
  {"Name":"Home Assistant","Reference":"home-assistant","SVG":"Yes","PNG":"Yes"},
  {"Name":"AdGuard Home","Reference":"adguard-home","SVG":"Yes","PNG":"Yes"},
  {"Name":"Vaultwarden","Reference":"vaultwarden","SVG":"Yes","PNG":"Yes"},
  {"Name":"Uptime Kuma","Reference":"uptime-kuma","SVG":"Yes","PNG":"Yes"},
  {"Name":"Plex","Reference":"plex","SVG":"Yes","PNG":"Yes"},
  {"Name":"Plex","Reference":"plex","SVG":"Yes","PNG":"Yes"},
  {"Name":"Stirling PDF","Reference":"stirling-pdf","SVG":"Yes","PNG":"Yes"},
  {"Name":"Cup (Updates)","Reference":"cup-updates","SVG":"Yes","PNG":"Yes"},
  {"Name":"Cupdate","Reference":"cupdate","SVG":"No","PNG":"Yes"},
  {"Name":"CUPS","Reference":"cups","SVG":"Yes","PNG":"Yes"},
  {"Name":"PostgreSQL","Reference":"postgresql","SVG":"Yes","PNG":"Yes"},
  {"Name":"Redis","Reference":"redis","SVG":"Yes","PNG":"Yes","Light":"Yes","Dark":"No"},
  {"Name":"Redis Commander","Reference":"redis-commander","SVG":"No","PNG":"Yes"},
  {"Name":"The Lounge","Reference":"the-lounge","SVG":"Yes","PNG":"Yes"},
  {"Name":"authentik","Reference":"authentik","SVG":"Yes","PNG":"Yes"},
  {"Name":"Docker","Reference":"docker","SVG":"Yes","PNG":"Yes"},
  {"Name":"Server","Reference":"server","SVG":"Yes","PNG":"Yes"},
  {"Name":"Bad","Reference":"../etc/passwd","SVG":"Yes","PNG":"Yes"},
  {"Name":"No files","Reference":"nothing","SVG":"No","PNG":"No"}
]`

func newTestService(t *testing.T, base string) *Service {
	t.Helper()
	s := New(base, t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := s.load([]byte(testIndex)); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestResolve(t *testing.T) {
	s := newTestService(t, "http://unused")
	tests := []struct {
		image, name, service string
		labels               map[string]string
		want                 string
	}{
		{"homeassistant/home-assistant:latest", "homeassistant", "", nil, "home-assistant"},
		{"adguard/adguardhome:v0.107.79", "adguard", "", nil, "adguard-home"}, // normalized match
		{"vaultwarden/server:1.37.2", "vaultwarden", "", nil, "vaultwarden"},  // generic "server" skipped
		{"woodpeckerci/woodpecker-agent:alpine", "woodpecker-agent", "", nil, "uptime-kuma"},    // trimmed
		{"meisnate12/plex-meta-manager:v4", "cwa", "", nil, "plex"},
		{"lobaro/restic-backup-docker:2.1.6", "backup", "", nil, "vaultwarden"},
		{"frooodle/s-pdf:2.6.13", "stirling", "stirling", nil, "stirling-pdf"}, // service + unique prefix
		{"ghcr.io/sergi0g/cup:v3.5.1", "cupdate", "cupdate", nil, "cup-updates"},    // image beats container name
		{"postgres:16-alpine", "app_db", "", nil, "postgresql"},                 // alias
		{"redis:latest", "app_redis", "", nil, "redis"},                     // exact beats prefix
		{"lscr.io/linuxserver/thelounge:1.20.0", "chat", "", nil, "the-lounge"},
		{"ghcr.io/goauthentik/server:15", "authentik", "authentik", nil, "authentik"},
		{"jlesage/mkvtoolnix:latest", "mkvtoolnix", "mkvtoolnix", nil, ""},
		{"redis:7", "x", "", map[string]string{LabelIcon: "cups"}, "cups"},
		{"redis:7", "x", "", map[string]string{LabelIcon: "none"}, ""},
		{"redis:7", "x", "", map[string]string{LabelIcon: "unknown-ref"}, "redis"}, // bad label falls back
		{"ghcr.io/bartvs98/tidemark:1", "tidemark", "", nil, "tidemark"},           // built in
		{"redis:7", "x", "", map[string]string{LabelIcon: "tidemark"}, "tidemark"},
		{"ghcr.io/bartvs98/tidemark:1", "tidemark", "", map[string]string{LabelIcon: "none"}, ""},
		{"ghcr.io/bartvs98/tidemark:1", "tidemark", "", map[string]string{LabelIcon: "cups"}, "cups"}, // label beats built in
		{"tecnativa/docker-socket-proxy:v0.5.0", "tidemark-socket-proxy", "", nil, "docker"},          // alias
		{"lscr.io/linuxserver/socket-proxy:latest", "socket-proxy", "", nil, "docker"},
	}
	for _, tt := range tests {
		ref, err := imageref.Parse(tt.image)
		if err != nil {
			t.Fatal(err)
		}
		ic, ok := s.Resolve(ref, tt.name, tt.service, tt.labels)
		if got := map[bool]string{true: ic.Ref, false: ""}[ok]; got != tt.want {
			t.Errorf("Resolve(%s, %s) = %q, want %q", tt.image, tt.name, got, tt.want)
		}
	}
}

func TestLoadSkipsUnusableEntries(t *testing.T) {
	s := newTestService(t, "http://unused")
	if _, ok := s.Lookup("../etc/passwd"); ok {
		t.Error("unsafe reference was loaded")
	}
	if _, ok := s.Lookup("nothing"); ok {
		t.Error("entry without files was loaded")
	}
	if ic, _ := s.Lookup("cupdate"); ic.Ext != "png" {
		t.Errorf("cupdate ext = %q, want png fallback", ic.Ext)
	}
}

func TestResolveBeforeIndexLoaded(t *testing.T) {
	s := New("http://unused", t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	ref, _ := imageref.Parse("redis")
	if _, ok := s.Resolve(ref, "redis", "", nil); ok {
		t.Error("expected no icon before the index is loaded")
	}
	ref, _ = imageref.Parse("ghcr.io/bartvs98/tidemark:latest")
	if ic, ok := s.Resolve(ref, "tidemark", "", nil); !ok || ic.Ref != "tidemark" {
		t.Errorf("built-in icon = %+v, %v; want tidemark before the index loads", ic, ok)
	}
}

func TestServeBuiltin(t *testing.T) {
	s := New("http://unused", t.TempDir(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := httptest.NewRecorder()
	s.Serve(rec, httptest.NewRequest("GET", "/api/icons/tidemark", nil), "tidemark", VariantDefault)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/svg+xml" || !strings.Contains(rec.Body.String(), "<title>Tidemark</title>") {
		t.Errorf("builtin: %d %q %q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.Serve(rec, httptest.NewRequest("GET", "/api/icons/tidemark?variant=light", nil), "tidemark", VariantLight)
	if rec.Code != 404 {
		t.Errorf("builtin light variant: %d, want 404", rec.Code)
	}
}

func TestServeFetchesOnceAndCaches(t *testing.T) {
	var hits atomic.Int32
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/svg/redis.svg":
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))
		case "/svg/redis-light.svg":
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" fill="#fff"/>`))
		case "/png/cupdate.png":
			_, _ = w.Write([]byte("\x89PNG"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer cdn.Close()
	s := newTestService(t, cdn.URL)

	for range 2 {
		rec := httptest.NewRecorder()
		s.Serve(rec, httptest.NewRequest("GET", "/api/icons/redis", nil), "redis", VariantDefault)
		if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/svg+xml" {
			t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Content-Type"))
		}
		if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") {
			t.Error("missing sandbox CSP on SVG")
		}
	}
	if hits.Load() != 1 {
		t.Errorf("CDN hit %d times, want 1 (second request should come from disk)", hits.Load())
	}
	if _, err := os.Stat(filepath.Join(s.dir, "redis.svg")); err != nil {
		t.Errorf("icon not cached on disk: %v", err)
	}

	rec := httptest.NewRecorder()
	s.Serve(rec, httptest.NewRequest("GET", "/api/icons/cupdate", nil), "cupdate", VariantDefault)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("png: got %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}

	rec = httptest.NewRecorder()
	s.Serve(rec, httptest.NewRequest("GET", "/api/icons/redis?variant=light", nil), "redis", VariantLight)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "#fff") {
		t.Errorf("light variant: got %d %q", rec.Code, rec.Body.String())
	}
	for _, v := range []string{VariantDark, "../x", "huge"} { // redis has no dark variant
		rec = httptest.NewRecorder()
		s.Serve(rec, httptest.NewRequest("GET", "/api/icons/redis", nil), "redis", v)
		if rec.Code != http.StatusNotFound {
			t.Errorf("variant %q: got %d, want 404", v, rec.Code)
		}
	}
}

func TestServeRejectsUnknownAndMissing(t *testing.T) {
	cdn := httptest.NewServer(http.NotFoundHandler())
	defer cdn.Close()
	s := newTestService(t, cdn.URL)
	for _, ref := range []string{"not-in-index", "../etc/passwd", "uptime-kuma"} { // uptime-kuma: in index, 404 on CDN
		rec := httptest.NewRecorder()
		s.Serve(rec, httptest.NewRequest("GET", "/api/icons/x", nil), ref, VariantDefault)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", ref, rec.Code)
		}
	}
}

func TestStartUsesCacheThenRefreshes(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"Name":"Jellyfin","Reference":"jellyfin","SVG":"Yes","PNG":"Yes"}]`))
	}))
	defer cdn.Close()

	dir := t.TempDir()
	stale := filepath.Join(dir, indexFile)
	if err := os.WriteFile(stale, []byte(testIndex), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(stale, old, old)

	s := New(cdn.URL, dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)

	if _, ok := s.Lookup("redis"); !ok {
		t.Fatal("stale cache should be usable immediately")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := s.Lookup("jellyfin"); ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("index was not refreshed from the CDN")
}

func TestServeFetchesDifferentIconsInParallel(t *testing.T) {
	var inFlight, peak atomic.Int32
	release := make(chan struct{})
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		<-release
		inFlight.Add(-1)
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))
	}))
	defer cdn.Close()
	s := newTestService(t, cdn.URL)

	var wg sync.WaitGroup
	for _, ref := range []string{"redis", "uptime-kuma", "vaultwarden", "redis"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			s.Serve(rec, httptest.NewRequest("GET", "/api/icons/"+ref, nil), ref, VariantDefault)
		}()
	}
	deadline := time.Now().Add(2 * time.Second)
	for peak.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	close(release)
	wg.Wait()
	if peak.Load() != 3 {
		t.Errorf("peak concurrent downloads = %d, want 3 (three different icons; the duplicate waits)", peak.Load())
	}
}

func TestServeRemembersFailures(t *testing.T) {
	var hits atomic.Int32
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusForbidden) // jsDelivr refuses some files with 403
	}))
	defer cdn.Close()
	s := newTestService(t, cdn.URL)
	for range 3 {
		rec := httptest.NewRecorder()
		s.Serve(rec, httptest.NewRequest("GET", "/api/icons/redis", nil), "redis", VariantDefault)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d, want 404", rec.Code)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("CDN asked %d times, want 1 (failures are remembered)", hits.Load())
	}
}

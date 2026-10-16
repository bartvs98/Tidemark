package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/bartvs98/tidemark/internal/docker"
)

var builtSPA = fstest.MapFS{
	"index.html":                       {Data: []byte("<tm-root></tm-root>")},
	"main-JEKI5GUH.js":                 {Data: []byte("console.log(1)")},
	"favicon.svg":                      {Data: []byte("<svg/>")},
	"media/geist-latin-XEQ56JUF.woff2": {Data: []byte("font")},
}

func do(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func handler(spa fstest.MapFS) http.Handler {
	return New("1.2.3", slog.New(slog.NewTextHandler(io.Discard, nil)), docker.NewSnapshot("../docker/testdata/snapshot.json")).Handler(spa)
}

func TestHealth(t *testing.T) {
	rec := do(t, handler(builtSPA), "/api/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["version"] != "1.2.3" {
		t.Errorf("body = %v", body)
	}
}

func TestUnknownAPIRouteIsJSON404(t *testing.T) {
	rec := do(t, handler(builtSPA), "/api/nope")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		t.Errorf("got %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestSPA(t *testing.T) {
	h := handler(builtSPA)
	tests := []struct {
		path      string
		code      int
		body      string
		immutable bool
	}{
		{"/", 200, "<tm-root>", false},
		{"/containers/nginx", 200, "<tm-root>", false}, // client-side route falls back to index
		{"/main-JEKI5GUH.js", 200, "console.log", true},
		{"/media/geist-latin-XEQ56JUF.woff2", 200, "font", true},
		{"/favicon.svg", 200, "<svg/>", false},
		{"/missing-AAAA1111.js", 404, "", false},
	}
	for _, tt := range tests {
		rec := do(t, h, tt.path)
		if rec.Code != tt.code || !strings.Contains(rec.Body.String(), tt.body) {
			t.Errorf("%s: got %d %q", tt.path, rec.Code, rec.Body.String())
		}
		if got := strings.Contains(rec.Header().Get("Cache-Control"), "immutable"); got != tt.immutable {
			t.Errorf("%s: immutable cache = %v, want %v", tt.path, got, tt.immutable)
		}
	}
}

func TestSPANotBuilt(t *testing.T) {
	rec := do(t, handler(fstest.MapFS{}), "/")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "npm run build") {
		t.Errorf("got %d %q", rec.Code, rec.Body.String())
	}
}

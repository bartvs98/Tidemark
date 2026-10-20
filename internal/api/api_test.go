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
	return New(Deps{
		Version: "1.2.3",
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		Source:  docker.NewSnapshot("../docker/testdata/snapshot.json"),
	}).Handler(spa)
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

func TestWritesNeedTidemarkHeader(t *testing.T) {
	h := reviewHandler(t)
	review := `{"kind":"reviewed","version":"10.10.7"}`
	tests := []struct {
		name, method, target, body string
		header                     string
		want                       int
	}{
		{"checks without header", http.MethodPost, "/api/checks", "", "", http.StatusForbidden},
		{"checks with header", http.MethodPost, "/api/checks", "", "1", http.StatusAccepted},
		{"review without header", http.MethodPut, "/api/containers/jellyfin/review", review, "", http.StatusForbidden},
		{"delete without header", http.MethodDelete, "/api/containers/jellyfin/review", "", "", http.StatusForbidden},
		{"review with header", http.MethodPut, "/api/containers/jellyfin/review", review, "1", http.StatusOK},
		{"get needs none", http.MethodGet, "/api/containers", "", "", http.StatusOK},
		{"options needs none", http.MethodOptions, "/api/containers", "", "", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			if tt.header != "" {
				req.Header.Set("X-Tidemark", tt.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("got %d, want %d (%s)", rec.Code, tt.want, rec.Body)
			}
			if tt.want == http.StatusForbidden && !strings.Contains(rec.Body.String(), "missing X-Tidemark header") {
				t.Errorf("body = %s", rec.Body)
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := handler(builtSPA)
	for _, target := range []string{"/api/health", "/api/nope", "/", "/main-JEKI5GUH.js"} {
		rec := do(t, h, target)
		for k, v := range map[string]string{
			"X-Content-Type-Options":  "nosniff",
			"X-Frame-Options":         "DENY",
			"Content-Security-Policy": "frame-ancestors 'none'",
		} {
			if got := rec.Header().Get(k); got != v {
				t.Errorf("%s %s = %q, want %q", target, k, got, v)
			}
		}
	}
}

package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/docker"
)

func TestSettings(t *testing.T) {
	h := New(Deps{
		Version:    "sha-abc1234",
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		Source:     docker.NewSnapshot("../docker/testdata/snapshot.json"),
		Schedule:   "0 */6 * * *",
		ConfigFile: "/data/config.yaml",
	}).Handler(builtSPA)

	rec := do(t, h, "/api/settings")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var body settingsJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Version != "sha-abc1234" || body.Source.Kind != "snapshot" || body.Checks.Schedule != "0 */6 * * *" ||
		body.Rules.Path != "/data/config.yaml" || !body.Rules.Missing || body.Icons || body.GitHub.Token || body.GitHub.RateLimit != nil {
		t.Errorf("settings = %+v", body)
	}
}

func TestSettingsGitHub(t *testing.T) {
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"resources":{"core":{"limit":5000,"remaining":4812,"reset":1792351465}}}`)
	}))
	defer github.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(Deps{
		Log:        log,
		Source:     docker.NewSnapshot("../docker/testdata/snapshot.json"),
		Changelogs: changelog.New(nil, "ghp_topsecret", log, changelog.WithBaseURLs(github.URL, github.URL, github.URL)),
	}).Handler(builtSPA)

	rec := do(t, h, "/api/settings")
	var body settingsJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.GitHub.Token || body.GitHub.RateLimit == nil || body.GitHub.RateLimit.Remaining != 4812 || body.Checks.Schedule != "" {
		t.Errorf("settings = %+v", body)
	}
	// Secrets are only ever reported as set or not.
	if strings.Contains(rec.Body.String(), "topsecret") {
		t.Errorf("body contains the token: %s", rec.Body.String())
	}
}

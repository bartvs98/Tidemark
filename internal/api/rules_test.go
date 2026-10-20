package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/store"
)

type staticSource []docker.Container

func (s staticSource) Containers(context.Context) ([]docker.Container, error) { return s, nil }
func (staticSource) Info() docker.SourceInfo                                  { return docker.SourceInfo{Kind: "docker"} }

type majorRegistry struct{}

func (majorRegistry) Tags(context.Context, imageref.Ref) ([]string, error) {
	return []string{"1.0.0", "1.2.0", "2.0.0"}, nil
}
func (majorRegistry) Digest(context.Context, imageref.Ref, string) (string, error) {
	return "", errors.New("unused")
}

func TestRulesHeldAndRulesErrorJSON(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte("containers:\n  held:\n    repo: github.com/team/app\n    pin: \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mk := func(name string, labels map[string]string) docker.Container {
		return docker.Container{ID: name, Name: name, ImageRef: "team/app:1.0.0", Image: docker.Image{ID: "sha256:1"}, Labels: labels}
	}
	src := staticSource{
		mk("plain", nil),
		mk("held", map[string]string{"tidemark.allow-major": "false"}),
		mk("skipped", map[string]string{"tidemark.ignore": "yes", "tidemark.tag-pattern": `^\d+\.\d+\.\d+$`}),
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	checks := check.New(src, majorRegistry{}, db, log)
	checks.UseRulesFile(cfg)
	h := New(Deps{Version: "t", Log: log, Source: src, Checks: checks}).Handler(fstest.MapFS{})

	runCheck := func() {
		t.Helper()
		send(t, h, http.MethodPost, "/api/checks", "")
		for deadline := time.Now().Add(5 * time.Second); checks.Status().Running && time.Now().Before(deadline); {
			time.Sleep(5 * time.Millisecond)
		}
	}
	list := func() map[string]map[string]any {
		t.Helper()
		var body struct {
			Check      map[string]any   `json:"check"`
			Containers []map[string]any `json:"containers"`
		}
		if err := json.Unmarshal(do(t, h, "/api/containers").Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		out := map[string]map[string]any{"#check": body.Check}
		for _, c := range body.Containers {
			out[c["name"].(string)] = c
		}
		return out
	}
	runCheck()
	got := list()

	if _, ok := got["#check"]["rulesError"]; ok {
		t.Errorf("rulesError set for a good file: %v", got["#check"])
	}
	if _, ok := got["plain"]["rules"]; ok {
		t.Errorf("plain container has rules: %v", got["plain"]["rules"])
	}
	if u := got["plain"]["update"].(map[string]any); u["latest"] != "2.0.0" || u["held"] != nil || u["heldBy"] != nil {
		t.Errorf("plain update = %v", u)
	}

	wantHeld := map[string]any{
		"allowMajor": false,
		"repo":       "github.com/team/app",
		"from":       map[string]any{"allow-major": "label", "repo": "config"},
		"problems":   []any{`config containers.held pin: "x" is not a version prefix`},
	}
	if r := got["held"]["rules"]; !reflect.DeepEqual(r, wantHeld) {
		t.Errorf("held rules =\n %v\nwant\n %v", r, wantHeld)
	}
	u := got["held"]["update"].(map[string]any)
	if u["latest"] != "1.2.0" || u["class"] != "minor" || u["held"] != "2.0.0" || u["heldBy"] != "allow-major" {
		t.Errorf("held update = %v", u)
	}
	// The verdict follows the held-in Latest: a minor jump, not the major one.
	if u["level"] != "none" {
		t.Errorf("held level = %v, want none (minor jump without notes)", u["level"])
	}

	wantSkipped := map[string]any{
		"ignore":     true,
		"tagPattern": `^\d+\.\d+\.\d+$`,
		"from":       map[string]any{"ignore": "label", "tag-pattern": "label"},
	}
	if r := got["skipped"]["rules"]; !reflect.DeepEqual(r, wantSkipped) {
		t.Errorf("skipped rules = %v, want %v", r, wantSkipped)
	}
	if _, ok := got["skipped"]["update"]; ok {
		t.Error("ignored container must have no update")
	}

	// A broken config file: reported on the check status; labels still apply.
	if err := os.WriteFile(cfg, []byte("containers: [oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCheck()
	got = list()
	if e, _ := got["#check"]["rulesError"].(string); e == "" {
		t.Errorf("check = %v, want a rulesError", got["#check"])
	}
	if r := got["held"]["rules"].(map[string]any); r["allowMajor"] != false || r["repo"] != "github.com/team/app" {
		t.Errorf("held rules after a bad file = %v (the previous file should stay in use)", r)
	}
}

func TestIgnoredDetailMakesNoOutsideCalls(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request for an ignored container: %s", r.URL)
		http.NotFound(w, r)
	}))
	defer gh.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	src := staticSource{{ID: "x", Name: "skipped", ImageRef: "ghcr.io/team/app:1.0.0", Image: docker.Image{ID: "sha256:1"},
		Labels: map[string]string{"tidemark.ignore": "true", "tidemark.repo": "github.com/team/app"}}}
	h := New(Deps{
		Version: "t", Log: log, Source: src, Checks: check.New(src, majorRegistry{}, db, log),
		Changelogs: changelog.New(db, "", log, changelog.WithBaseURLs(gh.URL, gh.URL, gh.URL)),
	}).Handler(fstest.MapFS{})

	rec := do(t, h, "/api/containers/skipped")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["changelog"]; ok {
		t.Errorf("ignored container has a changelog: %v", body["changelog"])
	}
	if c := body["container"].(map[string]any); c["rules"].(map[string]any)["ignore"] != true || c["update"] != nil {
		t.Errorf("container = %v", c)
	}
}

// channelRegistry: a nightly tag that moved on, next to a stable major jump.
type channelRegistry struct{ majorRegistry }

func (channelRegistry) Digest(_ context.Context, _ imageref.Ref, tag string) (string, error) {
	if tag == "nightly" {
		return "sha256:new", nil
	}
	return "", errors.New("unused")
}

// A channel build (nightly) is never compared with stable releases: no verdict from
// their notes, only "new image" when the tag moved.
func TestChannelUpdateJSON(t *testing.T) {
	src := staticSource{{ID: "n", Name: "n", ImageRef: "team/app:nightly",
		Image: docker.Image{ID: "sha256:1", RepoDigests: []string{"team/app@sha256:old"}}}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	checks := check.New(src, channelRegistry{}, db, log)
	h := New(Deps{Version: "t", Log: log, Source: src, Checks: checks}).Handler(fstest.MapFS{})
	send(t, h, http.MethodPost, "/api/checks", "")
	for deadline := time.Now().Add(5 * time.Second); checks.Status().Running && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}

	var body containerList
	if err := json.Unmarshal(do(t, h, "/api/containers").Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	u := body.Containers[0].Update
	if u == nil || u.Channel != "nightly" || u.Running != "" || u.Latest != "2.0.0" || u.Class != "digest" ||
		!u.NewImage || u.Level != 0 || len(u.Reasons) != 0 {
		t.Errorf("update = %+v", u)
	}
}

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

	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/source"
	"github.com/bartvs98/tidemark/internal/store"
)

func TestContainerDetail(t *testing.T) {
	// Fake GitHub: the fixture's jellyfin image declares github.com/jellyfin/jellyfin-packaging.
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/repos/jellyfin/jellyfin-packaging/releases") {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `[
			{"tag_name":"v10.10.7","name":"10.10.7","body":"## Breaking\nnew config","html_url":"https://github.com/jellyfin/jellyfin-packaging/releases/tag/v10.10.7","published_at":"2026-10-01T00:00:00Z"},
			{"tag_name":"v10.10.0","name":"10.10.0","body":"big one","html_url":"u","published_at":"2026-09-01T00:00:00Z"},
			{"tag_name":"v10.9.11","name":"10.9.11","body":"old","html_url":"u","published_at":"2026-08-01T00:00:00Z"}
		]`)
	}))
	defer gh.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	src := docker.NewSnapshot("../docker/testdata/snapshot.json")
	checks := check.New(src, tagsRegistry{}, db, log)
	h := New(Deps{
		Version: "t", Log: log, Source: src, Checks: checks,
		Changelogs: changelog.New(db, "", log, changelog.WithBaseURLs(gh.URL, gh.URL, gh.URL)),
	}).Handler(fstest.MapFS{})

	checks.Start(t.Context(), check.TriggerManual)
	for deadline := time.Now().Add(5 * time.Second); checks.Status().Running && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}

	rec := do(t, h, "/api/containers/jellyfin")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var d detailJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	cl := d.Changelog
	if cl == nil || cl.Repo.Name != "jellyfin/jellyfin-packaging" || cl.Repo.How != "image" || cl.Kind != "releases" {
		t.Fatalf("changelog = %+v", cl)
	}
	if len(cl.Releases) != 2 || cl.Releases[0].Tag != "v10.10.7" || cl.Releases[1].Tag != "v10.10.0" {
		t.Errorf("releases = %+v", cl.Releases)
	}
	if cl.Links.Compare != "https://github.com/jellyfin/jellyfin-packaging/compare/v10.9.11...v10.10.7" ||
		cl.Links.Running != "https://github.com/jellyfin/jellyfin-packaging/tree/v10.9.11" {
		t.Errorf("links = %+v", cl.Links)
	}
	if len(d.Image.Digests) != 1 || d.Image.Labels["org.opencontainers.image.source"] == "" {
		t.Errorf("image = %+v", d.Image)
	}

	if rec := do(t, h, "/api/containers/ghost"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown container: %d", rec.Code)
	}
}

func TestInListLink(t *testing.T) {
	repo, _ := source.ParseURL("https://github.com/linuxserver/docker-thelounge")
	ch := changelog.Changes{Kind: changelog.KindReleases, FromTag: "1.20.0-ls166", FromName: "1.20.0-ls166", ToTag: "1.20.0-ls168", FromIndex: 14}
	got := toChangelogJSON(repo, source.HowImage, ch).Links.InList
	want := "https://github.com/linuxserver/docker-thelounge/releases?page=2#:~:text=1.20.0%2Dls166,-Compare"
	if got != want {
		t.Errorf("in-list link\n got %s\nwant %s", got, want)
	}

	ch.FromIndex = 9 // last entry of page 1: no ?page
	if got := toChangelogJSON(repo, source.HowImage, ch).Links.InList; got != "https://github.com/linuxserver/docker-thelounge/releases#:~:text=1.20.0%2Dls166,-Compare" {
		t.Errorf("page 1 link = %s", got)
	}

	ch.FromIndex = -1 // position unknown: no link
	if got := toChangelogJSON(repo, source.HowImage, ch).Links.InList; got != "" {
		t.Errorf("unknown position should give no link, got %s", got)
	}
}

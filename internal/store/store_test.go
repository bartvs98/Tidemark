package store

import (
	"context"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func open(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sub", "tidemark.db") // parent dir is created
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func TestMigrationsAreIdempotent(t *testing.T) {
	s, path := open(t)
	s.Close()
	s2, err := Open(path) // re-open applies nothing new
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	files, _ := fs.Glob(migrations, "migrations/*.sql")
	var n int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil || n != len(files) {
		t.Errorf("schema_migrations rows = %d, want %d (%v)", n, len(files), err)
	}
}

func TestRuns(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	if _, ok, _ := s.LastFinishedRun(ctx); ok {
		t.Fatal("no runs yet")
	}
	start := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	id, err := s.StartRun(ctx, "manual", start)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.LastFinishedRun(ctx); ok {
		t.Fatal("an unfinished run must not count")
	}
	if err := s.FinishRun(ctx, id, start.Add(9*time.Second), 24, ""); err != nil {
		t.Fatal(err)
	}
	r, ok, err := s.LastFinishedRun(ctx)
	if err != nil || !ok || r.ID != id || r.Trigger != "manual" || r.Images != 24 || !r.FinishedAt.Equal(start.Add(9*time.Second)) {
		t.Errorf("run = %+v ok=%v err=%v", r, ok, err)
	}
}

func TestResults(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	runID, _ := s.StartRun(ctx, "schedule", time.Now())
	at := time.Date(2026, 10, 16, 12, 0, 0, 123, time.UTC)

	a := Result{ContainerName: "jellyfin", ImageRef: "jellyfin/jellyfin:10.9.11", ImageID: "sha256:a", Running: "10.9.11",
		Source: "tag", Latest: "10.10.7", Class: "minor", Floating: false, NewImage: true, CheckedAt: at, RunID: runID}
	b := Result{ContainerName: "redis", ImageRef: "redis:latest", ImageID: "sha256:b", Error: "rate limited", CheckedAt: at}
	for _, r := range []Result{a, b} {
		if err := s.SaveResult(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	a.Latest, a.Class = "10.10.8", "minor" // upsert
	if err := s.SaveResult(ctx, a); err != nil {
		t.Fatal(err)
	}

	got, err := s.Results(ctx)
	if err != nil || len(got) != 2 {
		t.Fatalf("results = %v, %v", got, err)
	}
	byName := map[string]Result{got[0].ContainerName: got[0], got[1].ContainerName: got[1]}
	if r := byName["jellyfin"]; r != a {
		t.Errorf("jellyfin round trip:\n got %+v\nwant %+v", r, a)
	}
	if r := byName["redis"]; r.Error != "rate limited" || r.RunID != 0 {
		t.Errorf("redis = %+v", r)
	}

	if err := s.KeepResults(ctx, []string{"redis"}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Results(ctx)
	if len(got) != 1 || got[0].ContainerName != "redis" {
		t.Errorf("after KeepResults: %+v", got)
	}
}

func TestTagCache(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	if _, _, ok := s.LoadTags(ctx, "docker.io/library/redis"); ok {
		t.Fatal("empty cache")
	}
	at := time.Now().UTC().Truncate(time.Second)
	if err := s.SaveTags(ctx, "docker.io/library/redis", []string{"8.10.1", "latest"}, at); err != nil {
		t.Fatal(err)
	}
	tags, fetched, ok := s.LoadTags(ctx, "docker.io/library/redis")
	if !ok || !slices.Equal(tags, []string{"8.10.1", "latest"}) || !fetched.Equal(at) {
		t.Errorf("tags=%v fetched=%v ok=%v", tags, fetched, ok)
	}
}

func TestReleasesAndRepoState(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	repo := "github.com/home-assistant/core"
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	if _, ok, _ := s.RepoState(ctx, repo); ok {
		t.Fatal("no state yet")
	}
	if err := s.SaveReleases(ctx, repo, []Release{
		{Tag: "2026.10.0", Name: "2026.10", Body: "## Breaking", URL: "u1", PublishedAt: at},
		{Tag: "2026.10.1", Body: "fixes", PublishedAt: at.Add(time.Hour)},
		{Tag: "2026.11.0b0", Prerelease: true},
	}, false); err != nil {
		t.Fatal(err)
	}
	// Upsert keeps others; replace drops them.
	_ = s.SaveReleases(ctx, repo, []Release{{Tag: "2026.10.1", Body: "fixes, edited"}}, false)
	rs, _ := s.Releases(ctx, repo)
	if len(rs) != 3 {
		t.Fatalf("got %d releases", len(rs))
	}
	_ = s.SaveReleases(ctx, repo, []Release{{Tag: "1.0.0"}}, true)
	if rs, _ := s.Releases(ctx, repo); len(rs) != 1 || rs[0].Tag != "1.0.0" || !rs[0].PublishedAt.IsZero() {
		t.Errorf("after replace: %+v", rs)
	}

	st := RepoState{Repo: repo, Kind: "releases", ETag: `W/"abc"`, HasMore: true, FetchedAt: at}
	if err := s.SaveRepoState(ctx, st); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.RepoState(ctx, repo)
	if err != nil || !ok || got != st {
		t.Errorf("state = %+v ok=%v err=%v", got, ok, err)
	}
}

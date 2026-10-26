package api

import (
	"context"
	"net/http"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bartvs98/tidemark/internal/store"
)

// Unused repos and tag lists go once they weren't fetched for pruneAfter; their notes keep
// counting in the lifetime totals.
func TestPruneUnused(t *testing.T) {
	app := ctrApp("app", "2.0.0", map[string]string{"tidemark.repo": "github.com/team/app"})
	ignored := ctrApp("ignored", "1.0.0", map[string]string{"tidemark.repo": "github.com/team/ignored", "tidemark.ignore": "true"})
	ignored.ImageRef = "team/ignored:1.0.0"
	srv, _, db := statsServer(t, staticSource{app, ignored})
	h := srv.Handler(fstest.MapFS{})
	ctx := context.Background()
	now := time.Now().UTC()
	old, recent := now.Add(-40*day), now.Add(-day)
	breaking := "## Breaking changes\n- The old API is removed"
	for _, r := range []struct {
		repo    string
		fetched time.Time
		rels    []store.Release
	}{
		{"github.com/team/app", old, []store.Release{{Tag: "v1.0.0", Body: "- Bug fixes"}}},     // in use
		{"github.com/team/ignored", old, []store.Release{{Tag: "v1.0.0", Body: "- Bug fixes"}}}, // ignored, still in use
		{"github.com/team/recent", recent, []store.Release{{Tag: "v1", Body: "- Bug fixes"}}},   // fetched lately
		{"github.com/team/old", old, []store.Release{{Tag: "v1", Body: breaking}, {Tag: "v2", Body: "- Bug fixes"}}},
	} {
		if err := db.SaveReleases(ctx, r.repo, r.rels, false); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveRepoState(ctx, store.RepoState{Repo: r.repo, Kind: "releases", FetchedAt: r.fetched}); err != nil {
			t.Fatal(err)
		}
	}
	for repo, at := range map[string]time.Time{"docker.io/team/app": old, "docker.io/team/ignored": old, "docker.io/team/gone": old, "docker.io/team/fresh": recent} {
		if err := db.SaveTags(ctx, repo, []string{"1.0.0"}, at); err != nil {
			t.Fatal(err)
		}
	}
	before := getStats(t, h, "all").Lifetime
	if before.ReleasesStored != 5 || before.BreakingFlagged != 1 || before.NotesWords != 4*3+9 {
		t.Fatalf("lifetime before = %+v", before)
	}
	gen := srv.statsGeneration()

	repos, tagLists, err := srv.PruneUnused(ctx)
	if err != nil || repos != 1 || tagLists != 1 {
		t.Fatalf("pruned %d repos, %d tag lists, err %v", repos, tagLists, err)
	}
	states, _ := db.AllRepoStates(ctx)
	if _, ok := states["github.com/team/old"]; ok || len(states) != 3 {
		t.Errorf("repo states left = %v", states)
	}
	if rels, _ := db.Releases(ctx, "github.com/team/old"); len(rels) != 0 {
		t.Errorf("pruned releases left = %v", rels)
	}
	for repo, want := range map[string]bool{"docker.io/team/app": true, "docker.io/team/ignored": true, "docker.io/team/gone": false, "docker.io/team/fresh": true} {
		if _, _, ok := db.LoadTags(ctx, repo); ok != want {
			t.Errorf("tag list %s kept = %v", repo, ok)
		}
	}
	if srv.statsGeneration() == gen {
		t.Error("pruning didn't invalidate the stats")
	}
	if _, ok := srv.levels["github.com/team/old"]; ok {
		t.Error("pruned repo's levels still cached")
	}
	if after := getStats(t, h, "all").Lifetime; after.ReleasesStored != before.ReleasesStored ||
		after.BreakingFlagged != before.BreakingFlagged || after.NotesWords != before.NotesWords {
		t.Errorf("lifetime after = %+v, before %+v", after, before)
	}

	// Nothing more to prune: the stats stay cached.
	gen = srv.statsGeneration()
	if repos, tagLists, err := srv.PruneUnused(ctx); err != nil || repos != 0 || tagLists != 0 || srv.statsGeneration() != gen {
		t.Errorf("second prune: %d repos, %d tag lists, err %v", repos, tagLists, err)
	}
}

// Without the container list nothing is known to be unused, so nothing goes.
func TestPruneUnusedWithoutDocker(t *testing.T) {
	srv, _, db := statsServer(t, failingSource{})
	ctx := context.Background()
	_ = db.SaveRepoState(ctx, store.RepoState{Repo: "github.com/team/old", Kind: "releases", FetchedAt: time.Now().Add(-40 * day)})
	if _, _, err := srv.PruneUnused(ctx); err == nil {
		t.Error("no error without Docker")
	}
	if states, _ := db.AllRepoStates(ctx); len(states) != 1 {
		t.Errorf("repo states = %v", states)
	}
}

// A stats response is reused until something it reads changes, per range; a partial one isn't.
func TestStatsCache(t *testing.T) {
	srv, checks, db := statsServer(t, staticSource{ctrApp("app", "1.0.0", nil)})
	runChecks(t, checks)
	h := srv.Handler(fstest.MapFS{})
	ctx := context.Background()

	first := getStats(t, h, "30d")
	_ = db.AddAICall(ctx, store.AICall{At: time.Now(), Provider: "anthropic", Model: "m", Trigger: "manual", Duration: time.Second})
	second := getStats(t, h, "30d")
	if !second.GeneratedAt.Equal(first.GeneratedAt) || second.AI != nil {
		t.Fatalf("second request rebuilt: %v vs %v, ai %+v", second.GeneratedAt, first.GeneratedAt, second.AI)
	}
	// Another range is its own entry.
	if out := getStats(t, h, "all"); out.AI == nil {
		t.Error("all range served from the 30d entry")
	}

	if rec := send(t, h, http.MethodPut, "/api/containers/app/review", `{"kind":"reviewed","version":"2.0.0"}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT review = %d: %s", rec.Code, rec.Body)
	}
	out := getStats(t, h, "30d")
	if out.AI == nil || len(out.Behind.Containers) != 1 || out.Behind.Containers[0].Review != "reviewed" {
		t.Errorf("after a review: ai %+v, behind %+v", out.AI, out.Behind.Containers)
	}
	if rec := send(t, h, http.MethodDelete, "/api/containers/app/review", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE review = %d", rec.Code)
	}
	if out := getStats(t, h, "30d"); out.Behind.Containers[0].Review != "" {
		t.Errorf("after undoing the review: %+v", out.Behind.Containers[0])
	}

	_ = db.AddAICall(ctx, store.AICall{At: time.Now(), Provider: "anthropic", Model: "m", Trigger: "manual", Duration: time.Second})
	if ai := getStats(t, h, "30d").AI; ai == nil || ai.Calls != 1 {
		t.Errorf("cached ai = %+v", ai)
	}
	srv.InvalidateStats()
	if ai := getStats(t, h, "30d").AI; ai == nil || ai.Calls != 2 {
		t.Errorf("ai after InvalidateStats = %+v", ai)
	}

	// Past statsMaxAge it's rebuilt too.
	srv.statsMu.Lock()
	c := srv.statsCache["30d"]
	c.built = c.built.Add(-statsMaxAge)
	srv.statsCache["30d"] = c
	srv.statsMu.Unlock()
	_ = db.AddAICall(ctx, store.AICall{At: time.Now(), Provider: "anthropic", Model: "m", Trigger: "manual", Duration: time.Second})
	if ai := getStats(t, h, "30d").AI; ai == nil || ai.Calls != 3 {
		t.Errorf("ai after max age = %+v", ai)
	}

	partial, _, _ := statsServer(t, failingSource{})
	if out := getStats(t, partial.Handler(fstest.MapFS{}), "30d"); !out.Partial || len(partial.statsCache) != 0 {
		t.Errorf("partial response cached: %v, %v", out.Partial, partial.statsCache)
	}
}

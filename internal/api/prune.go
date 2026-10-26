package api

import (
	"context"
	"time"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/store"
)

// pruneAfter: stored notes, repo state and cached tag lists no current container uses are
// deleted once they weren't fetched for this long.
const pruneAfter = 30 * 24 * time.Hour

// PruneUnused deletes stored release notes and repo state of repos no current container uses
// and that weren't fetched in pruneAfter (30 days), keeping them in the lifetime totals, and
// tag_cache rows of images no current container uses, unused as long. It's called by the
// after-run hook.
func (s *Server) PruneUnused(ctx context.Context) (repos, tagLists int, err error) {
	if s.store == nil {
		return 0, 0, nil
	}
	list, err := s.source.Containers(ctx)
	if err != nil {
		return 0, 0, err // without the containers, nothing is known to be unused
	}
	// Ignored containers count too: un-ignoring one shouldn't have to fetch everything again.
	usedRepos, usedImages := map[string]bool{}, map[string]bool{}
	for _, c := range list {
		ref, err := imageref.Parse(c.ImageRef)
		if err != nil {
			continue
		}
		usedImages[ref.Registry+"/"+ref.Repository] = true
		if repo, ok := s.notesRepo(c, ref, s.rulesFor(c)); ok {
			usedRepos[repo.Key()] = true
		}
	}
	cutoff := time.Now().Add(-pruneAfter)

	// Without the changelog service no container has a notes repo, so every repo would look unused.
	if s.changelogs != nil {
		if repos, err = s.pruneRepos(ctx, usedRepos, cutoff); err != nil {
			return 0, 0, err
		}
	}
	tagLists, err = s.store.PruneTagCache(ctx, usedImages, cutoff)
	if repos > 0 || tagLists > 0 {
		s.InvalidateStats()
	}
	return repos, tagLists, err
}

// pruneRepos deletes the stale repos' notes and state, counting what goes into the pruned
// totals the way the stats count stored notes.
func (s *Server) pruneRepos(ctx context.Context, used map[string]bool, cutoff time.Time) (int, error) {
	stale, err := s.store.StaleRepos(ctx, used, cutoff)
	if err != nil || len(stale) == 0 {
		return 0, err
	}
	var totals store.PrunedTotals
	for _, key := range stale {
		rels, err := s.store.Releases(ctx, key)
		if err != nil {
			return 0, err
		}
		for _, r := range rels {
			level, words := releaseLevel(r.Body)
			totals.Releases++
			if level == analysis.Breaking {
				totals.Breaking++
			}
			totals.Words += words
		}
	}
	if err := s.store.PruneRepos(ctx, stale, totals); err != nil {
		return 0, err
	}
	s.levelsMu.Lock()
	for _, key := range stale {
		delete(s.levels, key)
	}
	s.levelsMu.Unlock()
	for _, key := range stale {
		s.changelogs.Forget(key)
	}
	return len(stale), nil
}

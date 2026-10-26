package changelog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bartvs98/tidemark/internal/source"
	"github.com/bartvs98/tidemark/internal/store"
	"github.com/bartvs98/tidemark/internal/version"
)

// fakeHosts serves GitHub API, raw.githubusercontent and GitLab routes from one server.
type fakeHosts struct {
	t         *testing.T
	releases  map[string][]string // repo path → release tags, newest first
	files     map[string]string   // "owner/name/CHANGELOG.md" → content
	apiCalls  atomic.Int32
	notMod    atomic.Int32
	rateLimit atomic.Bool
	secondary atomic.Bool // secondary limit: Retry-After with quota left
}

func (f *fakeHosts) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/repos/"):
		f.apiCalls.Add(1)
		if f.rateLimit.Load() {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if f.secondary.Load() {
			w.Header().Set("X-RateLimit-Remaining", "42")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		repo := strings.TrimSuffix(strings.TrimPrefix(p, "/repos/"), "/releases")
		tags, ok := f.releases[repo]
		if !ok {
			http.NotFound(w, r)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		etag := fmt.Sprintf(`"%s-%d-%d"`, repo, len(tags), page)
		if r.Header.Get("If-None-Match") == etag {
			f.notMod.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		start, end := (page-1)*per, page*per
		var out []map[string]any
		for i := start; i < end && i < len(tags); i++ {
			out = append(out, map[string]any{
				"tag_name": tags[i], "name": "Release " + tags[i], "body": "notes for " + tags[i],
				"html_url": "https://github.com/" + repo + "/releases/tag/" + tags[i], "published_at": time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Hour).Format(time.RFC3339),
				"prerelease": strings.Contains(tags[i], "rc"), "draft": tags[i] == "draft",
			})
		}
		w.Header().Set("ETag", etag)
		_ = json.NewEncoder(w).Encode(out)
	case strings.HasPrefix(p, "/raw/"):
		parts := strings.SplitN(strings.TrimPrefix(p, "/raw/"), "/", 4) // owner/name/HEAD/file
		if body, ok := f.files[parts[0]+"/"+parts[1]+"/"+parts[3]]; ok {
			_, _ = io.WriteString(w, body)
			return
		}
		http.NotFound(w, r)
	case strings.HasPrefix(p, "/api/v4/projects/"):
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"tag_name": "v2.0.0", "name": "2.0", "description": "gitlab notes", "released_at": "2026-10-01T00:00:00Z", "_links": map[string]string{"self": "https://gitlab.com/x"}},
		})
	default:
		f.t.Errorf("unexpected request %s", r.URL)
		http.NotFound(w, r)
	}
}

func setup(t *testing.T, f *fakeHosts) *Service {
	t.Helper()
	f.t = t
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st, "", slog.New(slog.NewTextHandler(io.Discard, nil)), WithBaseURLs(srv.URL, srv.URL+"/raw", srv.URL))
}

func v(t *testing.T, s string) *version.Version {
	t.Helper()
	x, ok := version.Parse(s)
	if !ok {
		t.Fatalf("bad version %q", s)
	}
	return &x
}

func tags(cs Changes) []string {
	var out []string
	for _, r := range cs.Releases {
		out = append(out, r.Tag)
	}
	return out
}

func gh(path string) source.Repo { return source.Repo{Host: "github.com", Path: path} }

func TestReleasesInRange(t *testing.T) {
	f := &fakeHosts{releases: map[string][]string{
		"home-assistant/core": {"2026.11.0-rc.1", "2026.10.1", "2026.10.0", "draft", "2026.9.4", "2026.9.0", "2026.8.0"},
	}}
	s := setup(t, f)
	ctx := context.Background()
	repo := gh("home-assistant/core")

	if err := s.Sync(ctx, repo, v(t, "2026.9.0"), nil); err != nil {
		t.Fatal(err)
	}
	cs, err := s.Changes(ctx, repo, v(t, "2026.9.0"), v(t, "2026.10.1"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"2026.10.1", "2026.10.0", "2026.9.4"}; !slices.Equal(tags(cs), want) {
		t.Errorf("range = %v, want %v (no pre-releases, drafts, or running itself)", tags(cs), want)
	}
	if cs.FromTag != "2026.9.0" || cs.ToTag != "2026.10.1" || cs.Kind != KindReleases || cs.Partial {
		t.Errorf("changes = %+v", cs)
	}
	// GitHub lists newest first, pre-releases included: 2026.11.0rc1, 2026.10.1, 2026.10.0, 2026.9.4 come first.
	if cs.FromName != "Release 2026.9.0" {
		t.Errorf("running release title = %q", cs.FromName)
	}
	if cs.FromIndex != 4 {
		t.Errorf("running release position = %d, want 4", cs.FromIndex)
	}

	// Unknown running version: just the newest release up to latest.
	cs, _ = s.Changes(ctx, repo, nil, v(t, "2026.10.1"))
	if !slices.Equal(tags(cs), []string{"2026.10.1"}) {
		t.Errorf("unknown running = %v", tags(cs))
	}
}

func TestSyncIsCachedAndConditional(t *testing.T) {
	f := &fakeHosts{releases: map[string][]string{"filebrowser/filebrowser": {"v2.32.1", "v2.32.0"}}}
	s := setup(t, f)
	ctx := context.Background()
	repo := gh("filebrowser/filebrowser")

	_ = s.Sync(ctx, repo, v(t, "2.32.0"), nil)
	_ = s.Sync(ctx, repo, v(t, "2.32.0"), nil) // fresh: no request
	if f.apiCalls.Load() != 1 {
		t.Fatalf("api calls = %d, want 1", f.apiCalls.Load())
	}

	// Stale: the next sync sends the ETag and gets a free 304.
	st, _, _ := s.store.RepoState(ctx, repo.Key())
	st.FetchedAt = st.FetchedAt.Add(-7 * time.Hour)
	_ = s.store.SaveRepoState(ctx, st)
	if err := s.Sync(ctx, repo, v(t, "2.32.0"), nil); err != nil {
		t.Fatal(err)
	}
	if f.notMod.Load() != 1 {
		t.Errorf("expected a 304, got %d", f.notMod.Load())
	}
	if cs, _ := s.Changes(ctx, repo, v(t, "2.32.0"), v(t, "2.32.1")); !slices.Equal(tags(cs), []string{"v2.32.1"}) || cs.FromTag != "v2.32.0" {
		t.Errorf("after 304: %+v", cs)
	}
}

// A release whose image is already out but whose notes aren't stored yet is fetched
// after latestRetry, not freshFor; once it's there, the plain freshFor applies again.
func TestSyncFetchesAgainForMissingLatest(t *testing.T) {
	f := &fakeHosts{releases: map[string][]string{"a/b": {"v1.2.0", "v1.1.0"}}}
	s := setup(t, f)
	ctx := context.Background()
	repo := gh("a/b")
	age := func(d time.Duration) {
		st, _, _ := s.store.RepoState(ctx, repo.Key())
		st.FetchedAt = time.Now().Add(-d)
		_ = s.store.SaveRepoState(ctx, st)
	}
	calls := func(want int32) {
		t.Helper()
		if got := f.apiCalls.Load(); got != want {
			t.Errorf("api calls = %d, want %d", got, want)
		}
	}

	_ = s.Sync(ctx, repo, v(t, "1.1.0"), v(t, "1.2.0"))
	calls(1)
	// The image is at 1.3.0 (linuxserver build tag), the notes stop at 1.2.0.
	f.releases["a/b"] = []string{"v1.3.0", "v1.2.0", "v1.1.0"}
	latest := v(t, "1.3.0-ls12")
	_ = s.Sync(ctx, repo, v(t, "1.1.0"), latest) // fetched just now: wait
	calls(1)
	age(90 * time.Minute)
	_ = s.Sync(ctx, repo, v(t, "1.1.0"), latest)
	calls(2)
	if cs, _ := s.Changes(ctx, repo, v(t, "1.1.0"), latest); !slices.Equal(tags(cs), []string{"v1.3.0", "v1.2.0"}) {
		t.Errorf("range = %v", tags(cs))
	}
	// Has the latest now: back to freshFor.
	age(90 * time.Minute)
	_ = s.Sync(ctx, repo, v(t, "1.1.0"), latest)
	calls(2)

	// A latest that never gets notes is asked about at most every latestRetry.
	age(10 * time.Minute)
	_ = s.Sync(ctx, repo, v(t, "1.1.0"), v(t, "1.4.0"))
	calls(2)
	age(90 * time.Minute)
	_ = s.Sync(ctx, repo, v(t, "1.1.0"), v(t, "1.4.0"))
	calls(3)
	_ = s.Sync(ctx, repo, v(t, "1.1.0"), v(t, "1.4.0"))
	calls(3)
}

// Check now (RecheckSince) skips the latestRetry wait for notes from before it, but only
// for GitHub with a token, and only once per run.
func TestRecheckSince(t *testing.T) {
	f := &fakeHosts{releases: map[string][]string{"a/b": {"v1.1.0"}}}
	s := setup(t, f)
	repo := gh("a/b")
	_ = s.Sync(context.Background(), repo, v(t, "1.1.0"), nil)
	st, _, _ := s.store.RepoState(context.Background(), repo.Key())
	st.FetchedAt = time.Now().Add(-10 * time.Minute) // before the run, within latestRetry
	_ = s.store.SaveRepoState(context.Background(), st)
	f.releases["a/b"] = []string{"v1.2.0", "v1.1.0"}
	latest := v(t, "1.2.0")
	ctx := RecheckSince(context.Background(), time.Now())

	_ = s.Sync(ctx, repo, v(t, "1.1.0"), latest)
	if got := f.apiCalls.Load(); got != 1 {
		t.Errorf("without a token: api calls = %d, want 1 (waits)", got)
	}
	s.SetToken("t")
	_ = s.Sync(ctx, repo, v(t, "1.1.0"), latest)
	if got := f.apiCalls.Load(); got != 2 {
		t.Errorf("with a token: api calls = %d, want 2", got)
	}
	f.releases["a/b"] = []string{"v1.3.0", "v1.2.0", "v1.1.0"}
	_ = s.Sync(ctx, repo, v(t, "1.1.0"), v(t, "1.3.0")) // fetched during this run: waits again
	if got := f.apiCalls.Load(); got != 2 {
		t.Errorf("second sync in the run: api calls = %d, want 2", got)
	}
}

func TestPaginationUntilCovered(t *testing.T) {
	var many []string
	for i := 250; i >= 1; i-- {
		many = append(many, fmt.Sprintf("1.%d.0", i))
	}
	f := &fakeHosts{releases: map[string][]string{"big/repo": many}}
	s := setup(t, f)
	ctx := context.Background()
	repo := gh("big/repo")

	// Running 1.180.0 is on page 1 (releases 250…151): one request.
	_ = s.Sync(ctx, repo, v(t, "1.180.0"), nil)
	if f.apiCalls.Load() != 1 {
		t.Errorf("api calls = %d, want 1", f.apiCalls.Load())
	}
	// Running 1.20.0 is on page 3: the sync goes deeper even though page 1 is fresh.
	_ = s.Sync(ctx, repo, v(t, "1.20.0"), nil)
	cs, _ := s.Changes(ctx, repo, v(t, "1.20.0"), v(t, "1.250.0"))
	if len(cs.Releases) != 230 || cs.Partial || cs.FromTag != "1.20.0" {
		t.Errorf("got %d releases, partial=%v from=%q", len(cs.Releases), cs.Partial, cs.FromTag)
	}
}

func TestPaginationLimitIsNotRetriedWhileFresh(t *testing.T) {
	var many []string
	for i := 600; i >= 1; i-- {
		many = append(many, fmt.Sprintf("1.%d.0", i))
	}
	f := &fakeHosts{releases: map[string][]string{"huge/repo": many}}
	s := setup(t, f)
	ctx := context.Background()
	repo := gh("huge/repo")

	// Running 1.50.0 is beyond the 5 pages a sync fetches.
	_ = s.Sync(ctx, repo, v(t, "1.50.0"), nil)
	if f.apiCalls.Load() != maxPages {
		t.Fatalf("api calls = %d, want %d", f.apiCalls.Load(), maxPages)
	}
	// Going deeper is impossible, so while fresh the next syncs ask nothing.
	_ = s.Sync(ctx, repo, v(t, "1.50.0"), nil)
	_ = s.Sync(ctx, repo, v(t, "1.50.0"), nil)
	if f.apiCalls.Load() != maxPages {
		t.Errorf("api calls = %d after resyncs, want %d", f.apiCalls.Load(), maxPages)
	}
	if cs, _ := s.Changes(ctx, repo, v(t, "1.50.0"), v(t, "1.600.0")); !cs.Partial {
		t.Errorf("want partial notes, got %d releases", len(cs.Releases))
	}
}

func TestChangelogFallbackAndMissingRepo(t *testing.T) {
	f := &fakeHosts{
		releases: map[string][]string{"lobaro/restic-backup-docker": {}},
		files: map[string]string{"lobaro/restic-backup-docker/CHANGELOG.md": `# Changelog

## [Unreleased]
- wip

## [2.1.6] - 2026-09-01
### Fixed
- backup rotation

## [2.1.5] - 2026-08-01
- older
`},
	}
	s := setup(t, f)
	ctx := context.Background()

	repo := gh("lobaro/restic-backup-docker")
	if err := s.Sync(ctx, repo, v(t, "2.1.4"), nil); err != nil {
		t.Fatal(err)
	}
	cs, _ := s.Changes(ctx, repo, v(t, "2.1.4"), v(t, "2.1.6"))
	if cs.Kind != KindChangelog || !slices.Equal(tags(cs), []string{"2.1.6", "2.1.5"}) {
		t.Fatalf("changelog = %+v", cs)
	}
	if !strings.Contains(cs.Releases[0].Body, "backup rotation") || strings.Contains(cs.Releases[0].Body, "older") ||
		!strings.HasSuffix(cs.Releases[0].URL, "/blob/HEAD/CHANGELOG.md") || cs.Releases[0].PublishedAt.Month() != 9 {
		t.Errorf("section = %+v", cs.Releases[0])
	}

	missing := gh("nobody/nothing")
	if err := s.Sync(ctx, missing, nil, nil); err != nil {
		t.Fatal(err)
	}
	if cs, _ := s.Changes(ctx, missing, nil, nil); cs.Kind != KindNone || cs.Error != "repository not found" {
		t.Errorf("missing repo = %+v", cs)
	}
}

func TestRateLimitKeepsOldData(t *testing.T) {
	f := &fakeHosts{releases: map[string][]string{"a/b": {"1.1.0", "1.0.0"}}}
	s := setup(t, f)
	ctx := context.Background()
	repo := gh("a/b")
	_ = s.Sync(ctx, repo, v(t, "1.0.0"), nil)

	st, _, _ := s.store.RepoState(ctx, repo.Key())
	st.FetchedAt = st.FetchedAt.Add(-7 * time.Hour)
	_ = s.store.SaveRepoState(ctx, st)
	f.rateLimit.Store(true)

	if err := s.Sync(ctx, repo, v(t, "1.0.0"), nil); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v", err)
	}
	cs, _ := s.Changes(ctx, repo, v(t, "1.0.0"), v(t, "1.1.0"))
	if !slices.Equal(tags(cs), []string{"1.1.0"}) || !strings.Contains(cs.Error, "rate limit") {
		t.Errorf("after rate limit: %+v", cs)
	}

	// Until the reset, other repos fail fast without asking GitHub.
	before := f.apiCalls.Load()
	if err := s.Sync(ctx, gh("c/d"), nil, nil); !errors.Is(err, ErrRateLimited) || f.apiCalls.Load() != before {
		t.Errorf("err = %v, extra calls = %d", err, f.apiCalls.Load()-before)
	}
}

func TestSecondaryRateLimitWaits(t *testing.T) {
	f := &fakeHosts{releases: map[string][]string{"a/b": {"1.0.0"}}}
	f.secondary.Store(true)
	s := setup(t, f)
	ctx := context.Background()

	if err := s.Sync(ctx, gh("a/b"), nil, nil); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want rate limited", err)
	}
	// Until Retry-After passes, no further requests.
	before := f.apiCalls.Load()
	if err := s.Sync(ctx, gh("c/d"), nil, nil); !errors.Is(err, ErrRateLimited) || f.apiCalls.Load() != before {
		t.Errorf("err = %v, extra calls = %d", err, f.apiCalls.Load()-before)
	}
}

func TestGitLab(t *testing.T) {
	s := setup(t, &fakeHosts{})
	ctx := context.Background()
	repo := source.Repo{Host: "gitlab.com", Path: "group/sub/project"}
	if err := s.Sync(ctx, repo, nil, nil); err != nil {
		t.Fatal(err)
	}
	cs, _ := s.Changes(ctx, repo, v(t, "1.9.0"), v(t, "2.0.0"))
	if !slices.Equal(tags(cs), []string{"v2.0.0"}) || cs.Releases[0].Body != "gitlab notes" {
		t.Errorf("gitlab = %+v", cs)
	}
}

func TestParseTag(t *testing.T) {
	for _, tag := range []string{"v1.2.3", "release-1.2.3", "app@1.2.3", "release/1.2.3"} {
		if got, ok := parseTag(tag); !ok || got.String() != "1.2.3" {
			t.Errorf("parseTag(%q) = %v %v", tag, got, ok)
		}
	}
}

func TestPrereleaseLatestIncludesPrereleases(t *testing.T) {
	f := &fakeHosts{releases: map[string][]string{
		"home-assistant/core": {"2026.11.0-rc.2", "2026.11.0-rc.1", "2026.10.1", "2026.10.0"},
	}}
	s := setup(t, f)
	ctx := context.Background()
	repo := gh("home-assistant/core")
	if err := s.Sync(ctx, repo, v(t, "2026.10.0"), nil); err != nil {
		t.Fatal(err)
	}
	// A container that follows pre-releases: Latest is an rc, so the rcs in range count.
	cs, err := s.Changes(ctx, repo, v(t, "2026.10.0"), v(t, "2026.11.0-rc.2"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"2026.11.0-rc.2", "2026.11.0-rc.1", "2026.10.1"}; !slices.Equal(tags(cs), want) {
		t.Errorf("range = %v, want %v", tags(cs), want)
	}
	if cs.ToTag != "2026.11.0-rc.2" {
		t.Errorf("to tag = %q", cs.ToTag)
	}
}

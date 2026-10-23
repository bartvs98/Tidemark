// Package changelog fetches release notes for a source repository, keeps them in the
// store, and selects the ones between a running and a latest version.
//
// Sources, per repository: GitHub/GitLab releases, or, for repos without releases,
// the sections of CHANGELOG.md (or CHANGES.md). Notes are refreshed at most every
// freshFor, or every latestRetry while they don't have the latest version yet; GitHub
// requests are conditional (ETag), and "304 Not Modified" answers don't count against
// GitHub's rate limit (60 requests/hour without a token).
package changelog

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bartvs98/tidemark/internal/source"
	"github.com/bartvs98/tidemark/internal/store"
	"github.com/bartvs98/tidemark/internal/version"
)

const (
	KindReleases  = "releases"
	KindChangelog = "changelog"
	KindNone      = "none"

	freshFor     = 6 * time.Hour
	latestRetry  = time.Hour // as long as registry tag lists are cached
	perPage      = 100
	maxPages     = 5
	maxBodyBytes = 32 << 20
)

var changelogFiles = []string{"CHANGELOG.md", "CHANGES.md", "changelog.md"}

// ErrRateLimited means GitHub refused the request until its rate limit resets.
var ErrRateLimited = errors.New("GitHub rate limit reached (60 requests/hour without a token); set a GitHub token in Settings or TIDEMARK_GITHUB_TOKEN")

var errNotFound = errors.New("not found")

type Service struct {
	store *store.Store
	http  *http.Client
	log   *slog.Logger

	githubAPI string
	githubRaw string
	gitlab    string

	mu           sync.Mutex
	token        string                 // GitHub token, "" for none; see SetToken
	locks        map[string]*sync.Mutex // one sync per repo at a time
	limitedUntil time.Time              // GitHub rate limit reset; no API calls before it
	rateLimit    RateLimit              // last quota lookup (settings), see RateLimit
	rateLimitAt  time.Time

	// Changes results by repo, then by running→latest. The dashboard asks for every
	// container on each poll; a repo's entry is dropped whenever Sync stores something.
	cache map[string]map[string]Changes
}

type Option func(*Service)

// WithBaseURLs points the service at other hosts (tests).
func WithBaseURLs(githubAPI, githubRaw, gitlab string) Option {
	return func(s *Service) { s.githubAPI, s.githubRaw, s.gitlab = githubAPI, githubRaw, gitlab }
}

// WithTransport sends every request through rt (main counts them for the stats page).
func WithTransport(rt http.RoundTripper) Option {
	return func(s *Service) { s.http.Transport = rt }
}

func New(st *store.Store, githubToken string, log *slog.Logger, opts ...Option) *Service {
	s := &Service{
		store:     st,
		http:      &http.Client{Timeout: 30 * time.Second},
		token:     githubToken,
		log:       log,
		githubAPI: "https://api.github.com",
		githubRaw: "https://raw.githubusercontent.com",
		gitlab:    "https://gitlab.com",
		locks:     map[string]*sync.Mutex{},
		cache:     map[string]map[string]Changes{},
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Sync makes sure the repo's notes are stored and fresh. need is the oldest version
// that should be covered (the running version); with nil only the newest are needed.
// latest is the newest version the registry has (nil if unknown): notes without it are
// fetched again after latestRetry instead of freshFor, so a new release gets its notes
// soon after its image appears.
func (s *Service) Sync(ctx context.Context, repo source.Repo, need, latest *version.Version) error {
	key := repo.Key()
	unlock := s.lock(key)
	defer unlock()

	st, known, err := s.store.RepoState(ctx, key)
	if err != nil {
		return err
	}
	if age := time.Since(st.FetchedAt); known && age < freshFor && s.covered(ctx, key, st, need) &&
		(age < latestRetry || s.hasLatest(ctx, key, st, latest)) {
		return nil
	}
	defer s.forget(key) // anything below may store new notes or state
	return s.sync(ctx, repo, st, need)
}

func (s *Service) sync(ctx context.Context, repo source.Repo, st store.RepoState, need *version.Version) error {
	st.Repo = repo.Key()

	err := s.syncReleases(ctx, repo, &st, need)
	if errors.Is(err, errNotFound) {
		st.Kind, st.HasMore, st.ETag, st.Error, st.FetchedAt = KindNone, false, "", "repository not found", time.Now()
		return s.store.SaveRepoState(ctx, st)
	}
	if err != nil {
		// Keep what we had; record the error without marking the repo fresh.
		st.Error = err.Error()
		_ = s.store.SaveRepoState(context.WithoutCancel(ctx), st)
		return err
	}
	if st.Kind == KindNone {
		if err := s.syncChangelogFile(ctx, repo, &st); err != nil {
			st.Error = err.Error()
			_ = s.store.SaveRepoState(context.WithoutCancel(ctx), st)
			return err
		}
	}
	st.Error, st.FetchedAt = "", time.Now()
	return s.store.SaveRepoState(ctx, st)
}

// syncReleases fetches release pages, newest first, until the needed version is
// covered. It sets st.Kind to none when the repo has no releases at all.
func (s *Service) syncReleases(ctx context.Context, repo source.Repo, st *store.RepoState, need *version.Version) error {
	for page := 1; page <= maxPages; page++ {
		etag := ""
		if page == 1 && st.Kind == KindReleases {
			etag = st.ETag
		}
		rels, newETag, notModified, err := s.fetchReleases(ctx, repo, page, etag)
		if err != nil {
			return err
		}
		if !notModified {
			if page == 1 {
				st.ETag = newETag
				if len(rels) == 0 {
					st.Kind, st.HasMore = KindNone, false
					return nil
				}
			}
			if err := s.store.SaveReleases(ctx, repo.Key(), rels, false); err != nil {
				return err
			}
			st.HasMore = len(rels) == perPage
		}
		st.Kind = KindReleases
		if !st.HasMore || need == nil || s.covered(ctx, repo.Key(), *st, need) {
			return nil
		}
	}
	return nil
}

// covered: the stored notes reach back to the needed version, there's nothing older, or
// as many are stored as a sync can fetch (it always starts at page 1, so going deeper
// is impossible and retrying would only spend API quota).
func (s *Service) covered(ctx context.Context, key string, st store.RepoState, need *version.Version) bool {
	if need == nil || st.Kind != KindReleases || !st.HasMore {
		return true
	}
	rels, err := s.store.Releases(ctx, key)
	if err != nil {
		return false
	}
	for _, r := range rels {
		if v, ok := parseTag(r.Tag); ok && version.Compare(v, *need) <= 0 {
			return true
		}
	}
	return len(rels) >= maxPages*perPage
}

// hasLatest: the stored notes have the latest version or a newer one, or there's nothing
// to look for. Builds and numeric suffixes don't count ("4.0.12-ls266" is the image of
// release "v4.0.12"). A repo without notes keeps the plain freshFor: asking it more
// often would cost API quota for notes that most likely still don't exist.
func (s *Service) hasLatest(ctx context.Context, key string, st store.RepoState, latest *version.Version) bool {
	if latest == nil || st.Kind == KindNone {
		return true
	}
	rels, err := s.store.Releases(ctx, key)
	if err != nil {
		return false
	}
	want := *latest
	want.Build, want.Suffix = 0, nil
	for _, r := range rels {
		if v, ok := parseTag(r.Tag); ok {
			v.Build, v.Suffix = 0, nil
			if version.Compare(v, want) >= 0 {
				return true
			}
		}
	}
	return false
}

func (s *Service) syncChangelogFile(ctx context.Context, repo source.Repo, st *store.RepoState) error {
	for _, file := range changelogFiles {
		var raw, blob string
		if repo.IsGitHub() {
			raw = fmt.Sprintf("%s/%s/HEAD/%s", s.githubRaw, repo.Path, file)
			blob = fmt.Sprintf("%s/blob/HEAD/%s", repo.URL(), file)
		} else {
			raw = fmt.Sprintf("%s/%s/-/raw/HEAD/%s", s.gitlab, repo.Path, file)
			blob = fmt.Sprintf("%s/-/blob/HEAD/%s", repo.URL(), file)
		}
		body, _, status, err := s.get(ctx, raw, "", false)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			continue
		}
		sections := parseChangelog(string(body), blob)
		if len(sections) == 0 {
			continue
		}
		if err := s.store.SaveReleases(ctx, repo.Key(), sections, true); err != nil {
			return err
		}
		st.Kind, st.HasMore = KindChangelog, false
		return nil
	}
	st.Kind = KindNone
	return nil
}

// ── fetching ─────────────────────────────────────────────

func (s *Service) fetchReleases(ctx context.Context, repo source.Repo, page int, etag string) (rels []store.Release, newETag string, notModified bool, err error) {
	var u string
	if repo.IsGitHub() {
		u = fmt.Sprintf("%s/repos/%s/releases?per_page=%d&page=%d", s.githubAPI, repo.Path, perPage, page)
	} else {
		u = fmt.Sprintf("%s/api/v4/projects/%s/releases?per_page=%d&page=%d", s.gitlab, url.PathEscape(repo.Path), perPage, page)
	}
	body, newETag, status, err := s.get(ctx, u, etag, repo.IsGitHub())
	switch {
	case err != nil:
		return nil, "", false, err
	case status == http.StatusNotModified:
		return nil, etag, true, nil
	case status == http.StatusNotFound:
		return nil, "", false, errNotFound
	case status != http.StatusOK:
		return nil, "", false, fmt.Errorf("%s: HTTP %d", repo.Key(), status)
	}

	if repo.IsGitHub() {
		var raw []struct {
			TagName     string    `json:"tag_name"`
			Name        string    `json:"name"`
			Body        string    `json:"body"`
			HTMLURL     string    `json:"html_url"`
			PublishedAt time.Time `json:"published_at"`
			Prerelease  bool      `json:"prerelease"`
			Draft       bool      `json:"draft"`
		}
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, "", false, fmt.Errorf("%s: %w", repo.Key(), err)
		}
		for _, r := range raw {
			if !r.Draft {
				rels = append(rels, store.Release{Tag: r.TagName, Name: r.Name, Body: r.Body, URL: r.HTMLURL, PublishedAt: r.PublishedAt, Prerelease: r.Prerelease})
			}
		}
	} else {
		var raw []struct {
			TagName     string    `json:"tag_name"`
			Name        string    `json:"name"`
			Description string    `json:"description"`
			ReleasedAt  time.Time `json:"released_at"`
			Upcoming    bool      `json:"upcoming_release"`
			Links       struct {
				Self string `json:"self"`
			} `json:"_links"`
		}
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, "", false, fmt.Errorf("%s: %w", repo.Key(), err)
		}
		for _, r := range raw {
			rels = append(rels, store.Release{Tag: r.TagName, Name: r.Name, Body: r.Description, URL: r.Links.Self, PublishedAt: r.ReleasedAt, Prerelease: r.Upcoming})
		}
	}
	return rels, newETag, false, nil
}

func (s *Service) get(ctx context.Context, u, etag string, githubAPI bool) (body []byte, newETag string, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", 0, err
	}
	req.Header.Set("User-Agent", "tidemark")
	if githubAPI {
		s.mu.Lock()
		limited := time.Now().Before(s.limitedUntil)
		token := s.token
		s.mu.Unlock()
		if limited {
			return nil, "", 0, ErrRateLimited
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	defer resp.Body.Close()
	if githubAPI {
		s.noteQuota(resp.Header)
	}
	// Primary limit: Remaining 0 until Reset. Secondary limit: Retry-After seconds.
	retryAfter := resp.Header.Get("Retry-After")
	if githubAPI && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) &&
		(resp.Header.Get("X-RateLimit-Remaining") == "0" || retryAfter != "") {
		reset := time.Now().Add(15 * time.Minute)
		if sec, err := strconv.ParseInt(retryAfter, 10, 64); err == nil {
			reset = time.Now().Add(time.Duration(sec) * time.Second)
		} else if sec, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			reset = time.Unix(sec, 0)
		}
		s.mu.Lock()
		s.limitedUntil = reset
		s.mu.Unlock()
		return nil, "", resp.StatusCode, ErrRateLimited
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	return body, resp.Header.Get("ETag"), resp.StatusCode, err
}

func (s *Service) lock(key string) func() {
	s.mu.Lock()
	m, ok := s.locks[key]
	if !ok {
		m = &sync.Mutex{}
		s.locks[key] = m
	}
	s.mu.Unlock()
	m.Lock()
	return m.Unlock
}

// ── selecting ────────────────────────────────────────────

// Changes are the release notes between a running and a latest version.
type Changes struct {
	Kind     string          // releases | changelog | none ("" if never fetched)
	Releases []store.Release // newest first
	FromTag  string          // repo tag of the running version, if the repo has it
	// FromIndex is the running release's position in the forge's release list (newest
	// first, pre-releases included, 0-based), or -1 if unknown.
	FromIndex int
	// FromName is the running release's title as the forge shows it (its name, else its tag).
	FromName string
	// FromBody is the running release's own notes, to tell new lines from repeated history.
	FromBody  string
	ToTag     string // repo tag of the latest version
	Partial   bool   // older releases exist that weren't fetched
	Error     string // last fetch error
	FetchedAt time.Time
}

// Changes returns the stored notes after running, up to and including latest. With an
// unknown running version, only the newest release up to latest is returned. Results are
// cached until the repo's notes change; callers must not modify the returned slices.
func (s *Service) Changes(ctx context.Context, repo source.Repo, running, latest *version.Version) (Changes, error) {
	key := repo.Key()
	rangeKey := versionKey(running) + "→" + versionKey(latest)
	s.mu.Lock()
	ch, ok := s.cache[key][rangeKey]
	s.mu.Unlock()
	if ok {
		return ch, nil
	}
	ch, err := s.changes(ctx, key, running, latest)
	if err != nil {
		return ch, err
	}
	s.mu.Lock()
	if s.cache[key] == nil {
		s.cache[key] = map[string]Changes{}
	}
	s.cache[key][rangeKey] = ch
	s.mu.Unlock()
	return ch, nil
}

func (s *Service) forget(repo string) {
	s.mu.Lock()
	delete(s.cache, repo)
	s.mu.Unlock()
}

func versionKey(v *version.Version) string {
	if v == nil {
		return "?"
	}
	return fmt.Sprintf("%+v", *v) // all fields: String() leaves out the build ("-ls166")
}

func (s *Service) changes(ctx context.Context, key string, running, latest *version.Version) (Changes, error) {
	st, _, err := s.store.RepoState(ctx, key)
	if err != nil {
		return Changes{}, err
	}
	all, err := s.store.Releases(ctx, key)
	if err != nil {
		return Changes{}, err
	}
	out := Changes{Kind: st.Kind, Error: st.Error, FetchedAt: st.FetchedAt, FromIndex: -1}
	var fromRelease *store.Release

	type rv struct {
		r store.Release
		v version.Version
	}
	var picked []rv
	oldest := (*version.Version)(nil)
	// Pre-releases count when either end is one: running an rc, or a container that
	// follows pre-releases (Latest is an rc). Otherwise they're noise between stables.
	allowPre := running != nil && !running.Stable() || latest != nil && !latest.Stable()
	for _, r := range all {
		v, ok := parseTag(r.Tag)
		if !ok {
			continue
		}
		if oldest == nil || version.Compare(v, *oldest) < 0 {
			vv := v
			oldest = &vv
		}
		if running != nil && sameRelease(v, *running) {
			out.FromTag = r.Tag
			rel := r
			fromRelease = &rel
		}
		if latest != nil && sameRelease(v, *latest) {
			out.ToTag = r.Tag
		}
		if (r.Prerelease || !v.Stable()) && !allowPre {
			continue
		}
		if latest != nil && version.Compare(v, *latest) > 0 {
			continue
		}
		if running != nil && version.Compare(v, *running) <= 0 {
			continue
		}
		picked = append(picked, rv{r, v})
	}
	slices.SortFunc(picked, func(a, b rv) int { return version.Compare(b.v, a.v) })
	if running == nil && len(picked) > 1 {
		picked = picked[:1]
	}
	for _, p := range picked {
		out.Releases = append(out.Releases, p.r)
	}
	out.Partial = st.HasMore && running != nil && oldest != nil && version.Compare(*oldest, *running) > 0

	// The forge lists releases newest first; everything newer than the running release is
	// stored (sync pages down to it), so counting them gives its position in that list.
	if fromRelease != nil {
		out.FromName = cmp.Or(strings.TrimSpace(fromRelease.Name), fromRelease.Tag)
		out.FromBody = fromRelease.Body
	}
	if fromRelease != nil && !fromRelease.PublishedAt.IsZero() && st.Kind == KindReleases {
		out.FromIndex = 0
		for _, r := range all {
			if r.PublishedAt.After(fromRelease.PublishedAt) {
				out.FromIndex++
			}
		}
	}
	return out, nil
}

// sameRelease: same version numbers, pre-release and build; prefix, variant and numeric
// suffix don't matter ("v10.9.11" is the release of image tag "10.9.11", "v4.0.2" of
// node-red's "4.0.2-20").
func sameRelease(a, b version.Version) bool {
	a.Suffix, b.Suffix = nil, nil
	return version.Compare(a, b) == 0 && len(a.Nums) == len(b.Nums)
}

// parseTag reads a repo tag as a version, also accepting "release-1.2.3" and "app@1.2.3".
func parseTag(tag string) (version.Version, bool) {
	if v, ok := version.Parse(tag); ok {
		return v, true
	}
	t := tag
	if i := strings.LastIndex(t, "@"); i >= 0 {
		t = t[i+1:]
	}
	for _, p := range []string{"release-", "release/", "release_", "version-"} {
		t = strings.TrimPrefix(t, p)
	}
	return version.Parse(t)
}

package api

import (
	"cmp"
	"context"
	"math"
	"net/http"
	"slices"
	"time"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/review"
	"github.com/bartvs98/tidemark/internal/rules"
	"github.com/bartvs98/tidemark/internal/source"
	"github.com/bartvs98/tidemark/internal/stats"
	"github.com/bartvs98/tidemark/internal/store"
	"github.com/bartvs98/tidemark/internal/version"
)

// statsRanges are the ranges GET /api/stats takes; 0 means everything.
var statsRanges = map[string]time.Duration{
	"30d": 30 * 24 * time.Hour,
	"90d": 90 * 24 * time.Hour,
	"1y":  365 * 24 * time.Hour,
	"all": 0,
}

const (
	defaultStatsRange = "90d"
	// A time series with more points than this keeps one per UTC day.
	maxSeriesPoints = 180
	// Top-N lists.
	topJumps = 5
	// Requests over a longer span than this are summed per week instead of per day.
	maxRequestDays = 92
	// "all" starts the releases per month at the oldest release, at most this many months back.
	maxReleaseMonths = 24
	// A repo without a release for longer than this is quiet.
	quietAfterMonths = 9
)

type statsJSON struct {
	GeneratedAt time.Time    `json:"generatedAt"`
	Range       string       `json:"range"`
	Since       *time.Time   `json:"since,omitempty"`   // absent for "all"
	Partial     bool         `json:"partial,omitempty"` // Docker couldn't be listed
	Behind      behindJSON   `json:"behind"`
	Upstream    upstreamJSON `json:"upstream"`
	Health      healthJSON   `json:"health"`
	AI          *aiStatsJSON `json:"ai,omitempty"` // absent without any AI call
	Lifetime    lifetimeJSON `json:"lifetime"`
}

type behindJSON struct {
	Containers   []behindContainerJSON `json:"containers"`
	Applied      appliedJSON           `json:"applied"`
	TimeToUpdate timeToUpdateJSON      `json:"timeToUpdate"`
	// FromMessage: from the Telegram message about an update to when it was seen running.
	FromMessage medianJSON `json:"fromMessage"`
	Tide        []tideJSON `json:"tide"`
	// Now is the newest run_stats row, whatever the range; absent without one.
	Now *tideJSON `json:"now,omitempty"`
}

type behindContainerJSON struct {
	Name           string         `json:"name"`
	Stack          string         `json:"stack,omitempty"`
	Icon           *iconJSON      `json:"icon,omitempty"`
	Running        string         `json:"running"`
	Latest         string         `json:"latest"`
	Class          string         `json:"class"`
	Level          analysis.Level `json:"level"`
	Review         string         `json:"review,omitempty"` // reviewed | snoozed, while it applies
	ReleasesBehind int            `json:"releasesBehind"`
	BehindSince    *time.Time     `json:"behindSince,omitempty"`
	// ReleasesPartial: the stored notes stop before the running version, so ReleasesBehind
	// and BehindSince are lower bounds.
	ReleasesPartial bool `json:"releasesPartial,omitempty"`
}

type appliedJSON struct {
	Total    int              `json:"total"`
	PerStack []stackCountJSON `json:"perStack"`
	PerWeek  []weekCountJSON  `json:"perWeek"`
}

type stackCountJSON struct {
	Stack string `json:"stack"`
	// Removed: containers no longer on the host, whose stack isn't known any more.
	Removed bool `json:"removed,omitempty"`
	Count   int  `json:"count"`
}

type weekCountJSON struct {
	Week  string `json:"week"` // the week's Monday, UTC
	Count int    `json:"count"`
}

type timeToUpdateJSON struct {
	Count    int              `json:"count"`
	MedianMs *int64           `json:"medianMs,omitempty"`
	ByClass  []kindMedianJSON `json:"byClass"` // patch, minor, major
	ByLevel  []kindMedianJSON `json:"byLevel"` // none, attention, breaking
}

type kindMedianJSON struct {
	Kind     string `json:"kind"`
	Count    int    `json:"count"`
	MedianMs *int64 `json:"medianMs,omitempty"`
}

type medianJSON struct {
	Count    int    `json:"count"`
	MedianMs *int64 `json:"medianMs,omitempty"`
}

// appliedUpdateJSON is an applied update (a history row with a previous image); Ms is
// what the list it's in measures.
type appliedUpdateJSON struct {
	Name string    `json:"name"`
	From string    `json:"from"`
	To   string    `json:"to"`
	At   time.Time `json:"at"`
	Ms   int64     `json:"ms"`
}

type tideJSON struct {
	At        time.Time `json:"at"`
	Checked   int       `json:"checked"`
	UpToDate  int       `json:"upToDate"`
	Updates   int       `json:"updates"`
	Attention int       `json:"attention"`
	Breaking  int       `json:"breaking"`
	// Update debt: releases between running and latest over all updates (stored notes).
	ReleasesBehind int `json:"releasesBehind"`
}

type upstreamJSON struct {
	Repos        []repoStatsJSON     `json:"repos"`
	PerMonth     []monthReleasesJSON `json:"perMonth"`
	BiggestJumps []jumpJSON          `json:"biggestJumps"`
}

type repoStatsJSON struct {
	Repo       string   `json:"repo"` // owner/name
	URL        string   `json:"url"`
	Containers []string `json:"containers"`
	Releases   int      `json:"releases"` // published in range
	// Partial: the range reaches back further than the stored notes, which stop before the
	// repo's first release (more pages exist), so Releases is a lower bound.
	Partial       bool       `json:"partial,omitempty"`
	Breaking      int        `json:"breaking"`
	Attention     int        `json:"attention"`
	LastRelease   *time.Time `json:"lastRelease,omitempty"`
	MedianGapDays *float64   `json:"medianGapDays,omitempty"`
	Quiet         bool       `json:"quiet,omitempty"` // no release for quietAfterMonths
}

type monthReleasesJSON struct {
	Month     string `json:"month"` // YYYY-MM, UTC
	Releases  int    `json:"releases"`
	Breaking  int    `json:"breaking"`
	Attention int    `json:"attention"`
	// Partial: some of the repos' stored notes don't reach back to this month, so they're
	// left out of it (only repos covering the whole month count).
	Partial bool `json:"partial,omitempty"`
}

type jumpJSON struct {
	appliedUpdateJSON        // Ms: time to update, 0 when unknown
	Releases          int    `json:"releases"`
	Class             string `json:"class"`
	Partial           bool   `json:"partial,omitempty"` // Releases is a lower bound (as releasesPartial)
}

type healthJSON struct {
	Runs           runStatsJSON      `json:"runs"`
	Durations      []durationJSON    `json:"durations"`
	Failing        []failingJSON     `json:"failing"`
	FailuresByKind []kindCountJSON   `json:"failuresByKind"`
	Requests       []requestDayJSON  `json:"requests"`
	RequestsPer    string            `json:"requestsPer"` // day | week: what each requests entry covers
	RequestTotals  requestTotalsJSON `json:"requestTotals"`
	GitHub         githubQuotaJSON   `json:"github"`
}

type runStatsJSON struct {
	Total     int          `json:"total"`
	Failed    int          `json:"failed"`
	ByTrigger triggersJSON `json:"byTrigger"`
	MedianMs  *int64       `json:"medianMs,omitempty"`
	SlowestMs *int64       `json:"slowestMs,omitempty"`
}

type triggersJSON struct {
	Startup  int `json:"startup"`
	Schedule int `json:"schedule"`
	Manual   int `json:"manual"`
}

type durationJSON struct {
	At     time.Time `json:"at"`
	Ms     int64     `json:"ms"`
	Images int       `json:"images"`
	Failed bool      `json:"failed"`
}

type failingJSON struct {
	Name  string    `json:"name"`
	Kind  string    `json:"kind,omitempty"`
	Error string    `json:"error"`
	Since time.Time `json:"since"` // the failed attempt
}

type kindCountJSON struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

type requestDayJSON struct {
	Day      string `json:"day"`
	Registry int64  `json:"registry"`
	GitHub   int64  `json:"github"`
	GitLab   int64  `json:"gitlab"`
	Limited  int64  `json:"limited"`
}

type requestTotalsJSON struct {
	Registry int64 `json:"registry"`
	GitHub   int64 `json:"github"`
	GitLab   int64 `json:"gitlab"`
	Limited  int64 `json:"limited"`
}

type githubQuotaJSON struct {
	Token     bool       `json:"token"`
	Limit     *int       `json:"limit,omitempty"`
	Remaining *int       `json:"remaining,omitempty"`
	ResetAt   *time.Time `json:"resetAt,omitempty"`
}

type aiStatsJSON struct {
	Months   []aiMonthJSON `json:"months"`
	ByModel  []aiModelJSON `json:"byModel"`
	Calls    int           `json:"calls"`
	Failed   int           `json:"failed"`
	Manual   int           `json:"manual"`
	Auto     int           `json:"auto"`
	Extended int           `json:"extended"`
	MedianMs *int64        `json:"medianMs,omitempty"`
}

type aiMonthJSON struct {
	Month        string `json:"month"`
	Calls        int    `json:"calls"`
	Failed       int    `json:"failed"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
}

type aiModelJSON struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Calls        int    `json:"calls"`
	Failed       int    `json:"failed"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
}

type lifetimeJSON struct {
	Since             *time.Time         `json:"since,omitempty"` // the first check run's start
	StartedAt         time.Time          `json:"startedAt"`       // this process
	ContainersTracked int                `json:"containersTracked"`
	ReleasesStored    int                `json:"releasesStored"`
	BreakingFlagged   int                `json:"breakingFlagged"`
	FastestUpdate     *appliedUpdateJSON `json:"fastestUpdate,omitempty"`
	LongestBehind     *appliedUpdateJSON `json:"longestBehind,omitempty"`
	LongestStreak     *streakJSON        `json:"longestStreak,omitempty"`
}

type streakJSON struct {
	Runs int       `json:"runs"`
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// ── snapshot per run ─────────────────────────────────────

// RunStats counts the current containers as the dashboard's hero does, from the same
// container descriptions as the list endpoint: ignored containers only count in
// Containers and Ignored; open updates are neither reviewed nor snoozed; snoozed ones still
// count as checked. Main fills in the run, the requests and the GitHub quota.
func (s *Server) RunStats(ctx context.Context) (store.RunStats, error) {
	list, err := s.source.Containers(ctx)
	if err != nil {
		return store.RunStats{}, err
	}
	reviews := s.reviews(ctx)
	out := make([]containerJSON, 0, len(list))
	behind := 0
	for _, c := range list {
		cj := s.toContainerJSON(ctx, c, reviews)
		out = append(out, cj)
		rs := s.rulesFor(c)
		if u := cj.Update; !rs.Ignore && u != nil && u.CheckedAt != nil && u.Class != string(version.None) {
			ref, _ := imageref.Parse(c.ImageRef)
			if repo, ok := s.notesRepo(c, ref, rs); ok {
				n, _, _ := s.behindBy(ctx, repo, u)
				behind += n
			}
		}
	}
	snap := countSnapshot(out)
	snap.ReleasesBehind = behind
	return snap, nil
}

func countSnapshot(list []containerJSON) store.RunStats {
	rs := store.RunStats{Containers: len(list), Classes: map[string]int{}, Failures: map[string]int{}}
	for _, c := range list {
		if c.Rules != nil && c.Rules.Ignore {
			rs.Ignored++
			continue
		}
		u := c.Update
		if u == nil {
			continue
		}
		if u.Error != "" {
			rs.Failed++
			rs.Failures[cmp.Or(u.ErrorKind, "other")]++
		}
		if u.CheckedAt == nil {
			continue
		}
		rs.Checked++
		if u.Class == string(version.None) {
			rs.UpToDate++
			continue
		}
		rs.Classes[u.Class]++
		switch {
		case c.Review != nil && c.Review.Kind == review.Reviewed:
			rs.Reviewed++
		case c.Review != nil:
			rs.Snoozed++
		default:
			rs.Updates++
			switch u.Level {
			case analysis.Attention:
				rs.Attention++
			case analysis.Breaking:
				rs.Breaking++
			}
		}
	}
	return rs
}

// ── GET /api/stats ───────────────────────────────────────

func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeError(w, http.StatusNotImplemented, "stats are not enabled")
		return
	}
	rng := cmp.Or(r.URL.Query().Get("range"), defaultStatsRange)
	span, ok := statsRanges[rng]
	if !ok {
		writeError(w, http.StatusBadRequest, "range must be 30d, 90d, 1y or all")
		return
	}
	now := time.Now().UTC()
	var since time.Time
	if span > 0 {
		since = now.Add(-span)
	}
	out, err := s.buildStats(r.Context(), rng, now, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// statsData is everything one stats response is built from.
type statsData struct {
	now, since time.Time
	current    []currentContainer                   // nil when Docker couldn't be listed
	stackOf    map[string]string                    // current container name → Compose project
	repoOf     map[string]source.Repo               // current, non-ignored container name → notes repo
	releases   map[string][]store.Release           // without bodies
	levels     map[string]map[string]analysis.Level // repo key → tag → the release's own level
	byTag      map[string]map[string]store.Release  // repo key → tag → release
	runStats   []store.RunStats                     // all, oldest first
	runs       []store.Run                          // all, oldest first
	history    []store.HistoryEntry                 // all, oldest first
	calls      []store.AICall                       // all, oldest first
	applied    []appliedInfo                        // every history row with a previous image
	notified   map[store.NotifiedKey]store.Notified
	// coveredFrom: per repo key, where its stored notes are complete from: the start of the
	// month after its oldest stored release when older ones weren't fetched (has_more), zero
	// when the whole history is stored.
	coveredFrom map[string]time.Time
}

type currentContainer struct {
	c    docker.Container
	ref  imageref.Ref
	rs   rules.Rules
	json containerJSON
}

func (d *statsData) inRange(t time.Time) bool { return d.since.IsZero() || !t.Before(d.since) }

func (s *Server) buildStats(ctx context.Context, rng string, now, since time.Time) (statsJSON, error) {
	d, partial, err := s.loadStats(ctx, now, since)
	if err != nil {
		return statsJSON{}, err
	}
	out := statsJSON{GeneratedAt: now, Range: rng, Partial: partial}
	if !since.IsZero() {
		out.Since = &since
	}
	out.Behind = s.behindStats(ctx, d)
	out.Upstream = s.upstreamStats(d)
	out.Health = s.healthStats(d)
	out.AI = aiStats(d)
	out.Lifetime = s.lifetimeStats(d)
	return out, nil
}

// loadStats reads the store and the current containers; partial is true when Docker
// couldn't be listed (the parts that need it stay empty).
func (s *Server) loadStats(ctx context.Context, now, since time.Time) (*statsData, bool, error) {
	d := &statsData{now: now, since: since, stackOf: map[string]string{}, repoOf: map[string]source.Repo{}, byTag: map[string]map[string]store.Release{}}
	var err error
	if d.runStats, err = s.store.RunStatsSince(ctx, time.Time{}); err != nil {
		return nil, false, err
	}
	if d.runs, err = s.store.Runs(ctx); err != nil {
		return nil, false, err
	}
	if d.history, err = s.store.AllHistory(ctx); err != nil {
		return nil, false, err
	}
	// Ids follow insertion, not time (an imported or restored database), so order by time.
	slices.SortStableFunc(d.runs, func(a, b store.Run) int { return a.StartedAt.Compare(b.StartedAt) })
	slices.SortStableFunc(d.history, func(a, b store.HistoryEntry) int { return a.DetectedAt.Compare(b.DetectedAt) })
	if d.calls, err = s.store.AICalls(ctx); err != nil {
		return nil, false, err
	}
	if d.releases, err = s.store.AllReleases(ctx); err != nil {
		return nil, false, err
	}
	if d.notified, err = s.store.Notified(ctx); err != nil {
		return nil, false, err
	}
	states, err := s.store.AllRepoStates(ctx)
	if err != nil {
		return nil, false, err
	}
	if d.levels, err = s.releaseLevels(ctx, d.releases, states); err != nil {
		return nil, false, err
	}
	for key, rels := range d.releases {
		m := make(map[string]store.Release, len(rels))
		for _, r := range rels {
			m[r.Tag] = r
		}
		d.byTag[key] = m
	}

	list, err := s.source.Containers(ctx)
	partial := err != nil
	if partial {
		s.log.Warn("stats: listing containers failed", "err", err)
	} else {
		reviews := s.reviews(ctx)
		d.current = make([]currentContainer, 0, len(list))
		for _, c := range list {
			ref, _ := imageref.Parse(c.ImageRef)
			rs := s.rulesFor(c)
			d.current = append(d.current, currentContainer{c: c, ref: ref, rs: rs, json: s.toContainerJSON(ctx, c, reviews)})
			d.stackOf[c.Name] = c.Project
			if rs.Ignore {
				continue
			}
			if repo, ok := s.notesRepo(c, ref, rs); ok {
				d.repoOf[c.Name] = repo
			}
		}
	}
	d.coveredFrom = map[string]time.Time{}
	for _, repo := range d.repoOf {
		key := repo.Key()
		if _, done := d.coveredFrom[key]; done {
			continue
		}
		st, ok := states[key]
		var from time.Time
		if ok && st.HasMore {
			// The oldest stored release's month is only partly there, so coverage starts after it.
			if oldest, ok := oldestPublished(d.releases[key]); ok {
				from = monthOf(oldest).AddDate(0, 1, 0)
			}
		}
		d.coveredFrom[key] = from
	}
	d.applied = s.appliedUpdates(ctx, d)
	return d, partial, nil
}

// appliedInfo is an applied update (a history row with a previous image), measured
// against the stored notes of the container's repo.
type appliedInfo struct {
	h     store.HistoryEntry
	stack string
	// toUpdate: detected − the new version's publication; behind: detected − the first
	// release newer than the old version. nil without the notes, for a same-version rebuild
	// (its release can be months old), and when the image was seen before that release was
	// published (images often land before their release notes), which would otherwise read
	// as an instant update.
	toUpdate, behind *time.Duration
	releases         int  // releases after from up to and including to (0 without notes)
	partial          bool // the stored notes stop before from, so releases is a lower bound
	class            string
	// level: the update as the dashboard scores it from the stored notes (heuristics only:
	// AI verdicts are per current range); None without notes.
	level   analysis.Level
	rebuild bool // the same version again (a new image under the same tag)
	removed bool // the container is no longer on the host (false when Docker couldn't be listed)
}

func (i appliedInfo) update(ms int64) *appliedUpdateJSON {
	return &appliedUpdateJSON{Name: i.h.ContainerName, From: i.h.PrevRunning, To: i.h.Running, At: i.h.DetectedAt.UTC(), Ms: ms}
}

func (s *Server) appliedUpdates(ctx context.Context, d *statsData) []appliedInfo {
	var out []appliedInfo
	for _, h := range d.history {
		if h.PrevImageID == "" {
			continue
		}
		stack, here := d.stackOf[h.ContainerName]
		info := appliedInfo{h: h, stack: stack, removed: d.current != nil && !here, class: string(version.Digest), rebuild: h.PrevRunning == h.Running}
		from, to := parseVersion(h.PrevRunning), parseVersion(h.Running)
		if from != nil && to != nil {
			if c := version.Classify(*from, *to); c != version.None {
				info.class = string(c)
			} else {
				info.rebuild = true
			}
		}
		if repo, ok := d.repoOf[h.ContainerName]; ok && to != nil && !info.rebuild {
			ch, err := s.changelogs.Changes(ctx, repo, from, to)
			if err != nil {
				s.log.Debug("stats: stored release notes unavailable", "repo", repo.Key(), "err", err)
			} else {
				if rel, ok := d.byTag[repo.Key()][ch.ToTag]; ok && ch.ToTag != "" && !rel.PublishedAt.IsZero() {
					info.toUpdate = gap(rel.PublishedAt, h.DetectedAt)
				}
				if from != nil {
					info.releases = len(ch.Releases)
					info.partial = notesStopBefore(ch)
					if first, ok := oldestPublished(ch.Releases); ok {
						info.behind = gap(first, h.DetectedAt)
					}
				}
				info.level = assess(info.class, from, to, ch).Level
			}
		}
		out = append(out, info)
	}
	return out
}

// gap is to − from, nil when to is before from.
func gap(from, to time.Time) *time.Duration {
	g := to.Sub(from)
	if g < 0 {
		return nil
	}
	return &g
}

// behindBy is how far an update is behind in the stored notes: the releases between running
// and latest, and when the oldest of them came out (nil without a date); partial when the
// notes stop before running, so both are lower bounds. Zero for channel tags and versions
// that don't parse.
func (s *Server) behindBy(ctx context.Context, repo source.Repo, u *updateJSON) (n int, since *time.Time, partial bool) {
	running, latest := parseVersion(u.Running), parseVersion(u.Latest)
	if running == nil || latest == nil || u.Channel != "" || s.changelogs == nil {
		return 0, nil, false
	}
	ch, err := s.changelogs.Changes(ctx, repo, running, latest)
	if err != nil {
		return 0, nil, false
	}
	if first, ok := oldestPublished(ch.Releases); ok {
		first = first.UTC()
		since = &first
	}
	return len(ch.Releases), since, notesStopBefore(ch)
}

// notesStopBefore: older releases exist that weren't fetched and every stored one is newer
// than the running version (so its tag isn't among them either): the releases in between
// are missing from ch.Releases.
func notesStopBefore(ch changelog.Changes) bool { return ch.Partial && ch.FromTag == "" }

func oldestPublished(rels []store.Release) (time.Time, bool) {
	var oldest time.Time
	for _, r := range rels {
		if !r.PublishedAt.IsZero() && (oldest.IsZero() || r.PublishedAt.Before(oldest)) {
			oldest = r.PublishedAt
		}
	}
	return oldest, !oldest.IsZero()
}

// ── behind & updates ─────────────────────────────────────

func (s *Server) behindStats(ctx context.Context, d *statsData) behindJSON {
	out := behindJSON{Containers: []behindContainerJSON{}, Tide: []tideJSON{}}
	for _, cc := range d.current {
		u := cc.json.Update
		if cc.rs.Ignore || u == nil || u.CheckedAt == nil || u.Class == string(version.None) {
			continue
		}
		b := behindContainerJSON{
			Name: cc.c.Name, Stack: cc.c.Project, Icon: cc.json.Icon,
			Running: u.Running, Latest: u.Latest, Class: u.Class, Level: u.Level,
		}
		if cc.json.Review != nil {
			b.Review = cc.json.Review.Kind
		}
		if repo, ok := d.repoOf[cc.c.Name]; ok {
			b.ReleasesBehind, b.BehindSince, b.ReleasesPartial = s.behindBy(ctx, repo, u)
		}
		out.Containers = append(out.Containers, b)
	}
	// Longest behind first; those without a date last, by name.
	slices.SortStableFunc(out.Containers, func(a, b behindContainerJSON) int {
		switch {
		case a.BehindSince == nil && b.BehindSince == nil:
			return cmp.Compare(a.Name, b.Name)
		case a.BehindSince == nil:
			return 1
		case b.BehindSince == nil:
			return -1
		}
		return cmp.Or(a.BehindSince.Compare(*b.BehindSince), cmp.Compare(a.Name, b.Name))
	})

	out.Applied = appliedStats(d)
	out.TimeToUpdate = timeToUpdate(d)
	out.FromMessage = fromMessage(d)

	var tide []store.RunStats
	for _, r := range d.runStats {
		if d.inRange(r.At) {
			tide = append(tide, r)
		}
	}
	tide = thin(tide, func(r store.RunStats) time.Time { return r.At }, func(_, _ store.RunStats) bool { return true })
	for _, r := range tide {
		out.Tide = append(out.Tide, toTide(r))
	}
	if len(d.runStats) > 0 {
		now := toTide(d.runStats[len(d.runStats)-1])
		out.Now = &now
	}
	return out
}

func toTide(r store.RunStats) tideJSON {
	return tideJSON{At: r.At.UTC(), Checked: r.Checked, UpToDate: r.UpToDate, Updates: r.Updates, Attention: r.Attention, Breaking: r.Breaking, ReleasesBehind: r.ReleasesBehind}
}

func appliedStats(d *statsData) appliedJSON {
	out := appliedJSON{PerStack: []stackCountJSON{}, PerWeek: []weekCountJSON{}}
	type stackKey struct {
		stack   string
		removed bool
	}
	perStack, perWeek := map[stackKey]int{}, map[string]int{}
	for _, a := range d.applied {
		if !d.inRange(a.h.DetectedAt) {
			continue
		}
		out.Total++
		perStack[stackKey{a.stack, a.removed}]++
		perWeek[weekOf(a.h.DetectedAt)]++
	}
	for k, n := range perStack {
		out.PerStack = append(out.PerStack, stackCountJSON{Stack: k.stack, Removed: k.removed, Count: n})
	}
	slices.SortFunc(out.PerStack, func(a, b stackCountJSON) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), compareBool(a.Removed, b.Removed), cmp.Compare(a.Stack, b.Stack))
	})

	// Every week in range, zeros included; "all" starts at the first history row.
	start := d.since
	if start.IsZero() {
		if len(d.history) == 0 {
			return out
		}
		start = d.history[0].DetectedAt
	}
	for wk := mondayOf(start); !wk.After(d.now); wk = wk.AddDate(0, 0, 7) {
		key := wk.Format(time.DateOnly)
		out.PerWeek = append(out.PerWeek, weekCountJSON{Week: key, Count: perWeek[key]})
	}
	return out
}

// timeToUpdate is the median time to update overall, per class (patch, minor, major; a
// digest or unparseable update only counts overall) and per level.
func timeToUpdate(d *statsData) timeToUpdateJSON {
	var out timeToUpdateJSON
	var ms []int64
	byClass, byLevel := map[string][]int64{}, map[string][]int64{}
	for _, a := range d.applied {
		if !d.inRange(a.h.DetectedAt) || a.toUpdate == nil {
			continue
		}
		v := a.toUpdate.Milliseconds()
		ms = append(ms, v)
		byClass[a.class] = append(byClass[a.class], v)
		byLevel[a.level.String()] = append(byLevel[a.level.String()], v)
	}
	out.Count = len(ms)
	out.MedianMs = median(ms)
	kinds := func(by map[string][]int64, order ...string) []kindMedianJSON {
		list := make([]kindMedianJSON, 0, len(order))
		for _, k := range order {
			list = append(list, kindMedianJSON{Kind: k, Count: len(by[k]), MedianMs: median(by[k])})
		}
		return list
	}
	out.ByClass = kinds(byClass, string(version.Patch), string(version.Minor), string(version.Major))
	out.ByLevel = kinds(byLevel, analysis.None.String(), analysis.Attention.String(), analysis.Breaking.String())
	return out
}

// fromMessage is the median time from the message about an update to when that version was
// seen running. notified_at is the last send (a resend, say after a snooze ended, overwrites
// it), so this measures from the last message. A message sent after the update was seen
// isn't what led to it; rebuilds are left out (their version's message was for the update
// before them).
func fromMessage(d *statsData) medianJSON {
	var ms []int64
	for _, a := range d.applied {
		if !d.inRange(a.h.DetectedAt) || a.rebuild {
			continue
		}
		n, ok := d.notified[store.NotifiedKey{Container: a.h.ContainerName, Key: a.h.Running}]
		if !ok || n.NotifiedAt.After(a.h.DetectedAt) {
			continue
		}
		ms = append(ms, a.h.DetectedAt.Sub(n.NotifiedAt).Milliseconds())
	}
	return medianJSON{Count: len(ms), MedianMs: median(ms)}
}

// ── upstream ─────────────────────────────────────────────

func (s *Server) upstreamStats(d *statsData) upstreamJSON {
	out := upstreamJSON{Repos: []repoStatsJSON{}, PerMonth: []monthReleasesJSON{}, BiggestJumps: []jumpJSON{}}

	repos := map[string]source.Repo{}
	names := map[string][]string{}
	for name, repo := range d.repoOf {
		repos[repo.Key()] = repo
		names[repo.Key()] = append(names[repo.Key()], name)
	}
	type dated struct {
		at    time.Time
		level analysis.Level
		key   string
	}
	var all []dated // every dated release of these repos
	for key, repo := range repos {
		rels := d.releases[key]
		if len(rels) == 0 {
			continue
		}
		slices.Sort(names[key])
		rj := repoStatsJSON{Repo: repo.Path, URL: repo.URL(), Containers: names[key]}
		var times []time.Time
		for _, r := range rels {
			lvl := d.levels[key][r.Tag]
			if !r.PublishedAt.IsZero() {
				all = append(all, dated{r.PublishedAt, lvl, key})
				if rj.LastRelease == nil || r.PublishedAt.After(*rj.LastRelease) {
					t := r.PublishedAt.UTC()
					rj.LastRelease = &t
				}
			}
			// Undated sections (a CHANGELOG.md) only count when the range is everything.
			if r.PublishedAt.IsZero() && !d.since.IsZero() || !r.PublishedAt.IsZero() && !d.inRange(r.PublishedAt) {
				continue
			}
			rj.Releases++
			switch lvl {
			case analysis.Breaking:
				rj.Breaking++
			case analysis.Attention:
				rj.Attention++
			}
			if !r.PublishedAt.IsZero() {
				times = append(times, r.PublishedAt)
			}
		}
		rj.MedianGapDays = medianGapDays(times)
		rj.Quiet = rj.LastRelease != nil && rj.LastRelease.Before(d.now.AddDate(0, -quietAfterMonths, 0))
		if from := d.coveredFrom[key]; !from.IsZero() && (d.since.IsZero() || d.since.Before(from)) {
			rj.Partial = true
		}
		out.Repos = append(out.Repos, rj)
	}
	slices.SortFunc(out.Repos, func(a, b repoStatsJSON) int {
		return cmp.Or(cmp.Compare(b.Releases, a.Releases), cmp.Compare(a.Repo, b.Repo))
	})

	// Per month, every month in range; "all" starts at the oldest release, at most
	// maxReleaseMonths back.
	start := d.since
	if start.IsZero() {
		for _, r := range all {
			if start.IsZero() || r.at.Before(start) {
				start = r.at
			}
		}
		if limit := monthOf(d.now).AddDate(0, 1-maxReleaseMonths, 0); !start.IsZero() && start.Before(limit) {
			start = limit
		}
	}
	if !start.IsZero() {
		idx := map[string]int{}
		for m := monthOf(start); !m.After(d.now); m = m.AddDate(0, 1, 0) {
			idx[m.Format("2006-01")] = len(out.PerMonth)
			out.PerMonth = append(out.PerMonth, monthReleasesJSON{Month: m.Format("2006-01")})
		}
		// A month only counts the repos whose notes cover all of it, so older months aren't
		// lower just because fast-moving repos' stored notes don't reach back that far.
		for key := range repos {
			if len(d.releases[key]) == 0 {
				continue
			}
			from := d.coveredFrom[key]
			for i := range out.PerMonth {
				if m, _ := time.Parse("2006-01", out.PerMonth[i].Month); m.Before(from) {
					out.PerMonth[i].Partial = true
				}
			}
		}
		for _, r := range all {
			i, ok := idx[r.at.UTC().Format("2006-01")]
			if !ok || !d.inRange(r.at) || r.at.Before(d.coveredFrom[r.key]) {
				continue
			}
			pm := &out.PerMonth[i]
			pm.Releases++
			switch r.level {
			case analysis.Breaking:
				pm.Breaking++
			case analysis.Attention:
				pm.Attention++
			}
		}
	}

	// Applied updates that skipped the most releases at once.
	for _, a := range d.applied {
		if !d.inRange(a.h.DetectedAt) || a.releases == 0 {
			continue
		}
		var ms int64
		if a.toUpdate != nil {
			ms = a.toUpdate.Milliseconds()
		}
		out.BiggestJumps = append(out.BiggestJumps, jumpJSON{appliedUpdateJSON: *a.update(ms), Releases: a.releases, Class: a.class, Partial: a.partial})
	}
	slices.SortStableFunc(out.BiggestJumps, func(a, b jumpJSON) int { return cmp.Or(cmp.Compare(b.Releases, a.Releases), b.At.Compare(a.At)) })
	out.BiggestJumps = out.BiggestJumps[:min(len(out.BiggestJumps), topJumps)]
	return out
}

// medianGapDays is the median number of days between consecutive releases, one decimal;
// nil with fewer than two.
func medianGapDays(times []time.Time) *float64 {
	if len(times) < 2 {
		return nil
	}
	slices.SortFunc(times, func(a, b time.Time) int { return a.Compare(b) })
	gaps := make([]float64, 0, len(times)-1)
	for i := 1; i < len(times); i++ {
		gaps = append(gaps, times[i].Sub(times[i-1]).Hours()/24)
	}
	slices.Sort(gaps)
	m := gaps[len(gaps)/2]
	if len(gaps)%2 == 0 {
		m = (gaps[len(gaps)/2-1] + m) / 2
	}
	m = math.Round(m*10) / 10
	return &m
}

// releaseLevels is every stored release's level on its own, by repo key and tag, as the
// detail page flags its notes (without a running release whose repeated lines it would
// skip). A repo's levels are kept until it's fetched again (its fetched_at changes) or a
// release shows up that they don't cover (a sync that failed partway keeps the pages it got),
// so only then are its bodies read. Repos no longer stored drop out of the cache.
func (s *Server) releaseLevels(ctx context.Context, releases map[string][]store.Release, states map[string]store.RepoState) (map[string]map[string]analysis.Level, error) {
	s.levelsMu.Lock()
	defer s.levelsMu.Unlock()
	next := make(map[string]repoLevels, len(releases))
	for key, rels := range releases {
		fetched := states[key].FetchedAt
		if c, ok := s.levels[key]; ok && c.fetchedAt.Equal(fetched) && covers(c.byTag, rels) {
			next[key] = c
			continue
		}
		full, err := s.store.Releases(ctx, key)
		if err != nil {
			return nil, err
		}
		c := repoLevels{fetchedAt: fetched, byTag: make(map[string]analysis.Level, len(full))}
		for _, r := range full {
			c.byTag[r.Tag] = analysis.Analyze(r.Body, nil).Level
		}
		next[key] = c
	}
	s.levels = next
	out := make(map[string]map[string]analysis.Level, len(next))
	for key, c := range next {
		out[key] = c.byTag
	}
	return out, nil
}

// covers says whether levels has every release in rels.
func covers(levels map[string]analysis.Level, rels []store.Release) bool {
	for _, r := range rels {
		if _, ok := levels[r.Tag]; !ok {
			return false
		}
	}
	return true
}

type repoLevels struct {
	fetchedAt time.Time // the repo's fetched_at when the levels were computed
	byTag     map[string]analysis.Level
}

// ── check health ─────────────────────────────────────────

func (s *Server) healthStats(d *statsData) healthJSON {
	out := healthJSON{Durations: []durationJSON{}, Failing: []failingJSON{}, FailuresByKind: []kindCountJSON{}, Requests: []requestDayJSON{}, RequestsPer: "day"}

	var finished []store.Run
	var ms []int64
	for _, r := range d.runs {
		if !d.inRange(r.StartedAt) {
			continue
		}
		out.Runs.Total++
		if r.Error != "" {
			out.Runs.Failed++
		}
		switch r.Trigger {
		case "startup":
			out.Runs.ByTrigger.Startup++
		case "schedule":
			out.Runs.ByTrigger.Schedule++
		case "manual":
			out.Runs.ByTrigger.Manual++
		}
		if r.FinishedAt != nil {
			finished = append(finished, r)
			ms = append(ms, runMs(r))
		}
	}
	out.Runs.MedianMs = median(ms)
	if len(ms) > 0 {
		slowest := slices.Max(ms)
		out.Runs.SlowestMs = &slowest
	}
	finished = thin(finished, func(r store.Run) time.Time { return *r.FinishedAt }, func(cur, next store.Run) bool { return runMs(next) > runMs(cur) })
	for _, r := range finished {
		out.Durations = append(out.Durations, durationJSON{At: r.FinishedAt.UTC(), Ms: runMs(r), Images: r.Images, Failed: r.Error != ""})
	}

	for _, cc := range d.current {
		u := cc.json.Update
		if cc.rs.Ignore || u == nil || u.Error == "" {
			continue
		}
		f := failingJSON{Name: cc.c.Name, Kind: u.ErrorKind, Error: u.Error}
		if u.FailedAt != nil {
			f.Since = u.FailedAt.UTC()
		}
		out.Failing = append(out.Failing, f)
	}
	slices.SortFunc(out.Failing, func(a, b failingJSON) int { return cmp.Compare(a.Name, b.Name) })

	kinds := map[string]int{}
	days := map[string]*requestDayJSON{}
	var firstDay time.Time
	for _, r := range d.runStats {
		if d.inRange(r.At) {
			firstDay = dayOf(r.At)
			break
		}
	}
	// Per day, or per week (keyed by its Monday) when the span is too long for daily bars.
	bucket, step := dayOf, 1
	if !firstDay.IsZero() && d.now.Sub(firstDay) > maxRequestDays*24*time.Hour {
		bucket, step, firstDay = mondayOf, 7, mondayOf(firstDay)
		out.RequestsPer = "week"
	}
	for _, r := range d.runStats {
		if !d.inRange(r.At) {
			continue
		}
		for k, n := range r.Failures {
			kinds[k] += n
		}
		key := bucket(r.At).Format(time.DateOnly)
		day := days[key]
		if day == nil {
			day = &requestDayJSON{Day: key}
			days[key] = day
		}
		addRequests(day, r.Requests)
	}
	for k, n := range kinds {
		if n > 0 {
			out.FailuresByKind = append(out.FailuresByKind, kindCountJSON{Kind: k, Count: n})
		}
	}
	slices.SortFunc(out.FailuresByKind, func(a, b kindCountJSON) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Kind, b.Kind))
	})
	if !firstDay.IsZero() {
		for day := firstDay; !day.After(d.now); day = day.AddDate(0, 0, step) {
			key := day.Format(time.DateOnly)
			rd := requestDayJSON{Day: key}
			if v := days[key]; v != nil {
				rd = *v
			}
			out.Requests = append(out.Requests, rd)
			out.RequestTotals.Registry += rd.Registry
			out.RequestTotals.GitHub += rd.GitHub
			out.RequestTotals.GitLab += rd.GitLab
			out.RequestTotals.Limited += rd.Limited
		}
	}

	if s.changelogs != nil {
		out.GitHub.Token = s.changelogs.HasToken()
		if q, ok := s.changelogs.LastQuota(); ok {
			out.GitHub.Limit, out.GitHub.Remaining = &q.Limit, &q.Remaining
			if !q.ResetAt.IsZero() {
				reset := q.ResetAt.UTC()
				out.GitHub.ResetAt = &reset
			}
		}
	}
	return out
}

func addRequests(day *requestDayJSON, reqs map[string]stats.Count) {
	for group, c := range reqs {
		switch group {
		case "registry":
			day.Registry += c.Requests
		case "github":
			day.GitHub += c.Requests
		case "gitlab":
			day.GitLab += c.Requests
		}
		day.Limited += c.Limited
	}
}

func runMs(r store.Run) int64 { return r.FinishedAt.Sub(r.StartedAt).Milliseconds() }

// ── AI usage ─────────────────────────────────────────────

func aiStats(d *statsData) *aiStatsJSON {
	if len(d.calls) == 0 {
		return nil
	}
	out := &aiStatsJSON{Months: []aiMonthJSON{}, ByModel: []aiModelJSON{}}
	start := d.since
	if start.IsZero() {
		start = d.calls[0].At
	}
	idx := map[string]int{}
	for m := monthOf(start); !m.After(d.now); m = m.AddDate(0, 1, 0) {
		idx[m.Format("2006-01")] = len(out.Months)
		out.Months = append(out.Months, aiMonthJSON{Month: m.Format("2006-01")})
	}
	models := map[[2]string]*aiModelJSON{}
	var ms []int64
	for _, c := range d.calls {
		if !d.inRange(c.At) {
			continue
		}
		failed := c.Error != ""
		out.Calls++
		switch {
		case c.Trigger == "manual":
			out.Manual++
		case c.Trigger == "auto":
			out.Auto++
		}
		if c.Mode == "extended" {
			out.Extended++
		}
		m := models[[2]string{c.Provider, c.Model}]
		if m == nil {
			m = &aiModelJSON{Provider: c.Provider, Model: c.Model}
			models[[2]string{c.Provider, c.Model}] = m
		}
		m.Calls++
		m.InputTokens += c.InputTokens
		m.OutputTokens += c.OutputTokens
		if i, found := idx[c.At.UTC().Format("2006-01")]; found {
			mo := &out.Months[i]
			mo.Calls++
			mo.InputTokens += c.InputTokens
			mo.OutputTokens += c.OutputTokens
			if failed {
				mo.Failed++
			}
		}
		if failed {
			out.Failed++
			m.Failed++
			continue
		}
		ms = append(ms, c.Duration.Milliseconds())
	}
	for _, m := range models {
		out.ByModel = append(out.ByModel, *m)
	}
	slices.SortFunc(out.ByModel, func(a, b aiModelJSON) int {
		return cmp.Or(cmp.Compare(b.Calls, a.Calls), cmp.Compare(a.Provider, b.Provider), cmp.Compare(a.Model, b.Model))
	})
	out.MedianMs = median(ms)
	return out
}

// ── lifetime ─────────────────────────────────────────────

func (s *Server) lifetimeStats(d *statsData) lifetimeJSON {
	out := lifetimeJSON{StartedAt: s.started.UTC()}
	if len(d.runs) > 0 {
		first := d.runs[0].StartedAt.UTC()
		out.Since = &first
	}
	names := map[string]bool{}
	for _, h := range d.history {
		names[h.ContainerName] = true
	}
	out.ContainersTracked = len(names)
	for key, rels := range d.releases {
		out.ReleasesStored += len(rels)
		for _, r := range rels {
			if d.levels[key][r.Tag] == analysis.Breaking {
				out.BreakingFlagged++
			}
		}
	}

	for _, a := range d.applied {
		if a.toUpdate != nil {
			if v := a.toUpdate.Milliseconds(); out.FastestUpdate == nil || v <= out.FastestUpdate.Ms {
				out.FastestUpdate = a.update(v)
			}
		}
		if a.behind != nil {
			if v := a.behind.Milliseconds(); out.LongestBehind == nil || v >= out.LongestBehind.Ms {
				out.LongestBehind = a.update(v)
			}
		}
	}
	out.LongestStreak = longestStreak(d.runStats)
	return out
}

// longestStreak is the longest run of consecutive rows without open updates (the most
// recent one on a tie); nil without such a row.
func longestStreak(rows []store.RunStats) *streakJSON {
	var best *streakJSON
	start := -1
	for i, r := range rows {
		// A run that checked nothing (say every check failed) says nothing about being up to date.
		if r.Updates != 0 || r.Checked == 0 {
			start = -1
			continue
		}
		if start < 0 {
			start = i
		}
		if n := i - start + 1; best == nil || n >= best.Runs {
			best = &streakJSON{Runs: n, From: rows[start].At.UTC(), To: r.At.UTC()}
		}
	}
	return best
}

// ── helpers ──────────────────────────────────────────────

// thin keeps one item per UTC day when there are more than maxSeriesPoints, and per week,
// then per month, while there are still too many: the first of each, replaced by a later
// one whenever replace(kept, later) says so. Items are oldest first.
func thin[T any](items []T, at func(T) time.Time, replace func(kept, later T) bool) []T {
	buckets := []func(time.Time) string{
		func(t time.Time) string { return t.UTC().Format(time.DateOnly) },
		weekOf,
		func(t time.Time) string { return t.UTC().Format("2006-01") },
	}
	for _, bucket := range buckets {
		if len(items) <= maxSeriesPoints {
			break
		}
		var out []T
		var cur string
		for _, it := range items {
			b := bucket(at(it))
			switch {
			case len(out) == 0 || b != cur:
				out = append(out, it)
				cur = b
			case replace(out[len(out)-1], it):
				out[len(out)-1] = it
			}
		}
		items = out
	}
	return items
}

// compareBool orders false before true.
func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// median of ms, the mean of the middle two for an even count; nil when empty.
func median(ms []int64) *int64 {
	if len(ms) == 0 {
		return nil
	}
	s := slices.Clone(ms)
	slices.Sort(s)
	m := s[len(s)/2]
	if len(s)%2 == 0 {
		m = (s[len(s)/2-1] + m) / 2
	}
	return &m
}

func dayOf(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// mondayOf is the start of t's ISO week (Monday, UTC).
func mondayOf(t time.Time) time.Time {
	d := dayOf(t)
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

func weekOf(t time.Time) string { return mondayOf(t).Format(time.DateOnly) }

func monthOf(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

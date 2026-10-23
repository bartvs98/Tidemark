package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/stats"
	"github.com/bartvs98/tidemark/internal/store"
)

const day = 24 * time.Hour

// statsServer builds a server over src with a store, checks (majorRegistry: 1.0.0, 1.2.0,
// 2.0.0) and a changelog service that never fetches (it only reads stored notes here).
func statsServer(t *testing.T, src docker.Source) (*Server, *check.Service, *store.Store) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	checks := check.New(src, majorRegistry{}, db, log)
	srv := New(Deps{
		Version: "t", Log: log, Source: src, Checks: checks, Store: db,
		Changelogs: changelog.New(db, "", log, changelog.WithBaseURLs("http://127.0.0.1:1", "http://127.0.0.1:1", "http://127.0.0.1:1")),
		Started:    time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC),
	})
	return srv, checks, db
}

func runChecks(t *testing.T, checks *check.Service) {
	t.Helper()
	checks.Start(t.Context(), check.TriggerManual)
	for deadline := time.Now().Add(5 * time.Second); checks.Status().Running && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
}

func getStats(t *testing.T, h http.Handler, rng string) statsJSON {
	t.Helper()
	target := "/api/stats"
	if rng != "" {
		target += "?range=" + rng
	}
	rec := do(t, h, target)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", target, rec.Code, rec.Body)
	}
	var out statsJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func ctrApp(name, tag string, labels map[string]string) docker.Container {
	return docker.Container{ID: name, Name: name, ImageRef: "team/app:" + tag, Image: docker.Image{ID: "sha256:" + tag}, Labels: labels}
}

func TestStatsRangeParsing(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if rec := do(t, New(Deps{Log: log, Source: staticSource{}}).Handler(fstest.MapFS{}), "/api/stats"); rec.Code != http.StatusNotImplemented {
		t.Errorf("without a store = %d", rec.Code)
	}

	srv, _, _ := statsServer(t, staticSource{})
	h := srv.Handler(fstest.MapFS{})
	for _, bad := range []string{"7d", "ALL", "90"} {
		rec := do(t, h, "/api/stats?range="+bad)
		var body map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != http.StatusBadRequest || body["error"] == "" {
			t.Errorf("range=%s: %d %v", bad, rec.Code, body)
		}
	}
	for rng, want := range map[string]time.Duration{"": 90 * day, "30d": 30 * day, "90d": 90 * day, "1y": 365 * day} {
		out := getStats(t, h, rng)
		if out.Range != cmpOr(rng, "90d") || out.Since == nil {
			t.Fatalf("range %q: %s since %v", rng, out.Range, out.Since)
		}
		if got := out.GeneratedAt.Sub(*out.Since); got != want {
			t.Errorf("range %q spans %v, want %v", rng, got, want)
		}
	}
	if out := getStats(t, h, "all"); out.Since != nil || out.Range != "all" {
		t.Errorf("all: %s since %v", out.Range, out.Since)
	}

	// An empty database: every array is [] (never null), optional parts are left out.
	rec := do(t, h, "/api/stats?range=all")
	if body := rec.Body.String(); strings.Contains(body, "null") || strings.Contains(body, `"ai"`) ||
		strings.Contains(body, `"partial"`) || strings.Contains(body, "longestStreak") {
		t.Errorf("empty stats = %s", body)
	}
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// The run snapshot counts like the dashboard's hero: ignored containers only count as
// such, reviewed and snoozed updates aren't open, snoozed ones are still checked.
func TestRunStatsCounts(t *testing.T) {
	src := staticSource{
		ctrApp("ignored", "1.0.0", map[string]string{"tidemark.ignore": "true"}),
		ctrApp("current", "2.0.0", nil),
		ctrApp("open", "1.0.0", map[string]string{"tidemark.repo": "github.com/team/notes"}),
		ctrApp("reviewed", "1.2.0", nil),
		ctrApp("snoozed", "1.0.0", nil),
		ctrApp("held", "1.0.0", map[string]string{"tidemark.allow-major": "false"}), // 1.0.0 → 1.2.0, minor
		{ID: "gone", Name: "missing", ImageRef: "team/app:1.0.0", Image: docker.Image{ID: "sha256:x", Missing: true}},
	}
	src[2].Project = "web"
	srv, checks, db := statsServer(t, src)
	runChecks(t, checks)
	ctx := context.Background()
	now := time.Now()
	_ = db.SaveReview(ctx, store.Review{ContainerName: "reviewed", Kind: "reviewed", Version: "2.0.0", CreatedAt: now})
	_ = db.SaveReview(ctx, store.Review{ContainerName: "snoozed", Kind: "snoozed", Until: now.Add(day), CreatedAt: now})
	_ = db.SaveReview(ctx, store.Review{ContainerName: "open", Kind: "reviewed", Version: "1.2.0", CreatedAt: now}) // older: no longer applies
	// Notes for open's repo only: it's 3 releases behind (1.0.0 → 2.0.0), the update debt.
	if err := db.SaveReleases(ctx, "github.com/team/notes", []store.Release{
		{Tag: "v1.0.0"}, {Tag: "v1.1.0"}, {Tag: "v1.2.0"}, {Tag: "v2.0.0"},
	}, false); err != nil {
		t.Fatal(err)
	}

	got, err := srv.RunStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := store.RunStats{
		Containers: 7, Ignored: 1, Checked: 5, UpToDate: 1,
		Updates: 2, Breaking: 1, Reviewed: 1, Snoozed: 1, Failed: 1, ReleasesBehind: 3,
		Classes:  map[string]int{"major": 3, "minor": 1},
		Failures: map[string]int{"other": 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("run stats =\n %+v\nwant\n %+v", got, want)
	}

	// The endpoint's current parts agree.
	out := getStats(t, srv.Handler(fstest.MapFS{}), "")
	behind := map[string]behindContainerJSON{}
	for _, b := range out.Behind.Containers {
		behind[b.Name] = b
	}
	if len(behind) != 4 || behind["open"].Review != "" || behind["open"].Level != analysis.Breaking || behind["open"].Stack != "web" ||
		behind["reviewed"].Review != "reviewed" || behind["snoozed"].Review != "snoozed" || behind["held"].Class != "minor" {
		t.Errorf("behind = %+v", out.Behind.Containers)
	}
	if f := out.Health.Failing; len(f) != 1 || f[0].Name != "missing" || f[0].Kind != "" || f[0].Error == "" || f[0].Since.IsZero() {
		t.Errorf("failing = %+v", f)
	}
	if out.Partial {
		t.Error("partial with Docker reachable")
	}
}

func TestStatsDockerUnreachable(t *testing.T) {
	srv, _, db := statsServer(t, failingSource{})
	ctx := context.Background()
	now := time.Now()
	_ = db.AddHistory(ctx, store.HistoryEntry{ContainerName: "app", ImageID: "1", Running: "1.0.0", DetectedAt: now.Add(-2 * day)})
	_ = db.AddHistory(ctx, store.HistoryEntry{ContainerName: "app", ImageID: "2", Running: "1.2.0", PrevImageID: "1", PrevRunning: "1.0.0", DetectedAt: now.Add(-day)})

	rec := do(t, srv.Handler(fstest.MapFS{}), "/api/stats")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "null") {
		t.Errorf("null in %s", rec.Body)
	}
	var out statsJSON
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !out.Partial || len(out.Behind.Containers) != 0 || len(out.Upstream.Repos) != 0 || len(out.Health.Failing) != 0 {
		t.Errorf("partial stats = %+v", out)
	}
	// What the store has is still there; the stack of a container Docker didn't list is ''.
	if a := out.Behind.Applied; a.Total != 1 || !reflect.DeepEqual(a.PerStack, []stackCountJSON{{Stack: "", Count: 1}}) {
		t.Errorf("applied = %+v", a)
	}
	if _, err := srv.RunStats(ctx); err == nil {
		t.Error("RunStats without Docker didn't fail")
	}
}

// A seeded history: release notes, applied updates, check runs, run stats and AI calls.
func TestStatsFromHistory(t *testing.T) {
	repoLabel := map[string]string{"tidemark.repo": "github.com/team/app"}
	app := ctrApp("app", "2.0.0", repoLabel)
	app.Project = "web"
	srv, _, db := statsServer(t, staticSource{app})
	h := srv.Handler(fstest.MapFS{})
	ctx := context.Background()
	now := time.Now().UTC()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }

	if err := db.SaveReleases(ctx, "github.com/team/app", []store.Release{
		{Tag: "v1.0.0", PublishedAt: ago(120 * day)},
		{Tag: "v1.1.0", Body: "## Breaking\nnew config", PublishedAt: ago(60 * day)},
		{Tag: "v1.2.0", Body: "fixes", PublishedAt: ago(25 * day)},
		{Tag: "v2.0.0", Body: "more", PublishedAt: ago(4 * day)},
	}, false); err != nil {
		t.Fatal(err)
	}
	for _, he := range []store.HistoryEntry{
		{ContainerName: "app", ImageID: "1", Running: "1.0.0", DetectedAt: ago(100 * day)}, // tracking started
		{ContainerName: "app", ImageID: "2", Running: "1.2.0", PrevImageID: "1", PrevRunning: "1.0.0", DetectedAt: ago(20 * day)},
		{ContainerName: "gone", ImageID: "a", Running: "3", PrevImageID: "b", PrevRunning: "2", DetectedAt: ago(10 * day)},
		{ContainerName: "app", ImageID: "3", Running: "2.0.0", PrevImageID: "2", PrevRunning: "1.2.0", DetectedAt: ago(3 * day)},
	} {
		if err := db.AddHistory(ctx, he); err != nil {
			t.Fatal(err)
		}
	}
	addRun := func(trigger string, start time.Time, took time.Duration, images int, errMsg string) {
		id, err := db.StartRun(ctx, trigger, start)
		if err != nil {
			t.Fatal(err)
		}
		if took > 0 {
			_ = db.FinishRun(ctx, id, start.Add(took), images, errMsg)
		}
	}
	addRun("startup", ago(60*day), 9*time.Second, 3, "can't reach Docker") // out of the 30 days
	addRun("schedule", ago(20*day), 2*time.Second, 3, "")
	addRun("schedule", ago(10*day), 4*time.Second, 3, "")
	addRun("manual", ago(2*day), 3*time.Second, 3, "")
	addRun("manual", ago(time.Minute), 0, 0, "") // still running

	// 200 run_stats rows, 4 a day over 50 days: more than the series keeps.
	first := dayOf(ago(49 * day))
	var rows []store.RunStats
	for i := range 200 {
		r := store.RunStats{
			RunID: int64(i + 1), At: first.Add(time.Duration(i/4)*day + time.Duration(i%4)*5*time.Hour), Updates: 1, Checked: 3, UpToDate: i % 3,
			Requests: map[string]stats.Count{"registry": {Requests: 2}, "github": {Requests: 1}},
		}
		if i%50 == 0 {
			r.Requests["github"] = stats.Count{Requests: 1, Limited: 1}
			r.Failures = map[string]int{"rate-limit": 2, "network": 1}
		}
		if i >= 10 && i < 30 || i >= 100 && i < 105 {
			r.Updates = 0
		}
		rows = append(rows, r)
		if err := db.AddRunStats(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	out := getStats(t, h, "all")

	// Tide: one point per day, the day's last row.
	if tide := out.Behind.Tide; len(tide) != 50 || !tide[0].At.Equal(rows[3].At) || !tide[49].At.Equal(rows[199].At) || tide[0].UpToDate != rows[3].UpToDate {
		t.Errorf("tide: %d points, first %+v, last %+v", len(tide), tide[0], tide[len(tide)-1])
	}
	if s := out.Lifetime.LongestStreak; s == nil || s.Runs != 20 || !s.From.Equal(rows[10].At) || !s.To.Equal(rows[29].At) {
		t.Errorf("longest streak = %+v", s)
	}
	// Requests per day from the first row to today, zeros included.
	if r := out.Health.Requests; len(r) != 50 || r[0].Day != first.Format(time.DateOnly) || r[0].Registry != 8 || r[0].GitHub != 4 || r[0].Limited != 1 {
		t.Errorf("requests: %d days, first %+v", len(r), r[0])
	}
	if tot := out.Health.RequestTotals; tot != (requestTotalsJSON{Registry: 400, GitHub: 200, Limited: 4}) {
		t.Errorf("request totals = %+v", tot)
	}
	if k := out.Health.FailuresByKind; !reflect.DeepEqual(k, []kindCountJSON{{"rate-limit", 8}, {"network", 4}}) {
		t.Errorf("failures by kind = %+v", k)
	}

	// Applied updates and time to update, in the last 30 days.
	out = getStats(t, h, "30d")
	a := out.Behind.Applied
	if a.Total != 3 || !reflect.DeepEqual(a.PerStack, []stackCountJSON{{"web", 2}, {"", 1}}) {
		t.Errorf("applied = %+v", a)
	}
	weeks, sum := a.PerWeek, 0
	for i, w := range weeks {
		sum += w.Count
		if wd := mustDate(t, w.Week).Weekday(); wd != time.Monday {
			t.Errorf("week %s is a %s", w.Week, wd)
		}
		if i > 0 && mustDate(t, w.Week).Sub(mustDate(t, weeks[i-1].Week)) != 7*day {
			t.Errorf("weeks not contiguous: %v", weeks)
		}
	}
	if len(weeks) < 5 || len(weeks) > 6 || sum != 3 || weeks[0].Week != weekOf(*out.Since) || weeks[len(weeks)-1].Week != weekOf(now) {
		t.Errorf("per week = %+v", weeks)
	}
	ttu := out.Behind.TimeToUpdate
	if ttu.Count != 2 || ttu.MedianMs == nil || *ttu.MedianMs != (3*day).Milliseconds() {
		t.Errorf("time to update = %+v", ttu)
	}
	if j := out.Upstream.BiggestJumps; len(j) != 2 || j[0].To != "1.2.0" || j[0].Releases != 2 || j[0].Class != "minor" ||
		j[1].To != "2.0.0" || j[1].Releases != 1 || j[1].Class != "major" {
		t.Errorf("biggest jumps = %+v", j)
	}
	runs := out.Health.Runs
	if runs.Total != 4 || runs.Failed != 0 || runs.ByTrigger != (triggersJSON{Schedule: 2, Manual: 2}) ||
		runs.MedianMs == nil || *runs.MedianMs != 3000 || runs.SlowestMs == nil || *runs.SlowestMs != 4000 {
		t.Errorf("runs = %+v", runs)
	}
	if d := out.Health.Durations; len(d) != 3 || d[0].Ms != 2000 || d[0].Images != 3 || d[0].Failed {
		t.Errorf("durations = %+v", d)
	}
	if r := out.Upstream.Repos; len(r) != 1 || r[0].Repo != "team/app" || r[0].URL != "https://github.com/team/app" ||
		!reflect.DeepEqual(r[0].Containers, []string{"app"}) || r[0].Releases != 2 || r[0].Breaking != 0 ||
		r[0].MedianGapDays == nil || *r[0].MedianGapDays != 21 || r[0].LastRelease == nil {
		t.Errorf("repos = %+v", r)
	}
	if out.AI != nil {
		t.Errorf("ai without calls = %+v", out.AI)
	}

	// Everything: release counts by level, months zero-filled from the oldest release.
	if err := db.AddAICall(ctx, store.AICall{At: ago(40 * day), Provider: "anthropic", Model: "m", Trigger: "auto", InputTokens: 100, OutputTokens: 10, Duration: 2 * time.Second}); err != nil {
		t.Fatal(err)
	}
	_ = db.AddAICall(ctx, store.AICall{At: ago(day), Provider: "anthropic", Model: "m", Trigger: "manual", Mode: "extended", Duration: 5 * time.Second})
	_ = db.AddAICall(ctx, store.AICall{At: ago(day), Provider: "openai", Model: "g", Trigger: "manual", Error: "timeout", Duration: time.Minute})
	out = getStats(t, h, "all")
	r := out.Upstream.Repos[0]
	if r.Releases != 4 || r.Breaking != 1 || r.Attention != 0 || *r.MedianGapDays != 35 {
		t.Errorf("all-time repo = %+v", r)
	}
	months, total := out.Upstream.PerMonth, 0
	for _, m := range months {
		total += m.Releases
	}
	if total != 4 || months[0].Month != ago(120*day).Format("2006-01") || months[len(months)-1].Month != now.Format("2006-01") || len(months) < 4 || len(months) > 6 {
		t.Errorf("per month = %+v", months)
	}
	lt := out.Lifetime
	if lt.Since == nil || lt.ContainersTracked != 2 || lt.ReleasesStored != 4 ||
		lt.BreakingFlagged != 1 || !lt.StartedAt.Equal(time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("lifetime = %+v", lt)
	}
	if lt.FastestUpdate == nil || lt.FastestUpdate.To != "2.0.0" || lt.FastestUpdate.Ms != day.Milliseconds() {
		t.Errorf("lifetime fastest = %+v", lt.FastestUpdate)
	}
	// 1.0.0 → 1.2.0: v1.1.0 came out 60 days ago, the update was seen 20 days ago.
	if lb := lt.LongestBehind; lb == nil || lb.To != "1.2.0" || lb.Ms != (40*day).Milliseconds() {
		t.Errorf("longest behind = %+v", lb)
	}

	ai := out.AI
	if ai == nil || ai.Calls != 3 || ai.Failed != 1 || ai.Manual != 2 || ai.Auto != 1 || ai.Extended != 1 ||
		ai.MedianMs == nil || *ai.MedianMs != 3500 {
		t.Fatalf("ai = %+v", ai)
	}
	if !reflect.DeepEqual(ai.ByModel, []aiModelJSON{
		{Provider: "anthropic", Model: "m", Calls: 2, InputTokens: 100, OutputTokens: 10},
		{Provider: "openai", Model: "g", Calls: 1, Failed: 1},
	}) {
		t.Errorf("by model = %+v", ai.ByModel)
	}
	if m := ai.Months; m[0].Month != ago(40*day).Format("2006-01") || m[len(m)-1].Month != now.Format("2006-01") {
		t.Errorf("ai months = %+v", m)
	}
	// In the 30 days, only the newer two calls count, but the months still cover the range.
	if ai := getStats(t, h, "30d").AI; ai == nil || ai.Calls != 2 || ai.Months[0].Month != ago(30*day).Format("2006-01") {
		t.Errorf("30d ai = %+v", ai)
	}
}

// An image seen before its release was published isn't an instant update: it's left out.
func TestTimeToUpdateSkipsImagesBeforeTheirRelease(t *testing.T) {
	srv, _, db := statsServer(t, staticSource{ctrApp("app", "2.0.0", map[string]string{"tidemark.repo": "github.com/team/app"})})
	ctx := context.Background()
	now := time.Now().UTC()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	if err := db.SaveReleases(ctx, "github.com/team/app", []store.Release{
		{Tag: "v1.0.0", PublishedAt: ago(30 * day)},
		{Tag: "v2.0.0", PublishedAt: ago(2 * day)},
	}, false); err != nil {
		t.Fatal(err)
	}
	for _, he := range []store.HistoryEntry{
		{ContainerName: "app", ImageID: "1", Running: "1.0.0", DetectedAt: ago(20 * day)},
		{ContainerName: "app", ImageID: "2", Running: "2.0.0", PrevImageID: "1", PrevRunning: "1.0.0", DetectedAt: ago(3 * day)},
	} {
		if err := db.AddHistory(ctx, he); err != nil {
			t.Fatal(err)
		}
	}
	out := getStats(t, srv.Handler(fstest.MapFS{}), "30d")
	if ttu := out.Behind.TimeToUpdate; ttu.Count != 0 || ttu.MedianMs != nil {
		t.Errorf("time to update = %+v", ttu)
	}
	if lt := out.Lifetime; lt.FastestUpdate != nil || lt.LongestBehind != nil {
		t.Errorf("lifetime fastest %+v, longest behind %+v", lt.FastestUpdate, lt.LongestBehind)
	}
	if a := out.Behind.Applied; a.Total != 1 {
		t.Errorf("applied total = %d, want 1 (still counted)", a.Total)
	}
}

// Over a long span requests are summed per week, and runs that checked nothing don't make
// an up-to-date streak.
func TestStatsWeeklyRequestsAndEmptyRuns(t *testing.T) {
	srv, _, db := statsServer(t, staticSource{})
	ctx := context.Background()
	first := dayOf(time.Now().Add(-200 * day))
	for i := range 200 {
		r := store.RunStats{RunID: int64(i + 1), At: first.Add(time.Duration(i)*day + time.Hour), Requests: map[string]stats.Count{"registry": {Requests: 1}}}
		if i < 150 {
			r.Checked = 3 // the last 50 runs checked nothing
		}
		if i%10 == 0 {
			r.Updates = 1
		}
		if err := db.AddRunStats(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	out := getStats(t, srv.Handler(fstest.MapFS{}), "all")
	h := out.Health
	if h.RequestsPer != "week" || h.RequestTotals.Registry != 200 || mustDate(t, h.Requests[0].Day).Weekday() != time.Monday {
		t.Errorf("requests per %s: %d entries, first %+v, totals %+v", h.RequestsPer, len(h.Requests), h.Requests[0], h.RequestTotals)
	}
	for i := 1; i < len(h.Requests); i++ {
		if mustDate(t, h.Requests[i].Day).Sub(mustDate(t, h.Requests[i-1].Day)) != 7*day {
			t.Fatalf("weeks not contiguous: %v", h.Requests)
		}
	}
	if s := out.Lifetime.LongestStreak; s == nil || s.Runs != 9 {
		t.Errorf("longest streak = %+v, want 9 runs (empty runs don't count)", s)
	}
	if got := getStats(t, srv.Handler(fstest.MapFS{}), "30d").Health.RequestsPer; got != "day" {
		t.Errorf("30d requests per %q", got)
	}
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestThinAndHelpers(t *testing.T) {
	type pt struct {
		at time.Time
		v  int
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var few []pt
	for i := range maxSeriesPoints {
		few = append(few, pt{base.Add(time.Duration(i) * time.Hour), i})
	}
	at := func(p pt) time.Time { return p.at }
	if got := thin(few, at, func(_, _ pt) bool { return true }); len(got) != maxSeriesPoints {
		t.Errorf("thinned %d points at the limit", len(got))
	}
	// One more: per day, keep the highest value (like the slowest run).
	many := append(few, pt{base.Add(maxSeriesPoints * time.Hour), -1})
	got := thin(many, at, func(kept, later pt) bool { return later.v > kept.v })
	if len(got) != 8 || got[0].v != 23 || got[7].v != 179 {
		t.Errorf("thinned = %v", got)
	}

	if m := median([]int64{5, 1, 3}); *m != 3 {
		t.Errorf("odd median = %d", *m)
	}
	if m := median([]int64{4, 1, 3, 2}); *m != 2 { // (2+3)/2, integer ms
		t.Errorf("even median = %d", *m)
	}
	if median(nil) != nil {
		t.Error("median of nothing")
	}
	if g := medianGapDays([]time.Time{base, base.Add(36 * time.Hour)}); g == nil || *g != 1.5 {
		t.Errorf("gap = %v", g)
	}
	if medianGapDays([]time.Time{base}) != nil {
		t.Error("gap of one release")
	}
	sunday := time.Date(2026, 10, 25, 23, 0, 0, 0, time.UTC)
	if w := weekOf(sunday); w != "2026-10-19" {
		t.Errorf("week of a Sunday = %s", w)
	}
	if w := weekOf(time.Date(2026, 10, 26, 0, 30, 0, 0, time.FixedZone("x", 2*3600))); w != "2026-10-19" { // still Sunday in UTC
		t.Errorf("week across zones = %s", w)
	}
	streak := longestStreak([]store.RunStats{{Checked: 1, At: base}, {Checked: 1, Updates: 1}, {Checked: 1, At: base.Add(1)}, {Checked: 1, At: base.Add(2)}})
	if streak == nil || streak.Runs != 2 || !streak.From.Equal(base.Add(1)) {
		t.Errorf("streak = %+v", streak)
	}
	if longestStreak([]store.RunStats{{Updates: 1}}) != nil {
		t.Error("streak without an up-to-date run")
	}
}

package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/bartvs98/tidemark/internal/stats"
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

// A database from before the rules migration keeps its results, which count as
// checked without rules.
func TestRulesMigrationOnExistingDB(t *testing.T) {
	s := openOld(t, 2, `INSERT INTO results (container_name, image_ref, image_id, latest, class, checked_at)
		VALUES ('redis', 'redis:7.2', 'sha256:r', '7.4.1', 'minor', '2026-10-01T00:00:00Z')`)
	got, err := s.Results(context.Background())
	if err != nil || len(got) != 1 {
		t.Fatalf("results = %+v, %v", got, err)
	}
	if r := got[0]; r.Latest != "7.4.1" || r.Held != "" || r.HeldBy != "" || r.Rules != "" {
		t.Errorf("migrated row = %+v", r)
	}
}

// openOld creates a database with the first n migrations, runs the statements on it,
// then opens it, which applies the remaining migrations.
func openOld(t *testing.T, n int, stmts ...string) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	all := []string{`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)`}
	files, _ := fs.Glob(migrations, "migrations/*.sql")
	slices.Sort(files)
	for i, f := range files[:n] {
		body, err := migrations.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, string(body), fmt.Sprintf(`INSERT INTO schema_migrations (version) VALUES (%d)`, i+1))
	}
	for _, q := range append(all, stmts...) {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// Before failed_at, a failed check replaced the result, so an existing error row never
// succeeded: its checked_at becomes failed_at. Good rows stay as they were.
func TestFailedAtMigrationOnExistingDB(t *testing.T) {
	s := openOld(t, 3, `INSERT INTO results (container_name, image_ref, image_id, latest, class, checked_at)
		VALUES ('redis', 'redis:7.2', 'sha256:r', '7.4.1', 'minor', '2026-10-01T00:00:00Z')`,
		`INSERT INTO results (container_name, image_ref, image_id, error, checked_at)
		VALUES ('app', 'team/app:1', 'sha256:a', 'no such host', '2026-10-02T00:00:00Z')`)
	got, err := s.Results(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("results = %+v, %v", got, err)
	}
	byName := map[string]Result{got[0].ContainerName: got[0], got[1].ContainerName: got[1]}
	if r := byName["redis"]; r.Latest != "7.4.1" || !r.CheckedAt.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) || !r.FailedAt.IsZero() {
		t.Errorf("good row = %+v", r)
	}
	// Rows from before error_kind and tags_from: unclassified, fresh tags.
	if r := byName["app"]; r.Error != "no such host" || !r.CheckedAt.IsZero() || !r.FailedAt.Equal(time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)) ||
		r.ErrorKind != "" || !r.TagsFrom.IsZero() {
		t.Errorf("error row = %+v", r)
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
		Source: "tag", Latest: "10.10.7", Class: "minor", Floating: false, NewImage: true,
		Held: "11.0.0", HeldBy: "allow-major", TagDigest: "sha256:t", Rules: "ignore=false;allow-major=false", CheckedAt: at, RunID: runID}
	b := Result{ContainerName: "redis", ImageRef: "redis:latest", ImageID: "sha256:b", Error: "rate limited", ErrorKind: "rate-limit", FailedAt: at} // never succeeded
	c := a
	c.ContainerName, c.Error, c.ErrorKind, c.FailedAt = "kept", "no such host", "network", at.Add(time.Hour) // failed, kept its result
	c.TagsFrom = at.Add(-time.Hour)                                                                          // that result reused an older tag list
	for _, r := range []Result{a, b, c} {
		if err := s.SaveResult(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	a.Latest, a.Class = "10.10.8", "minor" // upsert
	if err := s.SaveResult(ctx, a); err != nil {
		t.Fatal(err)
	}

	got, err := s.Results(ctx)
	if err != nil || len(got) != 3 {
		t.Fatalf("results = %v, %v", got, err)
	}
	byName := map[string]Result{}
	for _, r := range got {
		byName[r.ContainerName] = r
	}
	for _, want := range []Result{a, b, c} {
		if r := byName[want.ContainerName]; r != want {
			t.Errorf("%s round trip:\n got %+v\nwant %+v", want.ContainerName, r, want)
		}
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

func TestReviews(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	if got, err := s.Reviews(ctx); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
	reviewed := Review{ContainerName: "jellyfin", Kind: "reviewed", Version: "10.10.7", CreatedAt: at}
	byTime := Review{ContainerName: "redis", Kind: "snoozed", Until: at.Add(7 * 24 * time.Hour), CreatedAt: at}
	for _, r := range []Review{reviewed, byTime} {
		if err := s.SaveReview(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Reviews(ctx)
	if err != nil || len(got) != 2 || got["jellyfin"] != reviewed || got["redis"] != byTime {
		t.Fatalf("round trip = %+v, %v", got, err)
	}

	// Snoozing replaces the review: one record per container.
	byVersion := Review{ContainerName: "jellyfin", Kind: "snoozed", Version: "10.10.7", CreatedAt: at.Add(time.Hour)}
	if err := s.SaveReview(ctx, byVersion); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Reviews(ctx); len(got) != 2 || got["jellyfin"] != byVersion {
		t.Errorf("after replace = %+v", got)
	}

	// Deleting twice is fine.
	for range 2 {
		if err := s.DeleteReview(ctx, "jellyfin"); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := s.Reviews(ctx); len(got) != 1 || got["redis"] != byTime {
		t.Errorf("after delete = %+v", got)
	}
}

func TestHistory(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	if last, err := s.LastHistory(ctx); err != nil || len(last) != 0 {
		t.Fatalf("empty: %v %v", last, err)
	}
	rows := []HistoryEntry{
		{ContainerName: "app", ImageID: "sha256:1", Running: "1.0.0", DetectedAt: at},
		{ContainerName: "db", ImageID: "sha256:d", DetectedAt: at},
		{ContainerName: "app", ImageID: "sha256:2", Running: "1.1.0", PrevImageID: "sha256:1", PrevRunning: "1.0.0", DetectedAt: at.Add(time.Hour)},
	}
	for _, h := range rows {
		if err := s.AddHistory(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.History(ctx, "app")
	if err != nil || len(got) != 2 {
		t.Fatalf("history = %+v, %v", got, err)
	}
	want := rows[2]
	want.ID, want.Via = got[0].ID, "check" // Via defaults to check
	if got[0] != want || got[1].ImageID != "sha256:1" || got[1].PrevImageID != "" || got[0].ID <= got[1].ID {
		t.Errorf("history (newest first) = %+v", got)
	}
	if none, err := s.History(ctx, "nope"); err != nil || len(none) != 0 {
		t.Errorf("unknown container: %v %v", none, err)
	}

	last, err := s.LastHistory(ctx)
	if err != nil || len(last) != 2 || last["app"].ImageID != "sha256:2" || last["db"].ImageID != "sha256:d" {
		t.Errorf("last = %+v, %v", last, err)
	}
}

func TestNotified(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	if got, err := s.Notified(ctx); err != nil || len(got) != 0 {
		t.Fatalf("empty = %v, %v", got, err)
	}
	if err := s.MarkNotified(ctx, []Notified{
		{ContainerName: "sonarr", Key: "5.0.0", Level: "none", NotifiedAt: at},
		{ContainerName: "radarr", Key: "sha256:b", Level: "attention", NotifiedAt: at},
	}); err != nil {
		t.Fatal(err)
	}
	// Upsert: a raised level replaces the row.
	later := at.Add(time.Hour)
	if err := s.MarkNotified(ctx, []Notified{{ContainerName: "sonarr", Key: "5.0.0", Level: "breaking", NotifiedAt: later}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Notified(ctx)
	if err != nil || len(got) != 2 {
		t.Fatalf("notified = %v, %v", got, err)
	}
	if n := got[NotifiedKey{"sonarr", "5.0.0"}]; n.Level != "breaking" || !n.NotifiedAt.Equal(later) {
		t.Errorf("sonarr = %+v", n)
	}
	if n := got[NotifiedKey{"radarr", "sha256:b"}]; n.Level != "attention" || !n.NotifiedAt.Equal(at) {
		t.Errorf("radarr = %+v", n)
	}
	if err := s.MarkNotified(ctx, nil); err != nil {
		t.Errorf("nothing to mark: %v", err)
	}
}

func TestNotifications(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	if _, ok, err := s.LastNotification(ctx); ok || err != nil {
		t.Fatalf("empty: ok=%v err=%v", ok, err)
	}
	sent := func() bool {
		t.Helper()
		ok, err := s.SentUpdatesBefore(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}
	add := func(n Notification) {
		t.Helper()
		if err := s.AddNotification(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	add(Notification{SentAt: at, Notifier: "telegram", Kind: "test"})
	add(Notification{SentAt: at, Notifier: "telegram", Kind: "first", Items: 3, Error: "telegram: Bad Request: chat not found"})
	if sent() {
		t.Error("a test and a failed first message count as sent updates")
	}
	n, ok, err := s.LastNotification(ctx)
	if err != nil || !ok || n.Kind != "first" || n.Items != 3 || n.Error == "" || n.Notifier != "telegram" || !n.SentAt.Equal(at) || n.ID == 0 {
		t.Errorf("last = %+v, %v, %v", n, ok, err)
	}
	add(Notification{SentAt: at.Add(time.Minute), Notifier: "telegram", Kind: "first", Items: 3})
	if !sent() {
		t.Error("a sent first message counts")
	}
	add(Notification{SentAt: at.Add(2 * time.Minute), Notifier: "telegram", Kind: "test", Error: "boom"})
	if n, _, _ := s.LastNotification(ctx); n.Kind != "test" || n.Error != "boom" {
		t.Errorf("last = %+v, want the failed test", n)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	if got, err := s.Settings(ctx); err != nil || len(got) != 0 {
		t.Fatalf("empty = %v, %v", got, err)
	}
	if err := s.SaveSettings(ctx, map[string]string{"schedule": "@daily", "github_token": "ghp_a"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSettings(ctx, map[string]string{"schedule": "@hourly"}, []string{"github_token", "never_set"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Settings(ctx)
	if err != nil || len(got) != 1 || got["schedule"] != "@hourly" {
		t.Errorf("settings = %v, %v", got, err)
	}
}

func TestAISummariesRoundTrip(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	if got, err := s.AISummaries(ctx); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
	at := time.Date(2026, 10, 1, 12, 0, 0, 123, time.UTC)
	a := AISummary{Repo: "github.com/a/b", From: "1.0.0", To: "1.2.0", Provider: "anthropic", Model: "m",
		Changes: []string{"c1", "c2"}, Actions: []string{"a1"}, Verdict: "breaking",
		Reasons: []AIReason{{Text: "r", Tag: "v1.2.0"}, {Text: "r2"}}, Releases: 2, Omitted: 1,
		NotesHash: "h", CreatedAt: at, InputTokens: 1200, OutputTokens: 340, Duration: 4200 * time.Millisecond,
		Latest: "v1.2.0", ExtendedFrom: "v1.1.0", Covered: 3, CoveredHash: "ch"}
	if err := s.PutAISummary(ctx, a); err != nil {
		t.Fatal(err)
	}
	// Only a failure so far for another key: no summary fields, nil slices stored as [].
	f := AISummary{Repo: "github.com/a/b", From: "?", To: "2.0", Error: "rate limited", FailedAt: at}
	if err := s.PutAISummary(ctx, f); err != nil {
		t.Fatal(err)
	}
	got, err := s.AISummaries(ctx)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v %v", got, err)
	}
	if fmt.Sprintf("%+v", got[0]) != fmt.Sprintf("%+v", a) {
		t.Errorf("summary:\n got %+v\nwant %+v", got[1], a)
	}
	g := got[1]
	if g.Verdict != "none" || !g.CreatedAt.IsZero() || g.Error != "rate limited" || !g.FailedAt.Equal(at) || g.Changes == nil || len(g.Changes) != 0 {
		t.Errorf("failure row: %+v", g)
	}

	// Upsert by key.
	a.Changes, a.Error, a.FailedAt = []string{"new"}, "", time.Time{}
	if err := s.PutAISummary(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, _ = s.AISummaries(ctx)
	if len(got) != 2 || !slices.Equal(got[0].Changes, []string{"new"}) {
		t.Errorf("upsert: %+v", got)
	}
}

func TestDeleteAISummaries(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	for _, to := range []string{"1", "2", "3"} {
		if err := s.PutAISummary(ctx, AISummary{Repo: "r", From: "0", To: to, CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.DeleteAISummaries(ctx, func(repo, from, to string) bool { return repo == "r" && from == "0" && to == "2" })
	if err != nil || n != 2 {
		t.Fatalf("deleted %d, %v", n, err)
	}
	got, _ := s.AISummaries(ctx)
	if len(got) != 1 || got[0].To != "2" {
		t.Errorf("left: %+v", got)
	}
	if n, err := s.DeleteAISummaries(ctx, func(string, string, string) bool { return true }); err != nil || n != 0 {
		t.Errorf("keep all: %d %v", n, err)
	}
}

func TestAICallsUsage(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	month := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if u, err := s.AIUsageSince(ctx, month); err != nil || u != (AIUsage{}) {
		t.Fatalf("empty: %+v %v", u, err)
	}
	for _, c := range []AICall{
		{At: month.Add(-time.Nanosecond), InputTokens: 999, OutputTokens: 999}, // last month
		{At: month, Provider: "anthropic", Model: "m", Trigger: "manual", Repo: "r", From: "1", To: "2",
			Releases: 3, InputTokens: 1000, OutputTokens: 100, Duration: time.Second},
		{At: month.Add(1500 * time.Millisecond), Trigger: "auto", InputTokens: 20, Error: "rate limited"},
		{At: month.Add(time.Hour).In(time.FixedZone("x", 2*3600)), Trigger: "auto", Mode: "extended", OutputTokens: 5},
	} {
		if err := s.AddAICall(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	u, err := s.AIUsageSince(ctx, month)
	if err != nil || u != (AIUsage{Calls: 3, Failed: 1, Extended: 1, InputTokens: 1020, OutputTokens: 105}) {
		t.Errorf("usage = %+v, %v", u, err)
	}
	// Sub-second precision orders correctly against a whole-second bound.
	if u, _ := s.AIUsageSince(ctx, month.Add(time.Second)); u.Calls != 2 {
		t.Errorf("since +1s: %+v", u)
	}
}

func TestRunStatsRoundTrip(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	day := time.Date(2026, 10, 1, 6, 0, 0, 0, time.UTC)
	if got, err := s.RunStatsSince(ctx, time.Time{}); err != nil || len(got) != 0 {
		t.Fatalf("empty: %+v %v", got, err)
	}
	remaining, limit := 4812, 5000
	full := RunStats{
		RunID: 2, At: day.Add(500 * time.Millisecond).In(time.FixedZone("x", 3600)), Duration: 1234 * time.Millisecond,
		Containers: 10, Ignored: 1, Checked: 8, UpToDate: 5, Updates: 2, Attention: 1, Breaking: 1, Reviewed: 1, Snoozed: 0, Failed: 1, ReleasesBehind: 17,
		Classes:         map[string]int{"minor": 2, "major": 1, "patch": 0},
		Failures:        map[string]int{"rate-limit": 1},
		Requests:        map[string]stats.Count{"registry": {Requests: 40, Limited: 1}, "github": {Requests: 3}},
		GitHubRemaining: &remaining, GitHubLimit: &limit,
	}
	for _, r := range []RunStats{
		{RunID: 1, At: day.Add(-time.Hour)}, // nil maps, unknown quota
		full,
		{RunID: 3, At: day.Add(time.Hour), Updates: 9},
	} {
		if err := s.AddRunStats(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.RunStatsSince(ctx, time.Time{})
	if err != nil || len(all) != 3 || all[0].RunID != 1 || all[2].RunID != 3 {
		t.Fatalf("all = %+v, %v", all, err)
	}
	if r := all[0]; r.GitHubRemaining != nil || r.GitHubLimit != nil || len(r.Classes) != 0 || r.Requests == nil {
		t.Errorf("sparse row = %+v", r)
	}
	got := all[1]
	want := full
	want.At = full.At.UTC()
	want.Classes = map[string]int{"minor": 2, "major": 1} // zero counts aren't stored
	if !reflect.DeepEqual(got, want) {
		t.Errorf("row =\n %+v\nwant\n %+v", got, want)
	}

	// Sub-second precision against a whole-second bound: the bound is inclusive.
	since, err := s.RunStatsSince(ctx, day)
	if err != nil || len(since) != 2 || since[0].RunID != 2 {
		t.Errorf("since = %+v, %v", since, err)
	}
	// Storing a run again replaces its row.
	if err := s.AddRunStats(ctx, RunStats{RunID: 3, At: day.Add(time.Hour), Updates: 1}); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.RunStatsSince(ctx, day.Add(time.Minute)); len(again) != 1 || again[0].Updates != 1 {
		t.Errorf("replaced = %+v", again)
	}
}

func TestStatsReads(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	id1, _ := s.StartRun(ctx, "startup", t0)
	_ = s.FinishRun(ctx, id1, t0.Add(time.Minute), 4, "")
	id2, _ := s.StartRun(ctx, "manual", t0.Add(time.Hour))
	runs, err := s.Runs(ctx)
	if err != nil || len(runs) != 2 || runs[0].ID != id1 || runs[0].FinishedAt == nil || runs[0].Images != 4 ||
		runs[1].ID != id2 || runs[1].FinishedAt != nil || runs[1].Trigger != "manual" {
		t.Errorf("runs = %+v, %v", runs, err)
	}

	for _, h := range []HistoryEntry{
		{ContainerName: "b", ImageID: "1", Running: "1.0", DetectedAt: t0},
		{ContainerName: "a", ImageID: "2", Running: "2.0", DetectedAt: t0},
		{ContainerName: "b", ImageID: "3", Running: "1.1", PrevImageID: "1", PrevRunning: "1.0", DetectedAt: t0.Add(time.Hour)},
	} {
		if err := s.AddHistory(ctx, h); err != nil {
			t.Fatal(err)
		}
	}
	hist, err := s.AllHistory(ctx)
	if err != nil || len(hist) != 3 || hist[0].ContainerName != "b" || hist[2].PrevRunning != "1.0" {
		t.Errorf("history = %+v, %v", hist, err)
	}

	_ = s.SaveReleases(ctx, "github.com/a/a", []Release{{Tag: "v1", PublishedAt: t0}, {Tag: "v2"}}, false)
	_ = s.SaveReleases(ctx, "github.com/b/b", []Release{{Tag: "v9", Body: "notes"}}, false)
	rels, err := s.AllReleases(ctx)
	if err != nil || len(rels) != 2 || len(rels["github.com/a/a"]) != 2 || rels["github.com/b/b"][0].Tag != "v9" || rels["github.com/b/b"][0].Body != "" {
		t.Errorf("releases = %+v, %v", rels, err)
	}
	_ = s.SaveRepoState(ctx, RepoState{Repo: "github.com/a/a", Kind: "releases", HasMore: true, FetchedAt: t0})
	states, err := s.AllRepoStates(ctx)
	if st := states["github.com/a/a"]; err != nil || len(states) != 1 || !st.HasMore || !st.FetchedAt.Equal(t0) {
		t.Errorf("repo states = %+v, %v", states, err)
	}

	_ = s.AddAICall(ctx, AICall{At: t0.Add(time.Hour), Provider: "openai", Trigger: "auto", Duration: 2 * time.Second})
	_ = s.AddAICall(ctx, AICall{At: t0, Provider: "anthropic", Trigger: "manual", Mode: "extended", Error: "boom"})
	calls, err := s.AICalls(ctx)
	if err != nil || len(calls) != 2 || calls[0].Provider != "anthropic" || calls[0].Mode != "extended" || calls[0].Error != "boom" ||
		calls[1].Duration != 2*time.Second || calls[1].Mode != "full" || !calls[1].At.Equal(t0.Add(time.Hour)) {
		t.Errorf("calls = %+v, %v", calls, err)
	}
}

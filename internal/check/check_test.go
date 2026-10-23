package check

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/registry"
	"github.com/bartvs98/tidemark/internal/resolve"
	"github.com/bartvs98/tidemark/internal/rules"
	"github.com/bartvs98/tidemark/internal/store"
	"github.com/bartvs98/tidemark/internal/version"
)

type listSource struct {
	list []docker.Container
	err  error
}

func (l listSource) Containers(context.Context) ([]docker.Container, error) { return l.list, l.err }
func (l listSource) Info() docker.SourceInfo                                { return docker.SourceInfo{} }

type countingRegistry struct {
	mu    sync.Mutex
	calls map[string]int
	block chan struct{} // if set, Tags waits for it
	down  bool          // every lookup fails, like before the network is up
	stale time.Time     // if set, tag lists are rate-limited and come from the cache of then
}

func (r *countingRegistry) Tags(_ context.Context, ref imageref.Ref) ([]string, error) {
	if r.block != nil {
		<-r.block
	}
	r.mu.Lock()
	r.calls[ref.Repository]++
	down, stale := r.down, r.stale
	r.mu.Unlock()
	if down {
		return nil, &registry.Error{Kind: registry.KindNetwork, Msg: "index.docker.io/" + ref.Repository + ": can't resolve index.docker.io"}
	}
	if !stale.IsZero() {
		limited := &registry.Error{Kind: registry.KindRateLimit, Msg: "rate limited"}
		return []string{"1.0.0", "1.1.0"}, &registry.StaleError{At: stale, Err: limited}
	}
	if ref.Repository == "team/broken" {
		return nil, errors.New("registry exploded")
	}
	return []string{"1.0.0", "1.1.0"}, nil
}

func (r *countingRegistry) Digest(context.Context, imageref.Ref, string) (string, error) {
	return "", errors.New("unused")
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func openStore(t *testing.T, path string) *store.Store {
	t.Helper()
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func waitDone(t *testing.T, s *Service) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := s.Status(); !st.Running {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("check did not finish")
	return Status{}
}

func ctr(name, image, imageID string) docker.Container {
	return docker.Container{ID: "id-" + name, Name: name, ImageRef: image, Image: docker.Image{ID: imageID}}
}

func TestRun(t *testing.T) {
	reg := &countingRegistry{calls: map[string]int{}}
	a, b := ctr("a", "team/app:1.0.0", "sha256:1"), ctr("b", "team/app:1.0.0", "sha256:1") // same image: checked once
	c := ctr("c", "team/broken:1.0.0", "sha256:2")
	d := docker.Container{ID: "id-d", Name: "d", ImageRef: "team/gone:1", Image: docker.Image{ID: "sha256:3", Missing: true}}
	s := New(listSource{list: []docker.Container{a, b, c, d}}, reg, openStore(t, filepath.Join(t.TempDir(), "t.db")), quiet)

	if !s.Start(context.Background(), TriggerManual) {
		t.Fatal("first Start should start a run")
	}
	st := waitDone(t, s)
	if st.Done != 3 || st.Total != 3 || st.FinishedAt == nil || st.Err != "" || st.Trigger != TriggerManual {
		t.Errorf("status = %+v", st)
	}
	if reg.calls["team/app"] != 1 {
		t.Errorf("team/app checked %d times, want 1", reg.calls["team/app"])
	}
	for _, x := range []docker.Container{a, b} {
		e, ok := s.Result(x)
		if !ok || e.Err != "" || e.Result.Running != "1.0.0" || e.Result.Latest != "1.1.0" || e.Result.Class != version.Minor {
			t.Errorf("%s = %+v", x.Name, e)
		}
	}
	if e, _ := s.Result(c); e.Err != "registry exploded" || e.ErrKind != "" {
		t.Errorf("c err = %q (kind %q), want it unclassified", e.Err, e.ErrKind)
	}
	if e, _ := s.Result(d); e.Err == "" || e.ErrKind != "" {
		t.Errorf("missing image: err = %q (kind %q), want an error without a kind", e.Err, e.ErrKind)
	}
}

func TestResultsSurviveRestartAndExpireOnNewImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	app := ctr("app", "team/app:1.0.0", "sha256:1")
	gone := ctr("gone", "team/app:1.0.0", "sha256:1")

	first := New(listSource{list: []docker.Container{app, gone}}, &countingRegistry{calls: map[string]int{}}, openStore(t, path), quiet)
	first.Start(context.Background(), TriggerStartup)
	waitDone(t, first)

	// "gone" is removed; the next run prunes it.
	first.source = listSource{list: []docker.Container{app}}
	first.Start(context.Background(), TriggerManual)
	waitDone(t, first)

	// A new process on the same database.
	second := New(listSource{list: []docker.Container{app}}, &countingRegistry{calls: map[string]int{}}, openStore(t, path), quiet)
	if err := second.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e, ok := second.Result(app); !ok || e.Result.Latest != "1.1.0" || e.Result.Class != version.Minor {
		t.Errorf("result after restart = %+v ok=%v", e, ok)
	}
	if _, ok := second.Result(gone); ok {
		t.Error("removed container's result should have been pruned")
	}
	if st := second.Status(); st.FinishedAt == nil || st.Trigger != TriggerManual || second.LastFinished().IsZero() {
		t.Errorf("status after restart = %+v", st)
	}

	// Same name, recreated with a new image (compose pull && up): the old result no longer applies.
	recreated := ctr("app", "team/app:1.1.0", "sha256:9")
	if _, ok := second.Result(recreated); ok {
		t.Error("result for an older image must not be shown")
	}
}

func TestStartWhileRunning(t *testing.T) {
	reg := &countingRegistry{calls: map[string]int{}, block: make(chan struct{})}
	s := New(listSource{list: []docker.Container{ctr("a", "team/app:1.0.0", "sha256:1")}}, reg, openStore(t, filepath.Join(t.TempDir(), "t.db")), quiet)
	s.Start(context.Background(), TriggerManual)
	if s.Start(context.Background(), TriggerManual) {
		t.Error("second Start while running should be refused")
	}
	close(reg.block)
	waitDone(t, s)
}

func TestSourceError(t *testing.T) {
	s := New(listSource{err: errors.New("can't reach Docker")}, &countingRegistry{calls: map[string]int{}}, openStore(t, filepath.Join(t.TempDir(), "t.db")), quiet)
	s.Start(context.Background(), TriggerManual)
	if st := waitDone(t, s); st.Err != "can't reach Docker" {
		t.Errorf("status = %+v", st)
	}
}

func TestSetSchedule(t *testing.T) {
	s := New(listSource{}, &countingRegistry{calls: map[string]int{}}, openStore(t, filepath.Join(t.TempDir(), "t.db")), quiet)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if s.Schedule() != "" || s.Status().NextRunAt != nil {
		t.Error("scheduled before SetSchedule")
	}
	if err := s.SetSchedule(ctx, "every so often"); err == nil {
		t.Error("invalid spec should be rejected")
	}
	if err := s.SetSchedule(ctx, "0 */6 * * *"); err != nil {
		t.Fatal(err)
	}
	next := s.Status().NextRunAt
	if next == nil || !next.After(time.Now()) || next.After(time.Now().Add(6*time.Hour)) || next.Minute() != 0 || next.Hour()%6 != 0 {
		t.Errorf("next run = %v", next)
	}

	// Replace: the new spec is in effect, the old one is gone.
	if err := s.SetSchedule(ctx, "@every 1m"); err != nil {
		t.Fatal(err)
	}
	next = s.Status().NextRunAt
	if s.Schedule() != "@every 1m" || next == nil || next.After(time.Now().Add(time.Minute+time.Second)) {
		t.Errorf("schedule %q, next run = %v", s.Schedule(), next)
	}
	// A failed replace keeps the current schedule.
	if err := s.SetSchedule(ctx, "nope"); err == nil || s.Schedule() != "@every 1m" {
		t.Errorf("after a failed replace: %q, %v", s.Schedule(), err)
	}

	// Off.
	if err := s.SetSchedule(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if s.Schedule() != "" || s.Status().NextRunAt != nil {
		t.Errorf("off: %q, next %v", s.Schedule(), s.Status().NextRunAt)
	}
}

// blockingSource holds a run in Containers until release is closed or ctx ends.
type blockingSource struct{ release chan struct{} }

func (b *blockingSource) Containers(ctx context.Context) ([]docker.Container, error) {
	select {
	case <-b.release:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (b *blockingSource) Info() docker.SourceInfo { return docker.SourceInfo{} }

// Turning the schedule off while a run is busy lets the run finish.
func TestSetScheduleKeepsRunningCheck(t *testing.T) {
	src := &blockingSource{release: make(chan struct{})}
	s := New(src, &countingRegistry{calls: map[string]int{}}, openStore(t, filepath.Join(t.TempDir(), "t.db")), quiet)
	ctx := context.Background()
	if err := s.SetSchedule(ctx, "@every 1h"); err != nil {
		t.Fatal(err)
	}
	s.Start(ctx, TriggerSchedule)
	if err := s.SetSchedule(ctx, ""); err != nil {
		t.Fatal(err)
	}
	close(src.release)
	if st := waitDone(t, s); st.Err != "" {
		t.Errorf("run ended with %q", st.Err)
	}
}

func withLabels(c docker.Container, kv ...string) docker.Container {
	c.Labels = map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		c.Labels[kv[i]] = kv[i+1]
	}
	return c
}

func runOnce(t *testing.T, s *Service) Status {
	t.Helper()
	if !s.Start(context.Background(), TriggerManual) {
		t.Fatal("Start refused")
	}
	return waitDone(t, s)
}

func TestIgnoredContainersAreNotChecked(t *testing.T) {
	reg := &countingRegistry{calls: map[string]int{}}
	st := openStore(t, filepath.Join(t.TempDir(), "t.db"))
	app := ctr("app", "team/app:1.0.0", "sha256:1")
	s := New(listSource{list: []docker.Container{app}}, reg, st, quiet)
	runOnce(t, s)
	if _, ok := s.Result(app); !ok {
		t.Fatal("app should have a result before it's ignored")
	}

	ignored := withLabels(app, "tidemark.ignore", "true")
	var hooked int
	s.SetAfterCheck(func(context.Context, docker.Container, rules.Rules, resolve.Result) { hooked++ })
	s.source = listSource{list: []docker.Container{ignored}}
	if got := runOnce(t, s); got.Total != 0 || got.Done != 0 {
		t.Errorf("status = %+v, want nothing checked", got)
	}
	if reg.calls["team/app"] != 1 || hooked != 0 {
		t.Errorf("registry calls = %d, hook calls = %d; an ignored container must not be checked", reg.calls["team/app"], hooked)
	}
	if _, ok := s.Result(ignored); ok {
		t.Error("ignored container should have no result")
	}
	if rows, _ := st.Results(context.Background()); len(rows) != 0 {
		t.Errorf("stored results = %+v, want the ignored one dropped", rows)
	}
}

func TestChangedRulesMakeResultStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	app := withLabels(ctr("app", "team/app:1.0.0", "sha256:1"), "tidemark.allow-major", "false")
	s := New(listSource{list: []docker.Container{app}}, &countingRegistry{calls: map[string]int{}}, openStore(t, path), quiet)
	runOnce(t, s)

	// After a restart the fingerprint still matches.
	again := New(listSource{list: []docker.Container{app}}, &countingRegistry{calls: map[string]int{}}, openStore(t, path), quiet)
	if err := again.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := again.Result(app); !ok {
		t.Error("result with unchanged rules should survive a restart")
	}
	// Recreated with another label but the same image: the old result doesn't apply.
	pinned := withLabels(app, "tidemark.pin", "1.0")
	if _, ok := again.Result(pinned); ok {
		t.Error("result checked with other rules must be stale")
	}
	// Labels removed entirely: also stale (the result was checked with a hold).
	if _, ok := again.Result(ctr("app", "team/app:1.0.0", "sha256:1")); ok {
		t.Error("result checked with rules must be stale once the rules are gone")
	}
}

func TestGroupsSplitByRules(t *testing.T) {
	reg := &countingRegistry{calls: map[string]int{}}
	plain := ctr("plain", "team/app:1.0.0", "sha256:1")
	twin := ctr("twin", "team/app:1.0.0", "sha256:1")
	pinned := withLabels(ctr("pinned", "team/app:1.0.0", "sha256:1"), "tidemark.pin", "1.0")
	s := New(listSource{list: []docker.Container{plain, twin, pinned}}, reg, openStore(t, filepath.Join(t.TempDir(), "t.db")), quiet)
	if st := runOnce(t, s); st.Total != 2 {
		t.Errorf("total = %d, want 2 checks (plain+twin share one)", st.Total)
	}
	if reg.calls["team/app"] != 2 {
		t.Errorf("team/app checked %d times, want 2", reg.calls["team/app"])
	}
	for _, c := range []docker.Container{plain, twin} {
		if e, ok := s.Result(c); !ok || e.Result.Latest != "1.1.0" || e.Result.Held != "" {
			t.Errorf("%s = %+v", c.Name, e)
		}
	}
	e, ok := s.Result(pinned)
	if !ok || e.Result.Latest != "1.0.0" || e.Result.Class != version.None || e.Result.Held != "1.1.0" || e.Result.HeldBy != resolve.HeldByPin {
		t.Errorf("pinned = %+v", e)
	}
}

func TestConfigReloadAndBadFile(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	reg := &countingRegistry{calls: map[string]int{}}
	app := ctr("app", "team/app:1.0.0", "sha256:1")
	s := New(listSource{list: []docker.Container{app}}, reg, openStore(t, filepath.Join(dir, "t.db")), quiet)
	s.UseRulesFile(cfg) // missing file: no rules, no error
	if s.RulesError() != "" || s.Rules(app).Active() {
		t.Fatalf("missing file: err=%q rules=%+v", s.RulesError(), s.Rules(app))
	}

	// Written after startup: picked up by the next run without a restart.
	write("containers:\n  app:\n    ignore: true\n")
	runOnce(t, s)
	if reg.calls["team/app"] != 0 || !s.Rules(app).Ignore {
		t.Errorf("config not applied on the next run: calls=%d", reg.calls["team/app"])
	}

	// A broken file keeps the previous rules and reports why.
	write("containers:\n  app:\n    ignroe: true\n")
	runOnce(t, s)
	if s.RulesError() == "" {
		t.Error("bad config file should be reported")
	}
	if reg.calls["team/app"] != 0 || !s.Rules(app).Ignore {
		t.Error("bad config file should keep the previous rules")
	}

	// Fixed: error cleared, new rules apply.
	write("images:\n  team/app:\n    pin: \"1.0\"\n")
	runOnce(t, s)
	if s.RulesError() != "" {
		t.Errorf("error after fixing the file = %q", s.RulesError())
	}
	if e, ok := s.Result(app); !ok || e.Result.Held != "1.1.0" || s.Rules(app).From[rules.KeyPin] != rules.FromConfig {
		t.Errorf("result after fix = %+v ok=%v", e, ok)
	}
}

func TestUnchecked(t *testing.T) {
	app := ctr("app", "team/app:1.0.0", "sha256:1")
	ignored := withLabels(ctr("skip", "team/other:1.0.0", "sha256:2"), "tidemark.ignore", "true")
	s := New(listSource{list: []docker.Container{app, ignored}}, &countingRegistry{calls: map[string]int{}}, openStore(t, filepath.Join(t.TempDir(), "t.db")), quiet)

	count := func() int {
		t.Helper()
		n, err := s.Unchecked(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(); n != 1 {
		t.Errorf("before any run: %d unchecked, want 1 (ignored doesn't count)", n)
	}
	runOnce(t, s)
	if n := count(); n != 0 {
		t.Errorf("after a run: %d unchecked, want 0", n)
	}
	// Recreated with a new label (stale fingerprint) and a new container appeared.
	s.source = listSource{list: []docker.Container{withLabels(app, "tidemark.pin", "1"), ctr("new", "team/app:1.0.0", "sha256:1"), ignored}}
	if n := count(); n != 2 {
		t.Errorf("after rule change + new container: %d unchecked, want 2", n)
	}
	s.source = listSource{err: errors.New("down")}
	if _, err := s.Unchecked(context.Background()); err == nil {
		t.Error("source error should be returned")
	}
}

func TestFailedCheckKeepsLastResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	reg := &countingRegistry{calls: map[string]int{}}
	app := ctr("app", "team/app:1.0.0", "sha256:1")
	s := New(listSource{list: []docker.Container{app}}, reg, openStore(t, path), quiet)
	var hooked int
	s.SetAfterCheck(func(context.Context, docker.Container, rules.Rules, resolve.Result) { hooked++ })
	runOnce(t, s)
	good, _ := s.Result(app)
	if good.Err != "" || good.CheckedAt.IsZero() || !good.FailedAt.IsZero() || hooked != 1 {
		t.Fatalf("first run = %+v, hooked %d", good, hooked)
	}

	// The network is down (woke from sleep). "twin" shares app's check but was never
	// checked before, so it has nothing to keep.
	twin := ctr("twin", "team/app:1.0.0", "sha256:1")
	reg.mu.Lock()
	reg.down = true
	reg.mu.Unlock()
	s.source = listSource{list: []docker.Container{app, twin}}
	runOnce(t, s)
	if hooked != 1 {
		t.Errorf("after-check hook ran %d times, want no run for a failed check", hooked)
	}

	kept := func(s *Service, when string) {
		t.Helper()
		e, ok := s.Result(app)
		if !ok || e.Result != good.Result || !e.CheckedAt.Equal(good.CheckedAt) || e.Err == "" || e.ErrKind != string(registry.KindNetwork) || e.FailedAt.Before(good.CheckedAt) {
			t.Errorf("%s: app = %+v ok=%v, want the good result kept with the error", when, e, ok)
		}
		e, ok = s.Result(twin)
		if !ok || e.Result != (resolve.Result{}) || !e.CheckedAt.IsZero() || e.Err == "" || e.ErrKind != string(registry.KindNetwork) || e.FailedAt.IsZero() {
			t.Errorf("%s: twin = %+v ok=%v, want the error alone", when, e, ok)
		}
	}
	kept(s, "in memory")

	// After a restart, and a failed check doesn't force a startup run.
	again := New(listSource{list: []docker.Container{app, twin}}, reg, openStore(t, path), quiet)
	if err := again.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	kept(again, "after restart")
	if n, _ := again.Unchecked(context.Background()); n != 0 {
		t.Errorf("unchecked = %d, want 0", n)
	}

	// Back online: success clears the failure.
	reg.mu.Lock()
	reg.down = false
	reg.mu.Unlock()
	runOnce(t, again)
	for _, c := range []docker.Container{app, twin} {
		if e, ok := again.Result(c); !ok || e.Err != "" || e.ErrKind != "" || !e.FailedAt.IsZero() || e.CheckedAt.Equal(good.CheckedAt) || e.Result.Latest != "1.1.0" {
			t.Errorf("%s after recovery = %+v ok=%v", c.Name, e, ok)
		}
	}

	// Recreated with another image while offline: the old result must not come back.
	recreated := ctr("app", "team/app:1.1.0", "sha256:9")
	reg.mu.Lock()
	reg.down = true
	reg.mu.Unlock()
	again.source = listSource{list: []docker.Container{recreated}}
	runOnce(t, again)
	if e, ok := again.Result(recreated); !ok || e.Result != (resolve.Result{}) || !e.CheckedAt.IsZero() || e.Err == "" {
		t.Errorf("recreated = %+v ok=%v, want the error alone", e, ok)
	}
	// Changed rules likewise.
	pinned := withLabels(recreated, "tidemark.pin", "1.1")
	reg.mu.Lock()
	reg.down = false
	reg.mu.Unlock()
	again.source = listSource{list: []docker.Container{recreated}}
	runOnce(t, again)
	reg.mu.Lock()
	reg.down = true
	reg.mu.Unlock()
	again.source = listSource{list: []docker.Container{pinned}}
	runOnce(t, again)
	if e, ok := again.Result(pinned); !ok || !e.CheckedAt.IsZero() || e.Err == "" {
		t.Errorf("pinned = %+v ok=%v, want the error alone", e, ok)
	}
}

// A check that ran on a cached tag list (rate-limited) succeeds and remembers since when.
func TestStaleTagsAreRecorded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	at := time.Date(2026, 10, 18, 6, 0, 0, 0, time.UTC)
	reg := &countingRegistry{calls: map[string]int{}, stale: at}
	app := ctr("app", "team/app:1.0.0", "sha256:1")
	s := New(listSource{list: []docker.Container{app}}, reg, openStore(t, path), quiet)
	runOnce(t, s)
	if e, ok := s.Result(app); !ok || e.Err != "" || e.CheckedAt.IsZero() || !e.Result.TagsFrom.Equal(at) || e.Result.Latest != "1.1.0" {
		t.Errorf("stale check = %+v ok=%v", e, ok)
	}
	again := New(listSource{list: []docker.Container{app}}, reg, openStore(t, path), quiet)
	if err := again.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e, ok := again.Result(app); !ok || !e.Result.TagsFrom.Equal(at) {
		t.Errorf("after restart = %+v ok=%v", e, ok)
	}
}

// History records each image change between successful checks, starting with a
// "tracking started" row, and continues across failed checks and restarts.
func TestHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	st := openStore(t, path)
	reg := &countingRegistry{calls: map[string]int{}}
	history := func(st *store.Store) []store.HistoryEntry {
		t.Helper()
		h, err := st.History(context.Background(), "app")
		if err != nil {
			t.Fatal(err)
		}
		return h
	}

	// First check: tracking started.
	s := New(listSource{list: []docker.Container{ctr("app", "team/app:1.0.0", "sha256:1")}}, reg, st, quiet)
	runOnce(t, s)
	h := history(st)
	if len(h) != 1 || h[0].ImageID != "sha256:1" || h[0].Running != "1.0.0" || h[0].PrevImageID != "" || h[0].PrevRunning != "" ||
		h[0].Via != "check" || h[0].DetectedAt.IsZero() {
		t.Fatalf("first check = %+v", h)
	}

	// Same image again: nothing new.
	runOnce(t, s)
	if h := history(st); len(h) != 1 {
		t.Fatalf("same image added rows: %+v", h)
	}

	// New image ID: a change row from the previous one.
	s.source = listSource{list: []docker.Container{ctr("app", "team/app:1.1.0", "sha256:2")}}
	runOnce(t, s)
	h = history(st)
	if len(h) != 2 || h[0].ImageID != "sha256:2" || h[0].Running != "1.1.0" || h[0].PrevImageID != "sha256:1" || h[0].PrevRunning != "1.0.0" {
		t.Fatalf("change = %+v", h)
	}

	// A failed check in between (image gone from the host) records nothing, and the next
	// successful one still changes from sha256:2.
	s.source = listSource{list: []docker.Container{{ID: "id-app", Name: "app", ImageRef: "team/app:1.2.0", Image: docker.Image{ID: "sha256:3", Missing: true}}}}
	runOnce(t, s)
	if h := history(st); len(h) != 2 {
		t.Fatalf("failed check added rows: %+v", h)
	}
	// So does a resolve error.
	s.source = listSource{list: []docker.Container{ctr("app", "team/broken:1.0.0", "sha256:4")}}
	runOnce(t, s)
	if h := history(st); len(h) != 2 {
		t.Fatalf("resolve error added rows: %+v", h)
	}
	s.source = listSource{list: []docker.Container{ctr("app", "team/app:1.1.0", "sha256:5")}}
	runOnce(t, s)
	h = history(st)
	if len(h) != 3 || h[0].ImageID != "sha256:5" || h[0].PrevImageID != "sha256:2" || h[0].PrevRunning != "1.1.0" {
		t.Fatalf("after failures = %+v", h)
	}

	// A restart continues from the stored last row.
	st2 := openStore(t, path)
	again := New(listSource{list: []docker.Container{ctr("app", "team/app:1.1.0", "sha256:5")}}, reg, st2, quiet)
	if err := again.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	runOnce(t, again)
	if h := history(st2); len(h) != 3 {
		t.Fatalf("restart with the same image added rows: %+v", h)
	}
	again.source = listSource{list: []docker.Container{ctr("app", "team/app:1.0.0", "sha256:6")}}
	runOnce(t, again)
	h = history(st2)
	if len(h) != 4 || h[0].ImageID != "sha256:6" || h[0].PrevImageID != "sha256:5" || h[0].Running != "1.0.0" || h[0].PrevRunning != "1.1.0" {
		t.Fatalf("after restart = %+v", h)
	}
}

// Ignored containers aren't checked, so they get no history; containers sharing a check
// each get their own rows.
func TestHistoryIgnoredAndShared(t *testing.T) {
	st := openStore(t, filepath.Join(t.TempDir(), "t.db"))
	a, b := ctr("a", "team/app:1.0.0", "sha256:1"), ctr("b", "team/app:1.0.0", "sha256:1")
	ign := withLabels(ctr("ign", "team/app:1.0.0", "sha256:1"), "tidemark.ignore", "true")
	s := New(listSource{list: []docker.Container{a, b, ign}}, &countingRegistry{calls: map[string]int{}}, st, quiet)
	runOnce(t, s)
	last, err := st.LastHistory(context.Background())
	if err != nil || len(last) != 2 || last["a"].ImageID != "sha256:1" || last["b"].ImageID != "sha256:1" {
		t.Errorf("last = %+v, %v", last, err)
	}
}

func TestWait(t *testing.T) {
	reg := &countingRegistry{calls: map[string]int{}, block: make(chan struct{})}
	s := New(listSource{list: []docker.Container{ctr("a", "team/app:1.0.0", "sha256:1")}}, reg, openStore(t, filepath.Join(t.TempDir(), "t.db")), quiet)
	if err := s.Wait(context.Background()); err != nil {
		t.Errorf("Wait with no run = %v", err)
	}
	s.Start(context.Background(), TriggerManual)

	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.Wait(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Wait while running = %v, want deadline exceeded", err)
	}

	close(reg.block)
	if err := s.Wait(context.Background()); err != nil {
		t.Errorf("Wait after run = %v", err)
	}
	if s.Status().Running {
		t.Error("run still marked running after Wait")
	}
}

func TestAfterRun(t *testing.T) {
	var runs []Finished
	db := openStore(t, filepath.Join(t.TempDir(), "t.db"))
	s := New(listSource{list: []docker.Container{ctr("a", "team/app:1.0.0", "sha256:1")}}, &countingRegistry{calls: map[string]int{}}, db, quiet)
	s.SetAfterRun(func(_ context.Context, run Finished) { runs = append(runs, run) })
	runOnce(t, s)
	if err := s.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Trigger != TriggerManual || runs[0].Images != 1 {
		t.Fatalf("hook calls = %+v", runs)
	}
	// The hook gets the run as recorded.
	last, ok, err := db.LastFinishedRun(context.Background())
	if r := runs[0]; err != nil || !ok || r.ID != last.ID || !r.StartedAt.Equal(last.StartedAt) || !r.FinishedAt.Equal(*last.FinishedAt) {
		t.Errorf("hook run = %+v, recorded %+v (%v)", r, last, err)
	}
}

func TestAfterRunNotCalledOnRunError(t *testing.T) {
	called := false
	s := New(listSource{err: errors.New("can't reach Docker")}, &countingRegistry{calls: map[string]int{}}, openStore(t, filepath.Join(t.TempDir(), "t.db")), quiet)
	s.SetAfterRun(func(context.Context, Finished) { called = true })
	if st := runOnce(t, s); st.Err == "" {
		t.Fatalf("status = %+v", st)
	}
	if err := s.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("AfterRun ran after a failed run")
	}
}

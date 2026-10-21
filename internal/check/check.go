// Package check runs update checks for all containers in the background, on a
// schedule and on demand, and keeps the latest result per container in memory
// backed by the store, so results survive restarts.
package check

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/registry"
	"github.com/bartvs98/tidemark/internal/resolve"
	"github.com/bartvs98/tidemark/internal/rules"
	"github.com/bartvs98/tidemark/internal/store"
	"github.com/bartvs98/tidemark/internal/version"
)

const (
	workers          = 6
	containerTimeout = 2 * time.Minute
	// afterCheckTimeout caps the AfterCheck hook (release-note prefetch), so a slow
	// forge can't hold a worker for the rest of the run.
	afterCheckTimeout = time.Minute
)

// Triggers say what started a run.
const (
	TriggerStartup  = "startup"
	TriggerSchedule = "schedule"
	TriggerManual   = "manual"
)

// Entry is a container's latest result. A failed check doesn't replace a good result:
// Result and CheckedAt stay those of the last successful check, and Err and FailedAt
// say that refreshing it failed. CheckedAt is zero if no check ever succeeded.
type Entry struct {
	ImageRef  string // what was checked: a result only applies while the
	ImageID   string // container still runs this exact image
	Rules     string // with these rules (fingerprint, "" for none)
	Result    resolve.Result
	Err       string    // the last attempt failed with this, "" if it succeeded
	ErrKind   string    // registry.Kind of Err (rate-limit, timeout, …), "" if unclassified
	CheckedAt time.Time // last successful check
	FailedAt  time.Time // the failed attempt Err is from
}

// valid says whether the entry applies to a container running image ref/id with rules fp.
func (e Entry) valid(ref, id, fp string) bool {
	return e.ImageRef == ref && e.ImageID == id && e.Rules == fp
}

type Status struct {
	Running    bool
	Trigger    string
	StartedAt  *time.Time
	FinishedAt *time.Time
	Done       int // images checked so far in the current/last run
	Total      int
	Err        string     // the run itself failed (e.g. Docker unreachable)
	NextRunAt  *time.Time // next scheduled run, nil without a schedule
}

// AfterCheck runs after a container's image was checked successfully (once per image
// and rules), e.g. to prefetch release notes. It runs inside the check run's worker.
type AfterCheck func(ctx context.Context, c docker.Container, rs rules.Rules, r resolve.Result)

// AfterRun runs once a run has finished and recorded its end, unless the run itself failed
// (Docker unreachable). It runs inside the run's goroutine, so Wait also waits for it.
type AfterRun func(ctx context.Context, trigger string)

type Service struct {
	source docker.Source
	reg    resolve.Registry
	store  *store.Store
	log    *slog.Logger
	after  AfterCheck
	ran    AfterRun

	rulesPath string // config file; "" means labels only

	mu       sync.RWMutex
	results  map[string]Entry              // by container name
	last     map[string]store.HistoryEntry // newest history row by container name
	status   Status
	runDone  chan struct{} // closed when the current (or last) run has fully ended, see Wait
	cron     *cron.Cron
	cronID   cron.EntryID
	rules    *rules.File // last config file that loaded; nil if none did
	rulesErr string      // why the config file last failed to load, "" if it loaded
}

func New(source docker.Source, reg resolve.Registry, st *store.Store, log *slog.Logger) *Service {
	return &Service{source: source, reg: reg, store: st, log: log, results: map[string]Entry{}, last: map[string]store.HistoryEntry{}}
}

// SetAfterCheck installs a hook that runs after each successfully checked image.
func (s *Service) SetAfterCheck(fn AfterCheck) { s.after = fn }

// SetAfterRun installs a hook that runs after each run that finished without a run error.
func (s *Service) SetAfterRun(fn AfterRun) { s.ran = fn }

// UseRulesFile loads per-container rules from the config file at path now, and again at
// the start of every check run, so edits apply without a restart. A file that fails to
// load is logged and reported by RulesError; the last good one stays in use.
func (s *Service) UseRulesFile(path string) {
	s.rulesPath = path
	s.reloadRules()
}

func (s *Service) reloadRules() {
	if s.rulesPath == "" {
		return
	}
	f, err := rules.Load(s.rulesPath)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.rulesErr = err.Error()
		s.log.Warn("config file not loaded, keeping the previous rules", "path", s.rulesPath, "err", err)
		return
	}
	s.rules, s.rulesErr = f, ""
}

// Rules returns a container's effective rules: its labels merged with the config file.
func (s *Service) Rules(c docker.Container) rules.Rules {
	s.mu.RLock()
	f := s.rules
	s.mu.RUnlock()
	return f.For(c.Name, c.ImageRef, c.Labels)
}

// RulesError says why the config file couldn't be loaded, "" if it could (or there is none).
func (s *Service) RulesError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rulesErr
}

// Load restores results, the last run's status and each container's newest history
// row from the store.
func (s *Service) Load(ctx context.Context) error {
	rows, err := s.store.Results(ctx)
	if err != nil {
		return fmt.Errorf("load results: %w", err)
	}
	last, err := s.store.LastHistory(ctx)
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}
	run, ok, err := s.store.LastFinishedRun(ctx)
	if err != nil {
		return fmt.Errorf("load last run: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range rows {
		s.results[r.ContainerName] = fromRow(r)
	}
	for name, h := range last {
		s.last[name] = h
	}
	if ok {
		started := run.StartedAt
		s.status = Status{Trigger: run.Trigger, StartedAt: &started, FinishedAt: run.FinishedAt, Done: run.Images, Total: run.Images, Err: run.Error}
	}
	return nil
}

// LastFinished is when the last run completed, zero if never.
func (s *Service) LastFinished() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.status.FinishedAt == nil {
		return time.Time{}
	}
	return *s.status.FinishedAt
}

// Schedule runs checks on a cron spec ("0 */6 * * *", "@daily", "@every 3h").
// Times are in the server's time zone; prefix with "CRON_TZ=Europe/Amsterdam" to override.
func (s *Service) Schedule(ctx context.Context, spec string) error {
	c := cron.New()
	id, err := c.AddFunc(spec, func() { s.Start(ctx, TriggerSchedule) })
	if err != nil {
		return fmt.Errorf("schedule %q: %w", spec, err)
	}
	s.mu.Lock()
	s.cron, s.cronID = c, id
	s.mu.Unlock()
	c.Start()
	go func() {
		<-ctx.Done()
		c.Stop()
	}()
	return nil
}

// Start begins a check run in the background. It returns false if one is already running.
func (s *Service) Start(ctx context.Context, trigger string) bool {
	s.mu.Lock()
	if s.status.Running {
		s.mu.Unlock()
		return false
	}
	now := time.Now()
	s.status = Status{Running: true, Trigger: trigger, StartedAt: &now}
	done := make(chan struct{})
	s.runDone = done
	s.mu.Unlock()

	go func() {
		defer close(done)
		s.run(ctx, trigger, now)
	}()
	return true
}

// Wait blocks until the running check run, if any, has ended (including writing its finish
// time), or until ctx ends, in which case it returns ctx.Err(). Used at shutdown before the
// database closes.
func (s *Service) Wait(ctx context.Context) error {
	s.mu.RLock()
	done := s.runDone
	s.mu.RUnlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := s.status
	if s.cron != nil {
		if next := s.cron.Entry(s.cronID).Next; !next.IsZero() {
			st.NextRunAt = &next
		}
	}
	return st
}

// Result returns the latest result for a container, if it was checked against the
// image the container runs now and with its current rules. After a pull and recreate,
// or a label change (which recreates the container, often with the same image), the
// old result is stale. Ignored containers have none.
func (s *Service) Result(c docker.Container) (Entry, bool) {
	rs := s.Rules(c)
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.results[c.Name]
	if !ok || rs.Ignore || !e.valid(c.ImageRef, c.Image.ID, rs.Fingerprint()) {
		return Entry{}, false
	}
	return e, true
}

// Unchecked counts the containers that should have a result but have no valid one:
// never checked, or checked against another image or other rules. Ignored ones don't count.
// A failed check counts as checked, whether or not it kept an earlier result, so a
// failing refresh doesn't force a check at every startup.
func (s *Service) Unchecked(ctx context.Context) (int, error) {
	list, err := s.source.Containers(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range list {
		if _, ok := s.Result(c); !ok && !s.Rules(c).Ignore {
			n++
		}
	}
	return n, nil
}

// group is a set of containers that share one check: same image, same rules.
type group struct {
	containers []docker.Container
	rules      rules.Rules
}

func (s *Service) run(ctx context.Context, trigger string, started time.Time) {
	runID, err := s.store.StartRun(ctx, trigger, started)
	if err != nil {
		s.log.Warn("check: can't record run", "err", err)
	}

	s.reloadRules()
	list, err := s.source.Containers(ctx)
	if err != nil {
		s.finish(ctx, runID, 0, err.Error())
		s.log.Warn("check failed", "trigger", trigger, "err", err)
		return
	}

	// Containers running the same image with the same rules share one check.
	// Ignored containers aren't checked and lose any result they had.
	groups := map[string]*group{}
	var order []string
	keep := make([]string, 0, len(list))
	for _, c := range list {
		rs := s.Rules(c)
		if rs.Ignore {
			continue
		}
		keep = append(keep, c.Name)
		key := c.ImageRef + "@" + c.Image.ID + "#" + rs.Fingerprint()
		g, seen := groups[key]
		if !seen {
			g = &group{rules: rs}
			groups[key] = g
			order = append(order, key)
		}
		g.containers = append(g.containers, c)
	}
	s.mu.Lock()
	s.status.Total = len(order)
	s.mu.Unlock()

	jobs := make(chan *group)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for g := range jobs {
				first := g.containers[0]
				entry := s.checkOne(ctx, first, g.rules)
				if entry.Err == "" && s.after != nil {
					actx, cancel := context.WithTimeout(ctx, afterCheckTimeout)
					s.after(actx, first, g.rules, entry.Result)
					cancel()
				}
				for _, c := range g.containers {
					s.save(ctx, c.Name, entry, runID)
				}
				s.mu.Lock()
				s.status.Done++
				s.mu.Unlock()
			}
		}()
	}
	for _, key := range order {
		jobs <- groups[key]
	}
	close(jobs)
	wg.Wait()

	// Forget containers that no longer exist or are ignored now.
	current := make(map[string]bool, len(keep))
	for _, name := range keep {
		current[name] = true
	}
	if err := s.store.KeepResults(ctx, keep); err != nil {
		s.log.Warn("check: can't prune results", "err", err)
	}
	s.mu.Lock()
	for name := range s.results {
		if !current[name] {
			delete(s.results, name)
		}
	}
	s.mu.Unlock()

	s.finish(ctx, runID, len(order), "")
	s.log.Info("check finished", "trigger", trigger, "images", len(order), "containers", len(list),
		"ignored", len(list)-len(keep), "took", time.Since(started).Round(time.Millisecond))
	if s.ran != nil {
		s.ran(ctx, trigger)
	}
}

func (s *Service) checkOne(ctx context.Context, c docker.Container, rs rules.Rules) Entry {
	entry := Entry{ImageRef: c.ImageRef, ImageID: c.Image.ID, Rules: rs.Fingerprint()}
	if c.Image.Missing {
		entry.Err, entry.FailedAt = "the container's image is no longer on this host", time.Now()
		if c.Image.InspectError != "" {
			entry.Err = "inspecting the image failed: " + c.Image.InspectError
		}
		return entry
	}
	ctx, cancel := context.WithTimeout(ctx, containerTimeout)
	defer cancel()
	r, err := resolve.Resolve(ctx, s.reg, c, resolveOptions(rs))
	if err != nil {
		entry.Err, entry.ErrKind, entry.FailedAt = err.Error(), string(registry.KindOf(err)), time.Now()
		s.log.Debug("check: container failed", "container", c.Name, "err", err)
		return entry
	}
	entry.Result, entry.CheckedAt = r, time.Now()
	return entry
}

func resolveOptions(rs rules.Rules) resolve.Options {
	return resolve.Options{Prerelease: rs.Prerelease, TagPattern: rs.TagPattern, Pin: rs.Pin, HoldMajor: !rs.AllowMajor}
}

func (s *Service) save(ctx context.Context, name string, e Entry, runID int64) {
	s.mu.Lock()
	// A failure (e.g. no network yet after waking from sleep) keeps the last good result
	// for this image and rules, and only records that refreshing it failed.
	if prev, ok := s.results[name]; ok && e.Err != "" && !prev.CheckedAt.IsZero() && prev.valid(e.ImageRef, e.ImageID, e.Rules) {
		prev.Err, prev.ErrKind, prev.FailedAt = e.Err, e.ErrKind, e.FailedAt
		e = prev
	}
	s.results[name] = e
	h, changed := s.historyFor(name, e)
	s.mu.Unlock()
	if err := s.store.SaveResult(ctx, toRow(name, e, runID)); err != nil {
		s.log.Warn("check: can't save result", "container", name, "err", err)
	}
	if changed {
		if err := s.store.AddHistory(ctx, h); err != nil {
			s.log.Warn("check: can't record history", "container", name, "err", err)
		}
	}
}

// historyFor returns the history row a successful check of a container adds, if any: a
// first row when there's none yet, a change row when the image differs from the newest
// row's. Comparing with the last history row, not the last result, means a failed check
// in between doesn't lose the from-version. Call with s.mu held; it updates s.last.
func (s *Service) historyFor(name string, e Entry) (store.HistoryEntry, bool) {
	if e.Err != "" {
		return store.HistoryEntry{}, false
	}
	prev, seen := s.last[name]
	if seen && prev.ImageID == e.ImageID {
		return store.HistoryEntry{}, false
	}
	h := store.HistoryEntry{ContainerName: name, ImageID: e.ImageID, Running: e.Result.Running, DetectedAt: e.CheckedAt, Via: "check"}
	if seen {
		h.PrevImageID, h.PrevRunning = prev.ImageID, prev.Running
	}
	s.last[name] = h
	return h, true
}

func (s *Service) finish(ctx context.Context, runID int64, images int, errMsg string) {
	now := time.Now()
	s.mu.Lock()
	s.status.Running = false
	s.status.FinishedAt = &now
	s.status.Err = errMsg
	s.mu.Unlock()
	if runID != 0 {
		// The run's own context may be cancelled at shutdown; still record how it ended.
		if err := s.store.FinishRun(context.WithoutCancel(ctx), runID, now, images, errMsg); err != nil {
			s.log.Warn("check: can't record run end", "err", err)
		}
	}
}

func toRow(name string, e Entry, runID int64) store.Result {
	r := e.Result
	return store.Result{
		ContainerName: name, ImageRef: e.ImageRef, ImageID: e.ImageID,
		Running: r.Running, Source: string(r.Source), Latest: r.Latest, LatestInMajor: r.LatestInMajor,
		Class: string(r.Class), Floating: r.Floating, NewImage: r.NewImage,
		Held: r.Held, HeldBy: r.HeldBy, TagDigest: r.TagDigest, Rules: e.Rules,
		Error: e.Err, ErrorKind: e.ErrKind, CheckedAt: e.CheckedAt, FailedAt: e.FailedAt, TagsFrom: r.TagsFrom, RunID: runID,
	}
}

func fromRow(r store.Result) Entry {
	return Entry{
		ImageRef: r.ImageRef, ImageID: r.ImageID, Rules: r.Rules,
		Err: r.Error, ErrKind: r.ErrorKind, CheckedAt: r.CheckedAt, FailedAt: r.FailedAt,
		Result: resolve.Result{
			Running: r.Running, Source: resolve.Source(r.Source), Latest: r.Latest, LatestInMajor: r.LatestInMajor,
			Class: version.Class(r.Class), Floating: r.Floating, NewImage: r.NewImage, Held: r.Held, HeldBy: r.HeldBy, TagDigest: r.TagDigest,
			TagsFrom: r.TagsFrom,
		},
	}
}

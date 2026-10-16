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
	"github.com/bartvs98/tidemark/internal/resolve"
	"github.com/bartvs98/tidemark/internal/store"
	"github.com/bartvs98/tidemark/internal/version"
)

const (
	workers          = 6
	containerTimeout = 2 * time.Minute
)

// Triggers say what started a run.
const (
	TriggerStartup  = "startup"
	TriggerSchedule = "schedule"
	TriggerManual   = "manual"
)

type Entry struct {
	ImageRef  string // what was checked: a result only applies while the
	ImageID   string // container still runs this exact image
	Result    resolve.Result
	Err       string
	CheckedAt time.Time
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

type Service struct {
	source docker.Source
	reg    resolve.Registry
	store  *store.Store
	log    *slog.Logger

	mu      sync.RWMutex
	results map[string]Entry // by container name
	status  Status
	cron    *cron.Cron
	cronID  cron.EntryID
}

func New(source docker.Source, reg resolve.Registry, st *store.Store, log *slog.Logger) *Service {
	return &Service{source: source, reg: reg, store: st, log: log, results: map[string]Entry{}}
}

// Load restores results and the last run's status from the store.
func (s *Service) Load(ctx context.Context) error {
	rows, err := s.store.Results(ctx)
	if err != nil {
		return fmt.Errorf("load results: %w", err)
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
	s.mu.Unlock()

	go s.run(ctx, trigger, now)
	return true
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
// image the container runs now. After a pull and recreate, the old result is stale.
func (s *Service) Result(c docker.Container) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.results[c.Name]
	if !ok || e.ImageRef != c.ImageRef || e.ImageID != c.Image.ID {
		return Entry{}, false
	}
	return e, true
}

func (s *Service) run(ctx context.Context, trigger string, started time.Time) {
	runID, err := s.store.StartRun(ctx, trigger, started)
	if err != nil {
		s.log.Warn("check: can't record run", "err", err)
	}

	list, err := s.source.Containers(ctx)
	if err != nil {
		s.finish(ctx, runID, 0, err.Error())
		s.log.Warn("check failed", "trigger", trigger, "err", err)
		return
	}

	// Containers running the same image share one check.
	groups := map[string][]docker.Container{}
	var order []string
	for _, c := range list {
		key := c.ImageRef + "@" + c.Image.ID
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}
		groups[key] = append(groups[key], c)
	}
	s.mu.Lock()
	s.status.Total = len(order)
	s.mu.Unlock()

	jobs := make(chan []docker.Container)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for group := range jobs {
				entry := s.checkOne(ctx, group[0])
				for _, c := range group {
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

	// Forget containers that no longer exist.
	names := make([]string, 0, len(list))
	current := make(map[string]bool, len(list))
	for _, c := range list {
		names = append(names, c.Name)
		current[c.Name] = true
	}
	if err := s.store.KeepResults(ctx, names); err != nil {
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
		"took", time.Since(started).Round(time.Millisecond))
}

func (s *Service) checkOne(ctx context.Context, c docker.Container) Entry {
	entry := Entry{ImageRef: c.ImageRef, ImageID: c.Image.ID, CheckedAt: time.Now()}
	if c.Image.Missing {
		entry.Err = "the container's image is no longer on this host"
		return entry
	}
	ctx, cancel := context.WithTimeout(ctx, containerTimeout)
	defer cancel()
	r, err := resolve.Resolve(ctx, s.reg, c)
	if err != nil {
		entry.Err = err.Error()
		s.log.Debug("check: container failed", "container", c.Name, "err", err)
	}
	entry.Result = r
	return entry
}

func (s *Service) save(ctx context.Context, name string, e Entry, runID int64) {
	s.mu.Lock()
	s.results[name] = e
	s.mu.Unlock()
	if err := s.store.SaveResult(ctx, toRow(name, e, runID)); err != nil {
		s.log.Warn("check: can't save result", "container", name, "err", err)
	}
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
		Error: e.Err, CheckedAt: e.CheckedAt, RunID: runID,
	}
}

func fromRow(r store.Result) Entry {
	return Entry{
		ImageRef: r.ImageRef, ImageID: r.ImageID, Err: r.Error, CheckedAt: r.CheckedAt,
		Result: resolve.Result{
			Running: r.Running, Source: resolve.Source(r.Source), Latest: r.Latest, LatestInMajor: r.LatestInMajor,
			Class: version.Class(r.Class), Floating: r.Floating, NewImage: r.NewImage,
		},
	}
}

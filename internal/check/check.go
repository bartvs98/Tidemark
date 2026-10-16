// Package check runs update checks for all containers in the background and keeps
// the latest result per container. Results live in memory for now; step 3 of the
// roadmap persists them and adds a schedule.
package check

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/resolve"
)

const (
	workers          = 6
	containerTimeout = 2 * time.Minute
)

type Entry struct {
	Result    resolve.Result
	Err       string
	CheckedAt time.Time
}

type Status struct {
	Running    bool
	StartedAt  *time.Time
	FinishedAt *time.Time
	Done       int // images checked so far in the current/last run
	Total      int
	Err        string // the run itself failed (e.g. Docker unreachable)
}

type Service struct {
	source docker.Source
	reg    resolve.Registry
	log    *slog.Logger

	mu      sync.RWMutex
	results map[string]Entry // by container ID
	status  Status
}

func New(source docker.Source, reg resolve.Registry, log *slog.Logger) *Service {
	return &Service{source: source, reg: reg, log: log, results: map[string]Entry{}}
}

// Start begins a check run in the background. It returns false if one is already running.
func (s *Service) Start(ctx context.Context) bool {
	s.mu.Lock()
	if s.status.Running {
		s.mu.Unlock()
		return false
	}
	now := time.Now()
	s.status = Status{Running: true, StartedAt: &now}
	s.mu.Unlock()

	go s.run(ctx)
	return true
}

func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *Service) Result(containerID string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.results[containerID]
	return e, ok
}

func (s *Service) run(ctx context.Context) {
	start := time.Now()
	list, err := s.source.Containers(ctx)
	if err != nil {
		s.finish(err.Error())
		s.log.Warn("check failed", "err", err)
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
				s.mu.Lock()
				for _, c := range group {
					s.results[c.ID] = entry
				}
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
	current := make(map[string]bool, len(list))
	for _, c := range list {
		current[c.ID] = true
	}
	s.mu.Lock()
	for id := range s.results {
		if !current[id] {
			delete(s.results, id)
		}
	}
	s.mu.Unlock()

	s.finish("")
	s.log.Info("check finished", "images", len(order), "containers", len(list), "took", time.Since(start).Round(time.Millisecond))
}

func (s *Service) checkOne(ctx context.Context, c docker.Container) Entry {
	entry := Entry{CheckedAt: time.Now()}
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

func (s *Service) finish(errMsg string) {
	now := time.Now()
	s.mu.Lock()
	s.status.Running = false
	s.status.FinishedAt = &now
	s.status.Err = errMsg
	s.mu.Unlock()
}

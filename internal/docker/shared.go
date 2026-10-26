package docker

import (
	"context"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Shared returns a Source that lists through src at most once per ttl, with concurrent
// callers sharing one list. It's for the API's reads (dashboard polls, detail pages, the
// stats page, the after-run hook), which all want the same inventory within a second or
// two; check runs list through src itself, so they always start from a fresh one.
// Failures aren't cached.
func Shared(src Source, ttl time.Duration) Source {
	return &shared{src: src, ttl: ttl}
}

type shared struct {
	src    Source
	ttl    time.Duration
	flight singleflight.Group

	mu   sync.Mutex
	list []Container
	at   time.Time
}

func (s *shared) Info() SourceInfo { return s.src.Info() }

// Containers returns a copy of the slice; the containers themselves are shared and must
// not be modified.
func (s *shared) Containers(ctx context.Context) ([]Container, error) {
	s.mu.Lock()
	list, fresh := s.list, s.list != nil && time.Since(s.at) < s.ttl
	s.mu.Unlock()
	if fresh {
		return slices.Clone(list), nil
	}
	// Detached, so one caller giving up doesn't fail the list for the others; each caller
	// stops waiting when its own ctx ends.
	ch := s.flight.DoChan("", func() (any, error) {
		at := time.Now()
		list, err := s.src.Containers(context.WithoutCancel(ctx))
		if err == nil {
			s.mu.Lock()
			s.list, s.at = list, at
			s.mu.Unlock()
		}
		return list, err
	})
	select {
	case r := <-ch:
		if r.Err != nil {
			return nil, r.Err
		}
		return slices.Clone(r.Val.([]Container)), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

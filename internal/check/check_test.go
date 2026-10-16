package check

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
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
}

func (r *countingRegistry) Tags(_ context.Context, ref imageref.Ref) ([]string, error) {
	if r.block != nil {
		<-r.block
	}
	r.mu.Lock()
	r.calls[ref.Repository]++
	r.mu.Unlock()
	if ref.Repository == "team/broken" {
		return nil, errors.New("registry exploded")
	}
	return []string{"1.0.0", "1.1.0"}, nil
}

func (r *countingRegistry) Digest(context.Context, imageref.Ref, string) (string, error) {
	return "", errors.New("unused")
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

func ctr(id, image, imageID string) docker.Container {
	return docker.Container{ID: id, Name: id, ImageRef: image, Image: docker.Image{ID: imageID}}
}

func TestRun(t *testing.T) {
	reg := &countingRegistry{calls: map[string]int{}}
	src := listSource{list: []docker.Container{
		ctr("a", "team/app:1.0.0", "sha256:1"),
		ctr("b", "team/app:1.0.0", "sha256:1"), // same image: checked once
		ctr("c", "team/broken:1.0.0", "sha256:2"),
		{ID: "d", Name: "d", ImageRef: "team/gone:1", Image: docker.Image{ID: "sha256:3", Missing: true}},
	}}
	s := New(src, reg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if !s.Start(context.Background()) {
		t.Fatal("first Start should start a run")
	}
	st := waitDone(t, s)
	if st.Done != 3 || st.Total != 3 || st.FinishedAt == nil || st.Err != "" {
		t.Errorf("status = %+v", st)
	}
	if reg.calls["team/app"] != 1 {
		t.Errorf("team/app checked %d times, want 1", reg.calls["team/app"])
	}

	for _, id := range []string{"a", "b"} {
		e, ok := s.Result(id)
		if !ok || e.Err != "" || e.Result.Running != "1.0.0" || e.Result.Latest != "1.1.0" || e.Result.Class != version.Minor {
			t.Errorf("%s = %+v", id, e)
		}
	}
	if e, _ := s.Result("c"); e.Err != "registry exploded" {
		t.Errorf("c err = %q", e.Err)
	}
	if e, _ := s.Result("d"); e.Err == "" {
		t.Error("missing image should be reported")
	}
}

func TestStartWhileRunning(t *testing.T) {
	reg := &countingRegistry{calls: map[string]int{}, block: make(chan struct{})}
	s := New(listSource{list: []docker.Container{ctr("a", "team/app:1.0.0", "sha256:1")}}, reg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.Start(context.Background())
	if s.Start(context.Background()) {
		t.Error("second Start while running should be refused")
	}
	close(reg.block)
	waitDone(t, s)
}

func TestSourceError(t *testing.T) {
	s := New(listSource{err: errors.New("can't reach Docker")}, &countingRegistry{calls: map[string]int{}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.Start(context.Background())
	if st := waitDone(t, s); st.Err != "can't reach Docker" {
		t.Errorf("status = %+v", st)
	}
}

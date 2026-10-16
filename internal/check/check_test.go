package check

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
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
	if e, _ := s.Result(c); e.Err != "registry exploded" {
		t.Errorf("c err = %q", e.Err)
	}
	if e, _ := s.Result(d); e.Err == "" {
		t.Error("missing image should be reported")
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

func TestSchedule(t *testing.T) {
	s := New(listSource{}, &countingRegistry{calls: map[string]int{}}, openStore(t, filepath.Join(t.TempDir(), "t.db")), quiet)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Schedule(ctx, "every so often"); err == nil {
		t.Error("invalid spec should be rejected")
	}
	if err := s.Schedule(ctx, "0 */6 * * *"); err != nil {
		t.Fatal(err)
	}
	next := s.Status().NextRunAt
	if next == nil || !next.After(time.Now()) || next.After(time.Now().Add(6*time.Hour)) || next.Minute() != 0 || next.Hour()%6 != 0 {
		t.Errorf("next run = %v", next)
	}
}

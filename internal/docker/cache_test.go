package docker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// cacheEngine serves a container list and image inspects whose answers the test changes,
// counting inspects per image.
type cacheEngine struct {
	mu          sync.Mutex
	containers  string // JSON
	summaries   string // JSON; "" answers 403, like a proxy that doesn't allow the list
	inspects    map[string]int
	inspectBody map[string]string
}

func (e *cacheEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case r.URL.Path == "/containers/json":
		_, _ = w.Write([]byte(e.containers))
	case r.URL.Path == "/images/json":
		if e.summaries == "" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"forbidden"}`))
			return
		}
		_, _ = w.Write([]byte(e.summaries))
	case strings.HasPrefix(r.URL.Path, "/images/"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/images/"), "/json")
		e.inspects[id]++
		if body, ok := e.inspectBody[id]; ok {
			_, _ = w.Write([]byte(body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"No such image"}`))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (e *cacheEngine) set(f func()) {
	e.mu.Lock()
	defer e.mu.Unlock()
	f()
}

// Inspects are cached by image ID, but tags and digests always come from the image list
// (a pull moves them), images no container uses are dropped, and without the image list
// every image is inspected again.
func TestClientImageCache(t *testing.T) {
	e := &cacheEngine{
		containers: `[{"Id":"1","Names":["/redis"],"Image":"redis:7","ImageID":"sha256:a"}]`,
		summaries:  `[{"Id":"sha256:a","RepoTags":["redis:7"],"RepoDigests":["redis@sha256:d1"]}]`,
		inspects:   map[string]int{},
		inspectBody: map[string]string{
			"sha256:a": `{"Id":"sha256:a","RepoTags":["redis:7"],"RepoDigests":["redis@sha256:d1"],"Config":{"Labels":{"x":"1"},"Env":["REDIS_VERSION=7.4.1"]}}`,
			"sha256:b": `{"Id":"sha256:b","RepoTags":["redis:7"],"RepoDigests":["redis@sha256:d2"]}`,
		},
	}
	srv := httptest.NewServer(e)
	defer srv.Close()
	c, _ := NewClient("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	ctx := context.Background()
	list := func() Container {
		t.Helper()
		l, err := c.Containers(ctx)
		if err != nil || len(l) != 1 {
			t.Fatalf("list = %+v, %v", l, err)
		}
		return l[0]
	}

	list()
	got := list()
	if e.inspects["sha256:a"] != 1 {
		t.Errorf("inspects = %d, want 1 (cached)", e.inspects["sha256:a"])
	}
	if got.Image.VersionEnv["REDIS_VERSION"] != "7.4.1" || got.Image.Labels["x"] != "1" {
		t.Errorf("cached image lost its config: %+v", got.Image)
	}

	// `docker pull redis:7` without recreating: the old image loses its tag, and Docker
	// reports the container's image as the bare ID.
	e.set(func() {
		e.containers = `[{"Id":"1","Names":["/redis"],"Image":"sha256:a","ImageID":"sha256:a"}]`
		e.summaries = `[{"Id":"sha256:a","RepoTags":["<none>:<none>"],"RepoDigests":["redis@sha256:d1"]},{"Id":"sha256:b","RepoTags":["redis:7"]}]`
	})
	got = list()
	if len(got.Image.RepoTags) != 0 || got.ImageRef != "redis@sha256:d1" || e.inspects["sha256:a"] != 1 {
		t.Errorf("after a pull: ref %q, tags %v, inspects %d", got.ImageRef, got.Image.RepoTags, e.inspects["sha256:a"])
	}

	// Recreated on the new image: the old one is dropped from the cache.
	e.set(func() { e.containers = `[{"Id":"2","Names":["/redis"],"Image":"redis:7","ImageID":"sha256:b"}]` })
	list()
	c.mu.Lock()
	ids := len(c.images)
	_, old := c.images["sha256:a"]
	c.mu.Unlock()
	if ids != 1 || old {
		t.Errorf("cache keeps %d images (old one: %v), want only the new one", ids, old)
	}

	// No image list: inspect again rather than trust cached tags.
	e.set(func() { e.summaries = "" })
	list()
	if e.inspects["sha256:b"] != 2 {
		t.Errorf("inspects without the image list = %d, want 2", e.inspects["sha256:b"])
	}
}

type countingSource struct {
	calls atomic.Int32
	err   error
	gate  chan struct{} // if set, Containers waits for it
}

func (s *countingSource) Containers(context.Context) ([]Container, error) {
	s.calls.Add(1)
	if s.gate != nil {
		<-s.gate
	}
	if s.err != nil {
		return nil, s.err
	}
	return []Container{{Name: "a"}, {Name: "b"}}, nil
}

func (s *countingSource) Info() SourceInfo { return SourceInfo{Kind: "test"} }

func TestShared(t *testing.T) {
	src := &countingSource{gate: make(chan struct{})}
	s := Shared(src, time.Hour)
	ctx := context.Background()

	// Concurrent callers share one list.
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l, err := s.Containers(ctx); err != nil || len(l) != 2 {
				t.Errorf("list = %v, %v", l, err)
			}
		}()
	}
	for src.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond) // let the others join the flight
	close(src.gate)
	wg.Wait()
	if n := src.calls.Load(); n != 1 {
		t.Errorf("source called %d times, want 1", n)
	}

	// Within the TTL: no new call, and the caller gets its own slice.
	l, _ := s.Containers(ctx)
	l[0].Name = "changed"
	if l2, _ := s.Containers(ctx); l2[0].Name != "a" || src.calls.Load() != 1 {
		t.Errorf("cached list = %v after %d calls", l2, src.calls.Load())
	}

	// Failures aren't cached.
	failing := &countingSource{err: errors.New("can't reach Docker")}
	f := Shared(failing, time.Hour)
	_, _ = f.Containers(ctx)
	if _, err := f.Containers(ctx); err == nil || failing.calls.Load() != 2 {
		t.Errorf("err = %v after %d calls, want a retry", err, failing.calls.Load())
	}

	// Past the TTL: listed again.
	short := &countingSource{}
	sh := Shared(short, time.Millisecond)
	_, _ = sh.Containers(ctx)
	time.Sleep(5 * time.Millisecond)
	_, _ = sh.Containers(ctx)
	if short.calls.Load() != 2 {
		t.Errorf("calls after the TTL = %d, want 2", short.calls.Load())
	}
}

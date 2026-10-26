package registry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/bartvs98/tidemark/internal/imageref"
)

// testRegistry is an in-memory OCI registry with a few tags pushed, counting requests.
func testRegistry(t *testing.T) (host string, digests map[string]string, requests *atomic.Int32) {
	return hookedRegistry(t, nil)
}

// hookedRegistry is testRegistry where intercept, if set, sees each request after the
// tags are pushed and can answer it itself (returning true) instead of the registry.
func hookedRegistry(t *testing.T, intercept func(w http.ResponseWriter, r *http.Request) bool) (host string, digests map[string]string, requests *atomic.Int32) {
	t.Helper()
	requests = &atomic.Int32{}
	reg := registry.New()
	var pushed atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if pushed.Load() && intercept != nil && intercept(w, r) {
			return
		}
		reg.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	host = strings.TrimPrefix(srv.URL, "http://")

	digests = map[string]string{}
	for _, tag := range []string{"1.0.0", "1.1.0", "latest"} {
		img, err := random.Image(64, 1)
		if err != nil {
			t.Fatal(err)
		}
		ref, _ := name.NewTag(host+"/team/app:"+tag, name.Insecure)
		if err := remote.Write(ref, img); err != nil {
			t.Fatal(err)
		}
		d, _ := img.Digest()
		digests[tag] = d.String()
	}
	requests.Store(0)
	pushed.Store(true)
	return host, digests, requests
}

// fastRetries makes go-containerregistry's retries of 429/5xx answers instant.
func fastRetries(c *Client) *Client {
	c.extra = []remote.Option{remote.WithRetryBackoff(remote.Backoff{Duration: time.Millisecond, Steps: 1})}
	return c
}

// tagQueries records the query of every tag list request.
type tagQueries struct {
	mu   sync.Mutex
	list []string
}

func (q *tagQueries) add(r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/tags/list") {
		q.mu.Lock()
		q.list = append(q.list, r.URL.RawQuery)
		q.mu.Unlock()
	}
}

// go-containerregistry asks for 1000 tags per page; GHCR would send 100 without it, so a
// repo with thousands of signature tags would take dozens of requests. Guards the default.
func TestTagsPageSize(t *testing.T) {
	var q tagQueries
	host, _, _ := hookedRegistry(t, func(_ http.ResponseWriter, r *http.Request) bool { q.add(r); return false })
	ref, _ := imageref.Parse(host + "/team/app:1.0.0")
	if _, err := New("t").Tags(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(q.list, []string{"n=1000"}) {
		t.Errorf("tag list queries = %q, want one with n=1000", q.list)
	}
}

// Rate-limited: an older persisted list is returned with a StaleError, and isn't cached
// as fresh. Without one, the error alone. A 429 is asked once (no retries), and the
// outcome is reused for a few minutes without asking again; a later success replaces it.
func TestStaleTagsOnRateLimit(t *testing.T) {
	var lists atomic.Int32
	var allow atomic.Bool
	host, _, _ := hookedRegistry(t, func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/tags/list") {
			lists.Add(1)
			if !allow.Load() {
				http.Error(w, `{"errors":[{"code":"TOOMANYREQUESTS"}]}`, http.StatusTooManyRequests)
				return true
			}
		}
		return false
	})
	ref, _ := imageref.Parse(host + "/team/app:1.0.0")
	key := host + "/team/app"
	ctx := context.Background()

	c := New("t") // default retry settings: a retried 429 would show as more requests
	tags, err := c.Tags(ctx, ref)
	if KindOf(err) != KindRateLimit || errors.As(err, new(*StaleError)) || tags != nil {
		t.Errorf("without a cached list: tags=%v err=%v kind=%q", tags, err, KindOf(err))
	}
	if n := lists.Load(); n != 1 {
		t.Errorf("429 asked %d times, want 1 (no retries)", n)
	}
	if _, err := c.Tags(ctx, ref); KindOf(err) != KindRateLimit || lists.Load() != 1 {
		t.Errorf("again: err=%v, %d requests; the rate limit must be remembered", err, lists.Load())
	}

	at := time.Now().Add(-5 * time.Hour).UTC()
	tc := &memTagCache{tags: map[string][]string{key: {"1.0.0", "old"}}, at: map[string]time.Time{key: at}}
	c = New("t", WithTagCache(tc))
	lists.Store(0)
	tags, err = c.Tags(ctx, ref)
	var stale *StaleError
	if !errors.As(err, &stale) || !stale.At.Equal(at) || KindOf(err) != KindRateLimit || !slices.Equal(tags, []string{"1.0.0", "old"}) {
		t.Fatalf("stale: tags=%v err=%v", tags, err)
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("stale message = %q", err)
	}
	tags, err = c.Tags(ctx, ref)
	if !errors.As(err, &stale) || !stale.At.Equal(at) || !slices.Equal(tags, []string{"1.0.0", "old"}) || lists.Load() != 1 {
		t.Errorf("second call: tags=%v err=%v, %d requests; the stale outcome must be reused", tags, err, lists.Load())
	}
	if !tc.at[key].Equal(at) {
		t.Error("the stale list must not be saved again")
	}

	// After staleTTL the registry is asked again, and a success replaces the outcome.
	c.mu.Lock()
	l := c.limited[key]
	l.at = time.Now().Add(-staleTTL)
	c.limited[key] = l
	c.mu.Unlock()
	allow.Store(true)
	tags, err = c.Tags(ctx, ref)
	if err != nil || len(tags) != 3 || lists.Load() != 2 {
		t.Errorf("after staleTTL: tags=%v err=%v requests=%d", tags, err, lists.Load())
	}
	if _, ok := c.limited[key]; ok {
		t.Error("a successful fetch must drop the rate-limited outcome")
	}
}

// gatedRegistry holds every request whose path ends in suffix until the returned
// release is called, counting them.
func gatedRegistry(t *testing.T, suffix string) (host string, digests map[string]string, count *atomic.Int32, release func()) {
	t.Helper()
	count = &atomic.Int32{}
	gate := make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	host, digests, _ = hookedRegistry(t, func(_ http.ResponseWriter, r *http.Request) bool {
		if strings.Contains(r.URL.Path, suffix) {
			count.Add(1)
			<-gate
		}
		return false
	})
	return host, digests, count, release
}

// Concurrent callers for the same tag list or digest share one request.
func TestConcurrentCallersShareRequest(t *testing.T) {
	const n = 10
	t.Run("tags", func(t *testing.T) {
		host, _, count, release := gatedRegistry(t, "/tags/list")
		ref, _ := imageref.Parse(host + "/team/app:1.0.0")
		c := New("t")
		var wg sync.WaitGroup
		errs := make(chan error, n)
		for range n {
			wg.Go(func() {
				tags, err := c.Tags(context.Background(), ref)
				if err == nil && len(tags) != 3 {
					err = fmt.Errorf("tags = %v", tags)
				}
				errs <- err
			})
		}
		time.Sleep(100 * time.Millisecond) // let every caller join the flight
		release()
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Error(err)
			}
		}
		if count.Load() != 1 {
			t.Errorf("%d tag list requests, want 1", count.Load())
		}
	})
	t.Run("digest", func(t *testing.T) {
		host, digests, count, release := gatedRegistry(t, "/manifests/latest")
		ref, _ := imageref.Parse(host + "/team/app:latest")
		c := New("t")
		var wg sync.WaitGroup
		errs := make(chan error, n)
		for range n {
			wg.Go(func() {
				d, err := c.Digest(context.Background(), ref, "latest")
				if err == nil && d != digests["latest"] {
					err = fmt.Errorf("digest = %s", d)
				}
				errs <- err
			})
		}
		time.Sleep(100 * time.Millisecond)
		release()
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Error(err)
			}
		}
		if count.Load() != 1 {
			t.Errorf("%d manifest requests, want 1", count.Load())
		}
	})
}

// A caller that gives up stops waiting, but the shared request goes on for the others.
func TestCancelledCallerKeepsSharedRequest(t *testing.T) {
	host, _, count, release := gatedRegistry(t, "/tags/list")
	ref, _ := imageref.Parse(host + "/team/app:1.0.0")
	c := New("t")

	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, err := c.Tags(ctx, ref); first <- err }()
	type result struct {
		tags []string
		err  error
	}
	second := make(chan result, 1)
	time.Sleep(50 * time.Millisecond)
	go func() { tags, err := c.Tags(context.Background(), ref); second <- result{tags, err} }()
	time.Sleep(50 * time.Millisecond)

	cancel()
	select {
	case err := <-first:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("cancelled caller: err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the cancelled caller kept waiting")
	}
	release()
	r := <-second
	if r.err != nil || len(r.tags) != 3 || count.Load() != 1 {
		t.Errorf("other caller: tags=%v err=%v requests=%d", r.tags, r.err, count.Load())
	}
}

func TestFriendly(t *testing.T) {
	const what, host = "ghcr.io/team/app", "ghcr.io"
	notFound := what + ": not found, or private (only public images are supported for now)"
	tests := []struct {
		name string
		err  error
		kind Kind
		msg  string
	}{
		{"not found", &transport.Error{StatusCode: http.StatusNotFound}, KindNotFound, notFound},
		{"unauthorized", &transport.Error{StatusCode: http.StatusUnauthorized}, KindNotFound, notFound},
		{"rate limit", &transport.Error{StatusCode: http.StatusTooManyRequests}, KindRateLimit, what + ": registry rate limit reached, try again later"},
		{"server error", fmt.Errorf("GET: %w", &transport.Error{StatusCode: http.StatusServiceUnavailable}), KindRegistry, what + ": registry error (HTTP 503), try again later"},
		{"deadline", fmt.Errorf("Get %q: %w", "https://ghcr.io/v2/", context.DeadlineExceeded), KindTimeout, what + ": the registry didn't answer in time"},
		{"net timeout", &net.OpError{Op: "read", Err: timeoutErr{}}, KindTimeout, what + ": the registry didn't answer in time"},
		{"dns", &net.OpError{Op: "dial", Err: &net.DNSError{Name: "ghcr.io", Err: "no such host", IsNotFound: true}}, KindNetwork, what + ": can't resolve ghcr.io"},
		{"refused", &net.OpError{Op: "dial", Err: sysErr(syscall.ECONNREFUSED)}, KindNetwork, what + ": can't connect to ghcr.io"},
		{"reset", &net.OpError{Op: "read", Err: sysErr(syscall.ECONNRESET)}, KindNetwork, what + ": can't connect to ghcr.io"},
		{"bad request", &transport.Error{StatusCode: http.StatusBadRequest}, "", ""},
		{"other", errors.New("unexpected EOF"), "", what + ": unexpected EOF"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := friendly(tt.err, what, host)
			if KindOf(got) != tt.kind || (tt.msg != "" && got.Error() != tt.msg) || !errors.Is(got, tt.err) {
				t.Errorf("friendly = %q (kind %q), want %q (kind %q)", got, KindOf(got), tt.msg, tt.kind)
			}
		})
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

// sysErr wraps an errno the way the net package does.
func sysErr(errno syscall.Errno) error { return &os.SyscallError{Syscall: "connect", Err: errno} }

// A registry that isn't there is a network failure, end to end.
func TestUnreachableRegistry(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	host := strings.TrimPrefix(srv.URL, "http://")
	srv.Close()
	ref, _ := imageref.Parse(host + "/team/app:1.0.0")
	_, err := fastRetries(New("t")).Tags(context.Background(), ref)
	if KindOf(err) != KindNetwork || !strings.Contains(err.Error(), "can't connect to "+host) {
		t.Errorf("err = %v, kind %q", err, KindOf(err))
	}
}

func TestTagsAndDigest(t *testing.T) {
	host, digests, requests := testRegistry(t)
	c := New("tidemark-test")
	ref, err := imageref.Parse(host + "/team/app:1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	tags, err := c.Tags(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(tags)
	if !slices.Equal(tags, []string{"1.0.0", "1.1.0", "latest"}) {
		t.Errorf("tags = %v", tags)
	}

	d, err := c.Digest(ctx, ref, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if d != digests["latest"] {
		t.Errorf("digest = %s, want %s", d, digests["latest"])
	}

	// Cached: no new requests.
	before := requests.Load()
	_, _ = c.Tags(ctx, ref)
	_, _ = c.Digest(ctx, ref, "latest")
	if requests.Load() != before {
		t.Errorf("cache miss: %d extra requests", requests.Load()-before)
	}
}

func TestMissingRepoIsFriendly(t *testing.T) {
	host, _, _ := testRegistry(t)
	c := New("tidemark-test")
	ref, _ := imageref.Parse(host + "/team/nope:1")
	_, err := c.Digest(context.Background(), ref, "1")
	if err == nil || !strings.Contains(err.Error(), "not found, or private") {
		t.Errorf("err = %v", err)
	}
}

type memTagCache struct {
	tags map[string][]string
	at   map[string]time.Time
}

func (m *memTagCache) LoadTags(_ context.Context, repo string) ([]string, time.Time, bool) {
	t, ok := m.tags[repo]
	return t, m.at[repo], ok
}

func (m *memTagCache) SaveTags(_ context.Context, repo string, tags []string, at time.Time) error {
	m.tags[repo], m.at[repo] = tags, at
	return nil
}

func TestPersistentTagCache(t *testing.T) {
	host, _, requests := testRegistry(t)
	ref, _ := imageref.Parse(host + "/team/app:1.0.0")
	key := host + "/team/app"
	ctx := context.Background()

	// A fresh persisted list is used without asking the registry (e.g. after a restart).
	tc := &memTagCache{tags: map[string][]string{key: {"cached"}}, at: map[string]time.Time{key: time.Now()}}
	tags, err := New("t", WithTagCache(tc)).Tags(ctx, ref)
	if err != nil || !slices.Equal(tags, []string{"cached"}) || requests.Load() != 0 {
		t.Fatalf("fresh cache: tags=%v err=%v requests=%d", tags, err, requests.Load())
	}

	// A stale one is refreshed from the registry and written back.
	tc.at[key] = time.Now().Add(-2 * time.Hour)
	tags, err = New("t", WithTagCache(tc)).Tags(ctx, ref)
	if err != nil || len(tags) != 3 || requests.Load() == 0 || len(tc.tags[key]) != 3 {
		t.Errorf("stale cache: tags=%v err=%v requests=%d saved=%v", tags, err, requests.Load(), tc.tags[key])
	}
}

// FreshSince (Check now) skips cached tag lists and digests from before its time, in
// memory and in the tag cache, but reuses what was fetched after it.
func TestFreshSince(t *testing.T) {
	host, _, requests := testRegistry(t)
	ref, _ := imageref.Parse(host + "/team/app:1.0.0")
	key := host + "/team/app"
	tc := &memTagCache{tags: map[string][]string{key: {"cached"}}, at: map[string]time.Time{key: time.Now().Add(-time.Minute)}}
	c := New("t", WithTagCache(tc))
	ctx := context.Background()
	if _, err := c.Digest(ctx, ref, "latest"); err != nil {
		t.Fatal(err)
	}
	requests.Store(0)

	fresh := FreshSince(ctx, time.Now())
	tags, err := c.Tags(fresh, ref)
	if err != nil || len(tags) != 3 {
		t.Errorf("tags = %v, %v; want the registry's, not the cached list", tags, err)
	}
	if _, err := c.Digest(fresh, ref, "latest"); err != nil {
		t.Fatal(err)
	}
	asked := requests.Load()
	if asked == 0 {
		t.Fatal("no requests: cached values were used")
	}
	_, _ = c.Tags(fresh, ref)
	_, _ = c.Digest(fresh, ref, "latest")
	if requests.Load() != asked {
		t.Errorf("%d extra requests: what the run fetched itself wasn't reused", requests.Load()-asked)
	}
}

func TestExpiredEntriesAreEvicted(t *testing.T) {
	c := New("tidemark-test", WithTTL(time.Hour))
	old := time.Now().Add(-2 * time.Hour)
	c.tags["a/old"] = cached[[]string]{value: []string{"1"}, at: old}
	c.digests["a/old:1"] = cached[string]{value: "sha256:x", at: old}
	c.limited["a/old"] = limitedTags{at: time.Now().Add(-2 * staleTTL)}

	store(c, c.digests, "a/new:1", "sha256:y")
	if _, ok := c.digests["a/old:1"]; ok || len(c.digests) != 1 {
		t.Errorf("digests = %v, want only the new entry", c.digests)
	}
	store(c, c.tags, "a/new", []string{"2"})
	if _, ok := c.tags["a/old"]; ok || len(c.tags) != 1 {
		t.Errorf("tags = %v, want only the new entry", c.tags)
	}

	// A rate-limited outcome evicts expired ones of other repos.
	host, _, _ := hookedRegistry(t, func(w http.ResponseWriter, r *http.Request) bool {
		w.WriteHeader(http.StatusTooManyRequests)
		return true
	})
	ref, err := imageref.Parse(host + "/team/app:1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	fastRetries(c)
	_, _ = c.Tags(context.Background(), ref)
	if _, ok := c.limited["a/old"]; ok || len(c.limited) != 1 {
		t.Errorf("limited = %v, want only the new outcome", c.limited)
	}
}

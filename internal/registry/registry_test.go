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
// as fresh. Without one, the error alone.
func TestStaleTagsOnRateLimit(t *testing.T) {
	host, _, requests := hookedRegistry(t, func(w http.ResponseWriter, r *http.Request) bool {
		if strings.HasSuffix(r.URL.Path, "/tags/list") {
			http.Error(w, `{"errors":[{"code":"TOOMANYREQUESTS"}]}`, http.StatusTooManyRequests)
			return true
		}
		return false
	})
	ref, _ := imageref.Parse(host + "/team/app:1.0.0")
	key := host + "/team/app"
	ctx := context.Background()

	tags, err := fastRetries(New("t")).Tags(ctx, ref)
	if KindOf(err) != KindRateLimit || errors.As(err, new(*StaleError)) || tags != nil {
		t.Errorf("without a cached list: tags=%v err=%v kind=%q", tags, err, KindOf(err))
	}

	at := time.Now().Add(-5 * time.Hour).UTC()
	tc := &memTagCache{tags: map[string][]string{key: {"1.0.0", "old"}}, at: map[string]time.Time{key: at}}
	c := fastRetries(New("t", WithTagCache(tc)))
	tags, err = c.Tags(ctx, ref)
	var stale *StaleError
	if !errors.As(err, &stale) || !stale.At.Equal(at) || KindOf(err) != KindRateLimit || !slices.Equal(tags, []string{"1.0.0", "old"}) {
		t.Fatalf("stale: tags=%v err=%v", tags, err)
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("stale message = %q", err)
	}
	before := requests.Load()
	if _, err := c.Tags(ctx, ref); !errors.As(err, &stale) || requests.Load() == before {
		t.Errorf("second call: err=%v, %d requests; the stale list must not be cached", err, requests.Load()-before)
	}
	if !tc.at[key].Equal(at) {
		t.Error("the stale list must not be saved again")
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

// Package registry reads tag lists and manifest digests from OCI registries
// (Docker Hub, GHCR, Quay, lscr.io, …) using anonymous, read-only requests.
//
// Digests come from HEAD requests, which don't count towards Docker Hub's pull
// rate limit. Results are cached so a check run doesn't repeat requests.
package registry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/bartvs98/tidemark/internal/imageref"
)

const (
	defaultTTL        = time.Hour
	perRegistryLimit  = 4 // concurrent requests per registry host
	defaultReqTimeout = 60 * time.Second
)

// Kind classifies a registry failure, so the UI can say what went wrong.
type Kind string

const (
	KindRateLimit Kind = "rate-limit" // HTTP 429
	KindNotFound  Kind = "not-found"  // 401/403/404: missing or private
	KindTimeout   Kind = "timeout"    // the registry didn't answer in time
	KindNetwork   Kind = "network"    // DNS or connection failure
	KindRegistry  Kind = "registry"   // 5xx
)

// Error is a classified registry failure; Error() is a short message meant to be shown as is.
type Error struct {
	Kind Kind
	Msg  string
	Err  error
}

func (e *Error) Error() string { return e.Msg }
func (e *Error) Unwrap() error { return e.Err }

// KindOf returns the kind of a classified registry error in err's chain, "" if there is none.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

// StaleError: the registry refused to list tags (rate limit), so Tags returned the list from At.
type StaleError struct {
	At  time.Time
	Err error
}

func (e *StaleError) Error() string { return e.Err.Error() }
func (e *StaleError) Unwrap() error { return e.Err }

// TagCache persists tag lists across restarts (the store implements it).
type TagCache interface {
	LoadTags(ctx context.Context, repository string) ([]string, time.Time, bool)
	SaveTags(ctx context.Context, repository string, tags []string, at time.Time) error
}

type Client struct {
	ttl       time.Duration
	transport http.RoundTripper
	userAgent string
	tagCache  TagCache
	extra     []remote.Option // appended to every request's options (tests: faster retries)

	mu      sync.Mutex
	tags    map[string]cached[[]string]
	digests map[string]cached[string]
	slots   map[string]chan struct{}
}

type cached[T any] struct {
	value T
	at    time.Time
}

type Option func(*Client)

// WithTransport replaces the HTTP transport (tests).
func WithTransport(rt http.RoundTripper) Option { return func(c *Client) { c.transport = rt } }

// WithTagCache adds a persistent second-level cache for tag lists.
func WithTagCache(tc TagCache) Option { return func(c *Client) { c.tagCache = tc } }

// WithTTL sets how long tag lists and digests are cached.
func WithTTL(d time.Duration) Option { return func(c *Client) { c.ttl = d } }

func New(userAgent string, opts ...Option) *Client {
	c := &Client{
		ttl:       defaultTTL,
		transport: remote.DefaultTransport,
		userAgent: userAgent,
		tags:      map[string]cached[[]string]{},
		digests:   map[string]cached[string]{},
		slots:     map[string]chan struct{}{},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Tags lists every tag of the repository. When the registry rate-limits the request
// and the tag cache has an older list, it returns that list with a *StaleError.
func (c *Client) Tags(ctx context.Context, ref imageref.Ref) ([]string, error) {
	key := ref.Registry + "/" + ref.Repository
	if v, ok := lookup(c, c.tags, key); ok {
		return v, nil
	}
	var old []string
	var oldAt time.Time
	haveOld := false
	if c.tagCache != nil {
		old, oldAt, haveOld = c.tagCache.LoadTags(ctx, key)
		if haveOld && time.Since(oldAt) < c.ttl {
			c.mu.Lock()
			c.tags[key] = cached[[]string]{value: old, at: oldAt}
			c.mu.Unlock()
			return old, nil
		}
	}
	repo, err := name.NewRepository(key)
	if err != nil {
		return nil, err
	}
	release, err := c.acquire(ctx, ref.Registry)
	if err != nil {
		return nil, err
	}
	defer release()

	ctx, cancel := context.WithTimeout(ctx, defaultReqTimeout)
	defer cancel()
	// remote.List asks for 1000 tags per page (GHCR returns 100 without n), so even
	// repos with thousands of signature tags (sha256-…) take only a few requests.
	tags, err := remote.List(repo, c.options(ctx)...)
	if err != nil {
		err = friendly(err, key, ref.Registry)
		if haveOld && KindOf(err) == KindRateLimit {
			return old, &StaleError{At: oldAt, Err: err} // not cached: the next check asks again
		}
		return nil, err
	}
	store(c, c.tags, key, tags)
	if c.tagCache != nil {
		_ = c.tagCache.SaveTags(ctx, key, tags, time.Now()) // best effort: the memory cache still works
	}
	return tags, nil
}

// Digest returns the manifest digest (sha256:…) a tag currently points to. For
// multi-platform images that's the index digest, which is also what Docker records
// in an image's RepoDigests after a pull.
func (c *Client) Digest(ctx context.Context, ref imageref.Ref, tag string) (string, error) {
	key := ref.Registry + "/" + ref.Repository + ":" + tag
	if v, ok := lookup(c, c.digests, key); ok {
		return v, nil
	}
	t, err := name.NewTag(key)
	if err != nil {
		return "", err
	}
	release, err := c.acquire(ctx, ref.Registry)
	if err != nil {
		return "", err
	}
	defer release()

	ctx, cancel := context.WithTimeout(ctx, defaultReqTimeout)
	defer cancel()
	desc, err := remote.Head(t, c.options(ctx)...)
	if err != nil {
		return "", friendly(err, key, ref.Registry)
	}
	d := desc.Digest.String()
	store(c, c.digests, key, d)
	return d, nil
}

func (c *Client) options(ctx context.Context) []remote.Option {
	return append([]remote.Option{
		remote.WithContext(ctx),
		remote.WithAuth(authn.Anonymous),
		remote.WithTransport(c.transport),
		remote.WithUserAgent(c.userAgent),
	}, c.extra...)
}

func (c *Client) acquire(ctx context.Context, host string) (func(), error) {
	c.mu.Lock()
	slot, ok := c.slots[host]
	if !ok {
		slot = make(chan struct{}, perRegistryLimit)
		c.slots[host] = slot
	}
	c.mu.Unlock()
	select {
	case slot <- struct{}{}:
		return func() { <-slot }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func lookup[T any](c *Client, m map[string]cached[T], key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := m[key]
	if ok && time.Since(e.at) < c.ttl {
		return e.value, true
	}
	var zero T
	return zero, false
}

func store[T any](c *Client, m map[string]cached[T], key string, v T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m[key] = cached[T]{value: v, at: time.Now()}
}

// friendly turns registry errors into short messages that say what went wrong (and,
// where it helps, what to do), classified by Kind. Errors it doesn't recognise keep
// their own text and have no kind.
func friendly(err error, what, host string) error {
	classified := func(k Kind, format string, args ...any) error {
		return &Error{Kind: k, Msg: what + ": " + fmt.Sprintf(format, args...), Err: err}
	}
	var te *transport.Error
	if errors.As(err, &te) {
		switch code := te.StatusCode; {
		case code == http.StatusUnauthorized, code == http.StatusForbidden, code == http.StatusNotFound:
			return classified(KindNotFound, "not found, or private (only public images are supported for now)")
		case code == http.StatusTooManyRequests:
			return classified(KindRateLimit, "registry rate limit reached, try again later")
		case code >= 500 && code <= 599:
			return classified(KindRegistry, "registry error (HTTP %d), try again later", code)
		}
	}
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) && ne.Timeout() {
		return classified(KindTimeout, "the registry didn't answer in time")
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return classified(KindNetwork, "can't resolve %s", dns.Name)
	}
	// A failed dial covers refused connections on every platform (Windows reports
	// WSAECONNREFUSED, not ECONNREFUSED).
	var op *net.OpError
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) || errors.As(err, &op) && op.Op == "dial" {
		return classified(KindNetwork, "can't connect to %s", host)
	}
	return fmt.Errorf("%s: %w", what, err)
}

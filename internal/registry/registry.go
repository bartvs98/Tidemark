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
	"net/http"
	"sync"
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

// Tags lists every tag of the repository.
func (c *Client) Tags(ctx context.Context, ref imageref.Ref) ([]string, error) {
	key := ref.Registry + "/" + ref.Repository
	if v, ok := lookup(c, c.tags, key); ok {
		return v, nil
	}
	if c.tagCache != nil {
		if tags, at, ok := c.tagCache.LoadTags(ctx, key); ok && time.Since(at) < c.ttl {
			c.mu.Lock()
			c.tags[key] = cached[[]string]{value: tags, at: at}
			c.mu.Unlock()
			return tags, nil
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
	tags, err := remote.List(repo, c.options(ctx)...)
	if err != nil {
		return nil, friendly(err, key)
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
		return "", friendly(err, key)
	}
	d := desc.Digest.String()
	store(c, c.digests, key, d)
	return d, nil
}

func (c *Client) options(ctx context.Context) []remote.Option {
	return []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuth(authn.Anonymous),
		remote.WithTransport(c.transport),
		remote.WithUserAgent(c.userAgent),
	}
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

// friendly turns registry errors into messages that say what to do.
func friendly(err error, what string) error {
	var te *transport.Error
	if errors.As(err, &te) {
		switch te.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
			return fmt.Errorf("%s: not found, or private (only public images are supported for now)", what)
		case http.StatusTooManyRequests:
			return fmt.Errorf("%s: registry rate limit reached, try again later", what)
		}
	}
	return fmt.Errorf("%s: %w", what, err)
}

package changelog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimit(t *testing.T) {
	var calls atomic.Int32
	var auth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rate_limit" {
			http.NotFound(w, r)
			return
		}
		calls.Add(1)
		auth.Store(r.Header.Get("Authorization"))
		_, _ = io.WriteString(w, `{"resources":{"core":{"limit":5000,"remaining":4812,"reset":1792351465}}}`)
	}))
	defer srv.Close()

	s := New(nil, "secret", slog.New(slog.NewTextHandler(io.Discard, nil)), WithBaseURLs(srv.URL, srv.URL, srv.URL))
	if !s.HasToken() {
		t.Error("HasToken = false")
	}
	for range 2 {
		rl, err := s.RateLimit(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if rl.Limit != 5000 || rl.Remaining != 4812 || rl.ResetAt.Unix() != 1792351465 {
			t.Errorf("rate limit = %+v", rl)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("GitHub asked %d times, want 1 (cached)", calls.Load())
	}
	if auth.Load() != "Bearer secret" {
		t.Errorf("Authorization = %q", auth.Load())
	}
}

func TestRateLimitBadToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	s := New(nil, "wrong", slog.New(slog.NewTextHandler(io.Discard, nil)), WithBaseURLs(srv.URL, srv.URL, srv.URL))
	if _, err := s.RateLimit(context.Background()); err == nil {
		t.Error("want an error for a rejected token")
	}
}

func TestRateLimitFromResponseHeaders(t *testing.T) {
	var quotaCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rate_limit" {
			quotaCalls.Add(1)
		}
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Remaining", "41")
		w.Header().Set("X-RateLimit-Reset", "1792351465")
		w.Header().Set("X-RateLimit-Resource", "core")
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()

	s := New(nil, "", slog.New(slog.NewTextHandler(io.Discard, nil)), WithBaseURLs(srv.URL, srv.URL, srv.URL))
	if _, _, _, err := s.get(context.Background(), srv.URL+"/repos/o/r/releases", "", true); err != nil {
		t.Fatal(err)
	}
	rl, err := s.RateLimit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rl.Limit != 60 || rl.Remaining != 41 || rl.ResetAt.Unix() != 1792351465 {
		t.Errorf("rate limit = %+v", rl)
	}
	if quotaCalls.Load() != 0 {
		t.Errorf("/rate_limit called %d times, want 0", quotaCalls.Load())
	}

	// A non-GitHub request or another resource must not touch the cache.
	s.noteQuota(http.Header{"X-Ratelimit-Limit": {"1"}, "X-Ratelimit-Remaining": {"1"}, "X-Ratelimit-Reset": {"1"}, "X-Ratelimit-Resource": {"search"}})
	s.noteQuota(http.Header{"X-Ratelimit-Limit": {"x"}, "X-Ratelimit-Remaining": {"1"}, "X-Ratelimit-Reset": {"1"}})
	if rl2, _ := s.RateLimit(context.Background()); rl2 != rl {
		t.Errorf("cache changed to %+v", rl2)
	}
}

func TestSetToken(t *testing.T) {
	var auth atomic.Value
	var limited atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
		if r.URL.Path == "/rate_limit" {
			_, _ = io.WriteString(w, `{"resources":{"core":{"limit":60,"remaining":0,"reset":1792351465}}}`)
			return
		}
		if limited.Load() {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "4102444800") // far away
			w.WriteHeader(http.StatusForbidden)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	s := New(nil, "", slog.New(slog.NewTextHandler(io.Discard, nil)), WithBaseURLs(srv.URL, srv.URL, srv.URL))
	ctx := context.Background()
	if s.HasToken() {
		t.Error("HasToken without a token")
	}
	limited.Store(true)
	if _, _, _, err := s.get(ctx, srv.URL+"/repos/a/b/releases", "", true); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v", err)
	}
	if _, err := s.RateLimit(ctx); err != nil {
		t.Fatal(err)
	}

	s.SetToken("ghp_new")
	if !s.HasToken() {
		t.Error("HasToken = false after SetToken")
	}
	// The rate limit belonged to the old token: requests go out again, with the new one.
	limited.Store(false)
	if _, _, status, err := s.get(ctx, srv.URL+"/repos/a/b/releases", "", true); err != nil || status != http.StatusNotFound {
		t.Fatalf("after SetToken: %d, %v", status, err)
	}
	if auth.Load() != "Bearer ghp_new" {
		t.Errorf("Authorization = %q", auth.Load())
	}
	s.mu.Lock()
	cached := !s.rateLimitAt.IsZero()
	s.mu.Unlock()
	if cached {
		t.Error("the old quota is still cached")
	}

	s.SetToken("")
	if _, _, _, _ = s.get(ctx, srv.URL+"/repos/a/b/releases", "", true); auth.Load() != "" {
		t.Errorf("Authorization after clearing = %q", auth.Load())
	}
}

// LastQuota only reports what responses already said, and WithTransport carries every
// request, so main can count them.
func TestLastQuotaAndTransport(t *testing.T) {
	reset := time.Now().Add(time.Hour).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Remaining", "12")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
		_, _ = io.WriteString(w, `[]`)
	}))
	defer srv.Close()
	var sent atomic.Int32
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sent.Add(1)
		return http.DefaultTransport.RoundTrip(r)
	})
	s := New(nil, "", slog.New(slog.NewTextHandler(io.Discard, nil)), WithBaseURLs(srv.URL, srv.URL, srv.URL), WithTransport(rt))

	if _, ok := s.LastQuota(); ok {
		t.Error("a quota is known before any request")
	}
	if _, _, _, err := s.get(context.Background(), srv.URL+"/repos/a/b/releases", "", true); err != nil {
		t.Fatal(err)
	}
	if rl, ok := s.LastQuota(); !ok || rl.Limit != 60 || rl.Remaining != 12 || rl.ResetAt.Unix() != reset {
		t.Errorf("LastQuota = %+v %v", rl, ok)
	}
	if sent.Load() != 1 {
		t.Errorf("requests through the transport = %d, want 1 (LastQuota must not ask)", sent.Load())
	}
	// Once the reset time has passed, the remaining count is stale.
	s.mu.Lock()
	s.rateLimit.ResetAt = time.Now().Add(-time.Minute)
	s.mu.Unlock()
	if _, ok := s.LastQuota(); ok {
		t.Error("a quota past its reset is still reported")
	}
	s.SetToken("ghp_x")
	if _, ok := s.LastQuota(); ok {
		t.Error("the old token's quota is still known")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

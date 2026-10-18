package changelog

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
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

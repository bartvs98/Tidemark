package changelog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// rateLimitFor: how long a quota is reused, so an open settings page doesn't ask GitHub
// on every request. The cache is also refreshed by every GitHub API response Tidemark
// makes (X-RateLimit-* headers), so it stays current during a check run.
const rateLimitFor = time.Minute

// RateLimit is GitHub's core API quota for Tidemark's IP (without a token) or token.
type RateLimit struct {
	Limit     int
	Remaining int
	ResetAt   time.Time
}

// HasToken reports whether GitHub requests are sent with a token.
func (s *Service) HasToken() bool { return s.token != "" }

// RateLimit asks GitHub for the current quota. GitHub's /rate_limit endpoint doesn't count
// against the quota itself. Answers are cached for rateLimitFor, and replaced by the
// headers of any later GitHub API response.
func (s *Service) RateLimit(ctx context.Context) (RateLimit, error) {
	s.mu.Lock()
	if !s.rateLimitAt.IsZero() && time.Since(s.rateLimitAt) < rateLimitFor {
		rl := s.rateLimit
		s.mu.Unlock()
		return rl, nil
	}
	s.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.githubAPI+"/rate_limit", nil)
	if err != nil {
		return RateLimit{}, err
	}
	req.Header.Set("User-Agent", "tidemark")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return RateLimit{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return RateLimit{}, fmt.Errorf("GitHub rejected the token (401)")
	}
	if resp.StatusCode != http.StatusOK {
		return RateLimit{}, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var body struct {
		Resources struct {
			Core struct {
				Limit     int   `json:"limit"`
				Remaining int   `json:"remaining"`
				Reset     int64 `json:"reset"`
			} `json:"core"`
		} `json:"resources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return RateLimit{}, fmt.Errorf("GitHub rate limit: %w", err)
	}
	core := body.Resources.Core
	rl := RateLimit{Limit: core.Limit, Remaining: core.Remaining, ResetAt: time.Unix(core.Reset, 0)}

	s.mu.Lock()
	s.rateLimit, s.rateLimitAt = rl, time.Now()
	s.mu.Unlock()
	return rl, nil
}

// noteQuota caches the quota GitHub reports in a response's headers (also on 304 and
// rate-limited answers). Missing or malformed headers, or another resource, are ignored.
func (s *Service) noteQuota(h http.Header) {
	if res := h.Get("X-RateLimit-Resource"); res != "" && res != "core" {
		return
	}
	limit, err1 := strconv.Atoi(h.Get("X-RateLimit-Limit"))
	remaining, err2 := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	reset, err3 := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return
	}
	s.mu.Lock()
	s.rateLimit, s.rateLimitAt = RateLimit{Limit: limit, Remaining: remaining, ResetAt: time.Unix(reset, 0)}, time.Now()
	s.mu.Unlock()
}

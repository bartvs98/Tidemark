// Package stats counts outgoing HTTP requests per group (registry, GitHub, GitLab),
// process-wide, for the stats page. Each check run stores the counts made since the
// previous run.
package stats

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// Count is a number of requests and how many of them were refused for a rate limit.
type Count struct {
	Requests int64 `json:"requests"`
	Limited  int64 `json:"limited"` // 429, or 403 with X-RateLimit-Remaining: 0
}

// Requests counts requests per group. The zero value isn't usable; see NewRequests.
type Requests struct {
	mu     sync.Mutex
	groups map[string]*counter
	base   map[string]Count // totals at the previous Since
}

type counter struct{ requests, limited atomic.Int64 }

func NewRequests() *Requests {
	return &Requests{groups: map[string]*counter{}, base: map[string]Count{}}
}

// Wrap counts every request through rt; group(req) names it ("registry" | "github" |
// "gitlab"; "" = not counted). A request that fails without an answer still counts.
func (r *Requests) Wrap(rt http.RoundTripper, group func(*http.Request) string) http.RoundTripper {
	if rt == nil {
		rt = http.DefaultTransport
	}
	return roundTripper{next: rt, reqs: r, group: group}
}

// Since returns the counts per group since the last commit (since the start before the
// first). Calling commit makes those counts the new baseline, so counts that couldn't be
// stored carry over to the next call; requests made in between stay counted either way.
// Groups without requests yet are left out.
func (r *Requests) Since() (counts map[string]Count, commit func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]Count, len(r.groups))
	seen := make(map[string]Count, len(r.groups))
	for g, c := range r.groups {
		now := Count{Requests: c.requests.Load(), Limited: c.limited.Load()}
		prev := r.base[g]
		out[g] = Count{Requests: now.Requests - prev.Requests, Limited: now.Limited - prev.Limited}
		seen[g] = now
	}
	return out, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		for g, now := range seen {
			r.base[g] = now
		}
	}
}

func (r *Requests) counter(group string) *counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.groups[group]
	if !ok {
		c = &counter{}
		r.groups[group] = c
	}
	return c
}

type roundTripper struct {
	next  http.RoundTripper
	reqs  *Requests
	group func(*http.Request) string
}

func (t roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	g := t.group(req)
	if g == "" {
		return t.next.RoundTrip(req)
	}
	c := t.reqs.counter(g)
	c.requests.Add(1)
	resp, err := t.next.RoundTrip(req)
	if err == nil && limited(resp) {
		c.limited.Add(1)
	}
	return resp, err
}

// limited: the answer refuses the request for a rate limit. GitHub answers its primary
// limit with 403 and no quota left; registries and GitHub's secondary limit with 429.
func limited(resp *http.Response) bool {
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusForbidden:
		return resp.Header.Get("X-RateLimit-Remaining") == "0"
	}
	return false
}

// Forge groups a release-note request by host: "github" for github.com and
// githubusercontent.com (and their subdomains), "gitlab" for gitlab.com, its subdomains
// and hosts named gitlab.*, else "" (not counted).
func Forge(req *http.Request) string {
	host := strings.ToLower(req.URL.Hostname())
	switch {
	case isOrUnder(host, "github.com"), isOrUnder(host, "githubusercontent.com"):
		return "github"
	case isOrUnder(host, "gitlab.com"), strings.HasPrefix(host, "gitlab."):
		return "gitlab"
	}
	return ""
}

func isOrUnder(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

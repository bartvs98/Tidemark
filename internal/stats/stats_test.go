package stats

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// fakeRT answers every request with the status and headers set for its path.
type fakeRT map[string]*http.Response

func (f fakeRT) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, ok := f[req.URL.Path]
	if !ok {
		return nil, errors.New("connection refused")
	}
	return resp, nil
}

func answer(status int, kv ...string) *http.Response {
	h := http.Header{}
	for i := 0; i+1 < len(kv); i += 2 {
		h.Set(kv[i], kv[i+1])
	}
	return &http.Response{StatusCode: status, Header: h, Body: http.NoBody}
}

func TestRequestsCountsAndResets(t *testing.T) {
	reqs := NewRequests()
	rt := reqs.Wrap(fakeRT{
		"/ok":        answer(200),
		"/429":       answer(429),
		"/403-empty": answer(403, "X-RateLimit-Remaining", "0"),
		"/403-left":  answer(403, "X-RateLimit-Remaining", "12"),
		"/304":       answer(304),
	}, Forge)
	get := func(url string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		_, _ = rt.RoundTrip(req)
	}
	get("https://api.github.com/ok")
	get("https://api.github.com/304")
	get("https://api.github.com/403-empty")
	get("https://api.github.com/403-left") // forbidden, but not for the quota
	get("https://raw.githubusercontent.com/429")
	get("https://gitlab.com/ok")
	get("https://gitlab.example.org/down") // no answer: counted, not limited
	get("https://example.org/ok")          // not a forge: not counted

	want := map[string]Count{"github": {Requests: 5, Limited: 2}, "gitlab": {Requests: 2}}
	got, commit := reqs.Since()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("first Since = %v, want %v", got, want)
	}
	// Not committed (the row wasn't stored): the same counts come back.
	if again, _ := reqs.Since(); !reflect.DeepEqual(again, want) {
		t.Errorf("Since without commit = %v, want %v", again, want)
	}
	// A request between Since and commit stays counted for the next row.
	get("https://api.github.com/429")
	commit()
	if got := since(reqs); !reflect.DeepEqual(got, map[string]Count{"github": {Requests: 1, Limited: 1}, "gitlab": {}}) {
		t.Errorf("after commit Since = %v", got)
	}
	// Committed: nothing new, so zeros for the groups seen so far.
	if got, want := since(reqs), (map[string]Count{"github": {}, "gitlab": {}}); !reflect.DeepEqual(got, want) {
		t.Errorf("Since after commit = %v, want %v", got, want)
	}
}

// since takes the counts and commits them.
func since(r *Requests) map[string]Count {
	got, commit := r.Since()
	commit()
	return got
}

func TestWrapFixedGroup(t *testing.T) {
	reqs := NewRequests()
	rt := reqs.Wrap(fakeRT{"/v2/": answer(200)}, func(*http.Request) string { return "registry" })
	for range 3 {
		_, _ = rt.RoundTrip(httptest.NewRequest(http.MethodGet, "https://index.docker.io/v2/", nil))
	}
	if got := since(reqs); got["registry"] != (Count{Requests: 3}) || len(got) != 1 {
		t.Errorf("Since = %v", got)
	}
}

func TestForge(t *testing.T) {
	tests := map[string]string{
		"https://api.github.com/repos/a/b/releases":   "github",
		"https://github.com/a/b":                      "github",
		"https://raw.githubusercontent.com/a/b/HEAD/": "github",
		"https://gitlab.com/api/v4/projects/x":        "gitlab",
		"https://gitlab.example.org/api/v4":           "gitlab",
		"https://notgithub.com/x":                     "",
		"https://cdn.jsdelivr.net/x":                  "",
	}
	for url, want := range tests {
		if got := Forge(httptest.NewRequest(http.MethodGet, url, nil)); got != want {
			t.Errorf("Forge(%s) = %q, want %q", url, got, want)
		}
	}
}

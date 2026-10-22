// Package ai summarises the release notes between a running and a latest version with a
// language model: what changed, what the user needs to do, and a verdict that can raise
// (never lower) the heuristic level. It's opt-in; nothing is sent unless a provider is
// configured.
//
// Release notes are untrusted input: the prompt says so, wraps them in delimiters the
// notes can't close, and the model's answer is constrained to a JSON schema, then
// validated and capped here before anyone sees it.
package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/version"
)

var (
	ErrOff     = errors.New("AI summaries are off")
	ErrNoNotes = errors.New("no release notes to summarise")
)

// Key identifies one summary: a repo's notes between two versions.
type Key struct{ Repo, From, To string }

// KeyFor builds the key from the repo key (source.Repo.Key()) and the update's versions;
// ok is false when latest is nil (nothing to summarise towards). From is "?" when running is nil.
func KeyFor(repo string, running, latest *version.Version) (Key, bool) {
	if latest == nil {
		return Key{}, false
	}
	k := Key{Repo: repo, From: "?", To: VersionKey(*latest)}
	if running != nil {
		k.From = VersionKey(*running)
	}
	return k, true
}

// VersionKey is a canonical version string for keys: numbers, pre-release and build
// (linuxserver -ls builds matter); no prefix, no variant. "1.2.3", "2.0.0-rc.1", "4.1.0+b175".
func VersionKey(v version.Version) string {
	s := v.String()
	if v.Build > 0 {
		s += "+b" + strconv.Itoa(v.Build)
	}
	return s
}

type Reason struct {
	Text string `json:"text"`
	Tag  string `json:"tag,omitempty"` // a release tag that was sent, else ""
}

type Summary struct {
	Changes   []string // ≤ 8 items, each ≤ 300 chars
	Actions   []string // ≤ 6 items, may be empty
	Verdict   analysis.Level
	Reasons   []Reason // ≤ 5; empty when Verdict is None
	Provider  string   // anthropic | openai | gemini
	Model     string
	Releases  int // releases sent
	Omitted   int // releases in range left out (input cap)
	NotesHash string
	CreatedAt time.Time
}

// Entry is the stored state of one key.
type Entry struct {
	Summary  *Summary // nil: never succeeded
	Error    string   // last attempt failed (scrubbed, short); "" if it worked
	FailedAt time.Time
}

// Notes is one release as sent to the model.
type Notes struct {
	Tag         string
	Name        string
	PublishedAt time.Time
	Body        string
	Level       analysis.Level // heuristic level, used to prioritise under the input cap
}

type Request struct {
	Image           string  // image reference as the user sees it (e.g. "ghcr.io/home-assistant/home-assistant")
	Repo            string  // owner/name
	Running, Latest string  // as displayed; Running may be ""
	Releases        []Notes // in range, newest first
}

// RequestFrom builds a Request from the stored notes in range (changelog.Changes.Releases)
// and computes each release's heuristic level with analysis.Assess (RunningNotes = ch.FromBody).
func RequestFrom(image, repo, running, latest string, ch changelog.Changes) Request {
	r := Request{Image: image, Repo: repo, Running: running, Latest: latest}
	in := analysis.Input{RunningNotes: ch.FromBody}
	for _, rel := range ch.Releases {
		in.Releases = append(in.Releases, analysis.Notes{Tag: rel.Tag, Body: rel.Body})
	}
	res := analysis.Assess(in) // no versions: only the notes' own flags per release
	for i, rel := range ch.Releases {
		n := Notes{Tag: rel.Tag, Name: rel.Name, PublishedAt: rel.PublishedAt, Body: rel.Body}
		if i < len(res.Releases) {
			n.Level = res.Releases[i].Level
		}
		r.Releases = append(r.Releases, n)
	}
	return r
}

// NotesHash hashes running, latest and every release's tag + body (before the cap), so a
// stored summary can be compared with the notes now.
func NotesHash(r Request) string {
	h := sha256.New()
	field := func(s string) { // length-prefixed, so fields can't run into each other
		h.Write([]byte(strconv.Itoa(len(s)) + ":" + s))
	}
	field(r.Running)
	field(r.Latest)
	for _, n := range r.Releases {
		field(n.Tag)
		field(n.Body)
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

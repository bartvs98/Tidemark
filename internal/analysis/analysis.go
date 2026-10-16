// Package analysis flags release notes for breaking changes and things that need
// attention, with plain heuristics: a major (or 0.x minor) version jump, and phrases in
// the notes. Each flagged line is reported, so the UI can highlight it.
//
// The patterns are deliberately phrase-level: release notes say "breaking" and "migrate"
// all the time without meaning the user ("fix a bug breaking the import", "Migrate mqtt
// to the new API"). Lines that were already in the running release's notes are ignored,
// because some projects repeat their whole history in every release.
package analysis

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/bartvs98/tidemark/internal/version"
)

// Level is how risky an update looks. Higher is worse.
type Level int

const (
	None Level = iota
	Attention
	Breaking
)

func (l Level) String() string {
	switch l {
	case Attention:
		return "attention"
	case Breaking:
		return "breaking"
	}
	return "none"
}

func (l Level) MarshalText() ([]byte, error) { return []byte(l.String()), nil }

func (l *Level) UnmarshalText(b []byte) error {
	switch string(b) {
	case "none":
		*l = None
	case "attention":
		*l = Attention
	case "breaking":
		*l = Breaking
	default:
		return fmt.Errorf("unknown level %q", b)
	}
	return nil
}

// Hit is one flagged line of a release's notes.
type Hit struct {
	Line  string `json:"line"` // the source line, trimmed, as it appears in the markdown
	Level Level  `json:"level"`
	// Label: a section heading ("## Breaking changes") whose lines inherit its level.
	Label bool `json:"-"`
}

// Release is the verdict for one release's notes.
type Release struct {
	Level Level
	Hits  []Hit
}

// Input is an update to assess.
type Input struct {
	Class           version.Class
	Running, Latest *version.Version // nil when unknown
	// Notes of the releases after running up to latest, newest first, with their tags.
	Releases []Notes
	// RunningNotes are the running release's own notes; lines from it aren't news.
	RunningNotes string
}

type Notes struct {
	Tag  string
	Body string
}

// Result is the overall verdict plus one entry per input release (same order).
type Result struct {
	Level    Level
	Reasons  []string // short, specific, most severe first
	Releases []Release
}

// maxQuoted is how many flagged lines Reasons quotes.
const maxQuoted = 3

// Assess combines the version jump with the flags in each release's notes.
func Assess(in Input) Result {
	var out Result
	if r := bumpReason(in); r != "" {
		out.Level = Breaking
		out.Reasons = append(out.Reasons, r)
	}

	known := lineSet(in.RunningNotes)
	type quote struct {
		tag, text string
		level     Level
	}
	var quotes []quote
	seen := map[string]bool{}
	for _, n := range in.Releases {
		rel := Analyze(n.Body, known)
		out.Releases = append(out.Releases, rel)
		out.Level = max(out.Level, rel.Level)
		// Quote what's under a "Breaking changes" heading rather than the heading itself.
		hasContent := slices.ContainsFunc(rel.Hits, func(h Hit) bool { return !h.Label })
		for _, h := range rel.Hits {
			if h.Label && hasContent {
				continue
			}
			text := plain(h.Line)
			if text == "" || seen[sameText(text)] {
				continue
			}
			seen[sameText(text)] = true
			quotes = append(quotes, quote{n.Tag, text, h.Level})
		}
	}
	// Breaking lines first, then attention, each in release order (newest first).
	quoted := 0
	for _, lvl := range []Level{Breaking, Attention} {
		for _, q := range quotes {
			if q.level == lvl && quoted < maxQuoted {
				out.Reasons = append(out.Reasons, q.tag+": "+truncate(q.text, 140))
				quoted++
			}
		}
	}
	if extra := len(quotes) - quoted; extra > 0 {
		out.Reasons = append(out.Reasons, fmt.Sprintf("and %d more flagged %s", extra, pluralLines(extra)))
	}
	return out
}

func bumpReason(in Input) string {
	if in.Running == nil || in.Latest == nil {
		return ""
	}
	switch {
	case in.Class == version.Major:
		return fmt.Sprintf("Major version: %s → %s", in.Running, in.Latest)
	case in.Class == version.Minor && at(in.Running.Nums, 0) == 0 && at(in.Latest.Nums, 0) == 0:
		return fmt.Sprintf("0.x minor version: %s → %s (before 1.0, minor releases may break things)", in.Running, in.Latest)
	}
	return ""
}

// ── per release ──────────────────────────────────────────

var (
	reFence    = regexp.MustCompile("^\\s*(```|~~~)")
	reHeading  = regexp.MustCompile(`^\s{0,3}(#{1,6})\s+\S`)
	reBoldOnly = regexp.MustCompile(`^\W*(\*\*|__)[^*_]+(\*\*|__)\W*$`) // "⚠️ **Breaking Changes**:" acts like a heading
	// Ordinary changelog sections; projects nest them inconsistently under a breaking one.
	reGenericSection = regexp.MustCompile(`(?i)^\W*(new |other |bug ?)?(features?|enhancements?|improvements?|fix(es)?|bug ?fixes|other|misc\w*|changes|what's changed|dependenc\w*|docs|documentation|chores?|maintenance|performance|refactor\w*|security|tests?|(new )?contributors|full changelog)\b`)
	reEmpty          = regexp.MustCompile(`(?i)^\W*(none|n/?a|nothing|-|no breaking changes?)\W*$`)
	reMarker         = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+(?:\[[ xX]\]\s+)?`)
	reEmphasis       = regexp.MustCompile(`\*\*|__|` + "`")
	reLink           = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	reSpaces         = regexp.MustCompile(`\s+`)
)

// Analyze flags the lines of one release's notes. Lines in known are skipped.
func Analyze(body string, known map[string]bool) Release {
	type section struct {
		depth int // 1–6 for headings, 7 for a bold-only label line
		level Level
		hit   int // index of the label's hit in hits, or -1
		items int // content lines under it
	}
	var (
		rel     Release
		inFence bool
		open    []section // flagged labels whose content inherits their level
	)
	closeTo := func(depth int) {
		for len(open) > 0 && open[len(open)-1].depth >= depth {
			s := open[len(open)-1]
			open = open[:len(open)-1]
			if s.items == 0 && s.hit >= 0 {
				rel.Hits[s.hit].Level = None // an empty "Breaking changes" section says nothing
			}
		}
	}

	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if reFence.MatchString(raw) {
			inFence = !inFence
			continue
		}
		line := strings.TrimSpace(raw)
		if inFence || line == "" || strings.HasPrefix(line, "<!--") {
			continue
		}

		depth := 0
		if m := reHeading.FindStringSubmatch(raw); m != nil {
			depth = len(m[1])
		} else if reBoldOnly.MatchString(line) {
			depth = 7
		}
		if depth > 0 && reGenericSection.MatchString(plain(line)) {
			closeTo(0) // "### Features" ends a flagged section even when nested under it
		} else if depth > 0 {
			closeTo(depth)
		} else if len(open) > 0 && reEmpty.MatchString(line) {
			continue
		}

		isKnown := known[sameText(line)]
		lvl := lineLevel(line)
		if isKnown {
			lvl = None
		}
		inherited := None
		for i := range open {
			inherited = max(inherited, open[i].level)
			if depth == 0 {
				open[i].items++
			}
		}
		if depth == 0 && !isKnown {
			lvl = max(lvl, inherited) // an attention line under "Breaking changes" is breaking
		}

		idx := -1
		if lvl > None {
			idx = len(rel.Hits)
			rel.Hits = append(rel.Hits, Hit{Line: line, Level: lvl, Label: depth > 0})
		}
		if depth > 0 && lvl > None {
			open = append(open, section{depth: depth, level: lvl, hit: idx})
		}
	}
	closeTo(0)

	hits := rel.Hits[:0]
	for _, h := range rel.Hits {
		if h.Level > None {
			hits = append(hits, h)
			rel.Level = max(rel.Level, h.Level)
		}
	}
	rel.Hits = hits
	return rel
}

// ── line patterns ────────────────────────────────────────

var (
	breakingPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\bBREAKING\b`), // case-sensitive: "BREAKING CHANGES", "BREAKING:"
		regexp.MustCompile(`(?i)\bbreaking[- ]changes?\b`),
		regexp.MustCompile(`(?i)\b(potentially|possibly) breaking\b`),
		regexp.MustCompile(`(?i)^\W*breaking\b(\W|$)`), // "## Breaking", "Breaking: …", "- **Breaking** …"
		regexp.MustCompile(`(?i)\bbackwards?[- ]incompatib`),
		regexp.MustCompile(`(?i)\bnot backwards?[- ]compatible\b`),
		regexp.MustCompile(`(?i)\bincompatible changes?\b`),
		regexp.MustCompile(`💥`),
	}
	// "no breaking changes", "non-breaking", "fix a breaking change in …",
	// "Add a Breaking change note to the changelog"
	breakingNegated = regexp.MustCompile(`(?i)\b(no|non|without|not a|zero|fix|fixes|fixed|fixing|avoid|avoids|prevent|prevents)\b[\s-]+(\w+\s+){0,2}breaking` +
		`|\b(add|adds|added|update|updates|updated|document|documents|documented)\b.{0,20}breaking[- ]changes? (note|notes|section|docs?|documentation|label)\b`)
	// Conventional commits: "feat!: …", "fix(api)!: …", after the list marker.
	conventionalBang = regexp.MustCompile(`^\w+(\([^)]*\))?!:`)

	attentionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`⚠`),
		regexp.MustCompile(`(?i)\baction (is )?required\b`),
		regexp.MustCompile(`(?i)\bmanual (step|intervention|action|migration|upgrade)s?\b`),
		regexp.MustCompile(`(?i)\b(read|see|check|follow)\b.{0,40}\b(migration|upgrade|upgrading) (guide|notes|instructions)\b`),
		regexp.MustCompile(`(?i)\bupgrade notes?\b`),
		regexp.MustCompile(`(?i)\b(must|need to|needs to|have to|has to|should) (migrate|update (your|their)|change (your|their)|re-?create|re-?configure|re-?index)\b`),
		regexp.MustCompile(`(?i)\bexisting (users|installs|installations|setups|instances|deployments)\b.{0,40}\b(should|must|need|needs|have to)\b`),
		regexp.MustCompile(`(?i)\bmigrations?\b.{0,30}\b(may|will|can|might) take\b`),
		// Passive "removed" only where it reads as an announcement: "settings have been removed",
		// "X is removed and ignored", not "while the entity is removed (@user - #123)".
		regexp.MustCompile(`(?i)\b(has|have) been (removed|deprecated|dropped)\b`),
		regexp.MustCompile(`(?i)\b(is|are|was|were) (now )?deprecated\b`),
		regexp.MustCompile(`(?i)\b(is|are) (now )?removed(\s*$|[.,;:]|\s+(and|in favou?r)\b)`),
		regexp.MustCompile(`(?i)\bwill be (removed|deprecated|dropped)\b`),
		regexp.MustCompile(`(?i)\bno longer (supported|available|works?|shared|visible|accepted|honou?red)\b`),
		regexp.MustCompile(`(?i)\b(drops?|dropped|removes?|removed|ends?|ended) support\b`),
		regexp.MustCompile(`(?i)\bend[- ]of[- ]life\b`),
		regexp.MustCompile(`(?i)^\W*(removed|deprecated|dropped)\b`), // "- **removed** basic auth", "Removed MySQL libraries"
		regexp.MustCompile(`(?i)^\W*(important|warning|caution|heads[- ]up)\b`),
		regexp.MustCompile(`(?i)\b(env(ironment)?( var(iable)?s?)?|variables?|settings?|options?|config(uration)?( keys?| options?)?|parameters?|flags?|endpoints?)\b.{0,60}\b(renamed|has changed name)\b`),
		regexp.MustCompile(`(?i)\brenamed\b.{0,60}\b(env(ironment)?|variables?|settings?|options?|config(uration)?|keys?|parameters?|flags?|endpoints?)\b`),
	}
)

func lineLevel(line string) Level {
	for _, p := range breakingPatterns {
		if p.MatchString(line) && !breakingNegated.MatchString(line) {
			return Breaking
		}
	}
	if conventionalBang.MatchString(reEmphasis.ReplaceAllString(reMarker.ReplaceAllString(line, ""), "")) {
		return Breaking
	}
	for _, p := range attentionPatterns {
		if p.MatchString(line) {
			return Attention
		}
	}
	return None
}

// ── helpers ──────────────────────────────────────────────

func lineSet(body string) map[string]bool {
	set := map[string]bool{}
	for _, l := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			set[sameText(l)] = true
		}
	}
	return set
}

var reDigits = regexp.MustCompile(`\d+`)

// sameText is a comparison key that ignores numbers, so boilerplate repeated in every release
// with its own version or link in it ("read the [migration guide](…#v3715)") counts as the same line.
func sameText(line string) string { return reDigits.ReplaceAllString(plain(line), "#") }

// plain turns a markdown line into readable text for a reason.
func plain(line string) string {
	s := reMarker.ReplaceAllString(line, "")
	s = strings.TrimLeft(s, "# >")
	s = reLink.ReplaceAllString(s, "$1")
	s = reEmphasis.ReplaceAllString(s, "")
	return strings.TrimSpace(reSpaces.ReplaceAllString(s, " "))
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

func at(nums []int, i int) int {
	if i < len(nums) {
		return nums[i]
	}
	return 0
}

func pluralLines(n int) string {
	if n == 1 {
		return "line"
	}
	return "lines"
}

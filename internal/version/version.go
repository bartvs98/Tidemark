// Package version parses Docker image tags as versions and compares them.
//
// Tags are much looser than semver, so parsing is lenient and splits a tag into:
//
//	v1.15.0-ls175      prefix "v", numbers [1 15 0], build 175
//	16-alpine          numbers [16], variant "alpine"
//	7.4.2-alpine3.21   numbers [7 4 2], variant "alpine" (variant version digits ignored)
//	2.0.0-rc.1         numbers [2 0 0], pre-release "rc.1"
//	2026.9.0           numbers [2026 9 0] (calendar versions work the same way)
//	16-3.4             numbers [16], suffix [3 4] (postgis: PostgreSQL 16, PostGIS 3.4)
//	4.0.2-20           numbers [4 0 2], suffix [20] (node-red on Node 20, Debian-style revisions)
//	2025-01-01         numbers [2025 1 1], a date
//
// Tags are only compared within a family (same variant, both stable, both dates or
// both not), so "16-alpine" is never "newer" than "17-bookworm" and "2025-01-01" never
// "newer" than "1.2.3". A numeric suffix stays in the family: it sorts after the
// numbers, and resolve prefers the running tag's suffix line.
package version

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

type Version struct {
	Raw     string // the tag as written
	Prefix  string // "v" or ""
	Nums    []int  // numeric components, at least one
	Pre     string // pre-release marker ("rc.1", "beta"), empty for stable releases
	Variant string // flavour such as "alpine" or "bookworm-slim", empty for the plain image
	Build   int    // packaging revision: linuxserver "-ls175", "-r3"; 0 if none
	Suffix  []int  // purely numeric part after the numbers: "16-3.4" → [3 4], "4.0.2-20" → [20]
	Date    bool   // the tag is a YYYY-MM-DD date; Nums holds year, month, day

	// RawVariant keeps the variant exactly as written ("alpine3.21"), to find tags that
	// are written the same way as another tag.
	RawVariant string
}

var (
	coreRe  = regexp.MustCompile(`^(\d+)((?:\.\d+)*)`)
	dateRe  = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})(?:$|[-_+])`)
	numRe   = regexp.MustCompile(`^\d+(?:\.\d+)*$`)
	buildRe = regexp.MustCompile(`^(?:ls|r)(\d+)$`)
	preRe   = regexp.MustCompile(`^(?:alpha|beta|rc|pre|preview|dev|develop|nightly|snapshot|canary|next|test|testing|unstable|edge|insiders|b|a)[.\d]*$`)
)

// Parse reads a tag as a version. ok is false for tags like "latest", "alpine",
// "nightly" or commit hashes ("e8a527a2-ls87").
func Parse(tag string) (v Version, ok bool) {
	v.Raw = tag
	s := strings.ToLower(strings.TrimSpace(tag))
	if len(s) > 1 && s[0] == 'v' && s[1] >= '0' && s[1] <= '9' {
		v.Prefix = "v"
		s = s[1:]
	}

	var rest string
	if d := dateRe.FindStringSubmatch(s); d != nil && v.Prefix == "" && validDate(d[2], d[3]) {
		// "2025-01-01" is a date, not version 2025 with a suffix.
		v.Nums = splitNums(d[1] + "." + d[2] + "." + d[3])
		v.Date = true
		rest = s[len("YYYY-MM-DD"):]
	} else {
		m := coreRe.FindStringSubmatch(s)
		if m == nil {
			return Version{}, false
		}
		rest = s[len(m[0]):]
		// The number must end the tag or be followed by a separator; "1234abc" is a hash, not a version.
		if rest != "" && !strings.ContainsRune("-_+", rune(rest[0])) {
			return Version{}, false
		}
		if v.Nums = splitNums(m[1] + m[2]); v.Nums == nil {
			return Version{}, false // overflow
		}
		// Bare 7+ digit numbers are almost always build IDs or dates of CI builds, not versions.
		if len(v.Nums) == 1 && len(m[1]) >= 7 {
			return Version{}, false
		}
	}

	var variant, raw []string
	afterPre := false
	for _, tok := range strings.FieldsFunc(rest, func(r rune) bool { return r == '-' || r == '_' || r == '+' }) {
		wasPre := afterPre
		afterPre = false
		switch {
		case buildRe.MatchString(tok):
			v.Build, _ = strconv.Atoi(buildRe.FindStringSubmatch(tok)[1])
		case preRe.MatchString(tok):
			v.Pre = joinNonEmpty(v.Pre, tok, ".")
			afterPre = true
		case numRe.MatchString(tok) && wasPre:
			v.Pre += "." + tok // "rc-2" is "rc.2", not a suffix that would rank above the release
		case numRe.MatchString(tok):
			n := splitNums(tok)
			if n == nil {
				return Version{}, false // overflow
			}
			v.Suffix = append(v.Suffix, n...)
		default:
			// alpine3.21 → alpine, cuda12 → cuda.
			if t := strings.TrimRight(tok, "0123456789."); t != "" {
				variant = append(variant, t)
				raw = append(raw, tok)
			}
		}
	}
	v.Variant = strings.Join(variant, "-")
	v.RawVariant = strings.Join(raw, "-")
	return v, true
}

// splitNums reads "3.4" as [3 4]; nil on overflow.
func splitNums(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}

// validDate checks month and day, so "2025-13-40" isn't read as a date.
func validDate(month, day string) bool {
	m, _ := strconv.Atoi(month)
	d, _ := strconv.Atoi(day)
	return m >= 1 && m <= 12 && d >= 1 && d <= 31
}

// HasPrefix reports whether v starts with all of p's numbers: 16.4.1 has prefix 16 and 16.4.
func (v Version) HasPrefix(p Version) bool {
	if len(p.Nums) > len(v.Nums) {
		return false
	}
	for i, n := range p.Nums {
		if v.Nums[i] != n {
			return false
		}
	}
	return true
}

// Stable reports whether this is a release (not alpha/beta/rc/…).
func (v Version) Stable() bool { return v.Pre == "" }

// SameVariant reports whether two versions are the same flavour of the image: same
// variant, and both dates or both not.
func (v Version) SameVariant(o Version) bool {
	return v.Variant == o.Variant && v.Date == o.Date
}

// SameFamily reports whether two versions are comparable: same variant and both stable,
// or both pre-releases.
func (v Version) SameFamily(o Version) bool {
	return v.SameVariant(o) && v.Stable() == o.Stable()
}

// String is the version without packaging details: "1.15.0", "16", "2.0.0-rc.1".
func (v Version) String() string {
	parts := make([]string, len(v.Nums))
	for i, n := range v.Nums {
		parts[i] = strconv.Itoa(n)
	}
	s := strings.Join(parts, ".")
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Compare orders versions: numbers first (a missing component sorts below 0, so the
// more precise "17.0" ranks above "17"), then the numeric suffix the same way, then
// stable above pre-release, then build. Variant and Date aren't compared: compare
// within a family.
func Compare(a, b Version) int {
	if c := compareNums(a.Nums, b.Nums); c != 0 {
		return c
	}
	if c := compareNums(a.Suffix, b.Suffix); c != 0 {
		return c
	}
	if a.Stable() != b.Stable() {
		if a.Stable() {
			return 1
		}
		return -1
	}
	if c := comparePre(a.Pre, b.Pre); c != 0 {
		return c
	}
	return cmpInt(a.Build, b.Build)
}

// compareNums compares component by component; a missing one sorts below 0.
func compareNums(a, b []int) int {
	for i := 0; i < max(len(a), len(b)); i++ {
		if x, y := at(a, i, -1), at(b, i, -1); x != y {
			return cmpInt(x, y)
		}
	}
	return 0
}

// preRank orders the usual pre-release stages; unlisted words (nightly, dev, …) rank lowest.
var preRank = map[string]int{"a": 1, "alpha": 1, "b": 2, "beta": 2, "pre": 3, "preview": 3, "rc": 4}

var preTokRe = regexp.MustCompile(`[a-z]+|\d+`)

// comparePre orders pre-release markers token by token: stage first (alpha < beta < rc),
// numbers numerically, so rc.2 < rc10 and rc1 == rc.1. More tokens rank higher (rc < rc1).
func comparePre(a, b string) int {
	x, y := preTokRe.FindAllString(a, -1), preTokRe.FindAllString(b, -1)
	for i := 0; i < min(len(x), len(y)); i++ {
		xn, xerr := strconv.Atoi(x[i])
		yn, yerr := strconv.Atoi(y[i])
		var c int
		switch {
		case xerr == nil && yerr == nil:
			c = cmpInt(xn, yn)
		case xerr == nil:
			c = -1 // numbers before words, as in semver
		case yerr == nil:
			c = 1
		default:
			if c = cmpInt(preRank[x[i]], preRank[y[i]]); c == 0 {
				c = strings.Compare(x[i], y[i])
			}
		}
		if c != 0 {
			return c
		}
	}
	return cmpInt(len(x), len(y))
}

// Class is how big a version jump is.
type Class string

const (
	None  Class = "none"
	Patch Class = "patch"
	Minor Class = "minor"
	Major Class = "major"
	// Digest means a new image without a new version: a rebuild of the same tag,
	// or a packaging bump such as linuxserver's -ls174 → -ls175.
	Digest Class = "digest"
)

// Classify describes the jump from running to latest. Missing components count as 0,
// so 16 → 16.4 is a minor update. A new year in a calendar version or date
// (2024.12.4 → 2025.1.0) is minor: the year says when, not how much changed. A higher
// numeric suffix alone (4.0.5-20 → 4.0.5-22, 16-3.4 → 16-3.5) is a patch.
func Classify(running, latest Version) Class {
	n := max(len(running.Nums), len(latest.Nums))
	for i := 0; i < n; i++ {
		x, y := at(running.Nums, i, 0), at(latest.Nums, i, 0)
		if x == y {
			continue
		}
		if y < x {
			return None // latest is older: nothing to do
		}
		switch i {
		case 0:
			if isYear(x) && isYear(y) {
				return Minor
			}
			return Major
		case 1:
			return Minor
		default:
			return Patch
		}
	}
	if running.Pre != latest.Pre {
		if Compare(latest, running) > 0 {
			return Patch // 2.0.0-rc.2 → 2.0.0, 2.0.0-rc.2 → 2.0.0-rc.3
		}
		return None
	}
	if c := compareNums(latest.Suffix, running.Suffix); c != 0 {
		if c > 0 {
			return Patch
		}
		return None
	}
	if latest.Build > running.Build {
		return Digest
	}
	return None
}

// Matches reports whether two tags name the same release, ignoring a "v" prefix:
// "v4.33.0" matches "4.33.0".
func Matches(a, b Version) bool {
	return Compare(a, b) == 0 && a.SameVariant(b) && len(a.Nums) == len(b.Nums) && slices.Equal(a.Suffix, b.Suffix)
}

// isYear: a first number that reads as a year makes a calendar version.
func isYear(n int) bool { return n >= 2000 && n <= 2099 }

func at(nums []int, i, missing int) int {
	if i < len(nums) {
		return nums[i]
	}
	return missing
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func joinNonEmpty(a, b, sep string) string {
	if a == "" {
		return b
	}
	return a + sep + b
}

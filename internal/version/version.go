// Package version parses Docker image tags as versions and compares them.
//
// Tags are much looser than semver, so parsing is lenient and splits a tag into:
//
//	v1.15.0-ls175      prefix "v", numbers [1 15 0], build 175
//	16-alpine          numbers [16], variant "alpine"
//	7.4.2-alpine3.21   numbers [7 4 2], variant "alpine" (variant version digits ignored)
//	2.0.0-rc.1         numbers [2 0 0], pre-release "rc.1"
//	2026.9.0           numbers [2026 9 0] (calendar versions work the same way)
//
// Tags are only compared within a family (same variant, both stable), so
// "16-alpine" is never "newer" than "17-bookworm".
package version

import (
	"regexp"
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

	// RawVariant keeps the variant exactly as written ("alpine3.21"), to find tags that
	// are written the same way as another tag.
	RawVariant string
}

var (
	coreRe  = regexp.MustCompile(`^(\d+)((?:\.\d+)*)`)
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

	m := coreRe.FindStringSubmatch(s)
	if m == nil {
		return Version{}, false
	}
	rest := s[len(m[0]):]
	// The number must end the tag or be followed by a separator; "1234abc" is a hash, not a version.
	if rest != "" && !strings.ContainsRune("-_+", rune(rest[0])) {
		return Version{}, false
	}
	for _, part := range strings.Split(m[1]+m[2], ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return Version{}, false // overflow
		}
		v.Nums = append(v.Nums, n)
	}
	// Bare 7+ digit numbers are almost always build IDs or dates of CI builds, not versions.
	if len(v.Nums) == 1 && len(m[1]) >= 7 {
		return Version{}, false
	}

	var variant, raw []string
	for _, tok := range strings.FieldsFunc(rest, func(r rune) bool { return r == '-' || r == '_' || r == '+' }) {
		switch {
		case buildRe.MatchString(tok):
			v.Build, _ = strconv.Atoi(buildRe.FindStringSubmatch(tok)[1])
		case preRe.MatchString(tok):
			v.Pre = joinNonEmpty(v.Pre, tok, ".")
		default:
			// alpine3.21 → alpine, cuda12 → cuda; a purely numeric token is dropped.
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

// SameFamily reports whether two versions are comparable: same variant and both stable,
// or both pre-releases.
func (v Version) SameFamily(o Version) bool {
	return v.Variant == o.Variant && v.Stable() == o.Stable()
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
// more precise "17.0" ranks above "17"), then stable above pre-release, then build.
func Compare(a, b Version) int {
	for i := 0; i < max(len(a.Nums), len(b.Nums)); i++ {
		x, y := at(a.Nums, i, -1), at(b.Nums, i, -1)
		if x != y {
			return cmpInt(x, y)
		}
	}
	if a.Stable() != b.Stable() {
		if a.Stable() {
			return 1
		}
		return -1
	}
	if a.Pre != b.Pre {
		return strings.Compare(a.Pre, b.Pre)
	}
	return cmpInt(a.Build, b.Build)
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
// so 16 → 16.4 is a minor update.
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
			return Major
		case 1:
			return Minor
		default:
			return Patch
		}
	}
	if running.Pre != latest.Pre && latest.Stable() {
		return Patch // 2.0.0-rc.2 → 2.0.0
	}
	if latest.Build > running.Build {
		return Digest
	}
	return None
}

// Matches reports whether two tags name the same release, ignoring a "v" prefix:
// "v4.33.0" matches "4.33.0".
func Matches(a, b Version) bool {
	return Compare(a, b) == 0 && a.Variant == b.Variant && len(a.Nums) == len(b.Nums)
}

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

// Package resolve works out, for one container, which version is running, which is
// the latest in the same family, and how big the update is.
//
// Running version, first hit wins:
//  1. the tag, when it's a full version (1.4.2, v1.15.0-ls175)
//  2. the image's version label, but only if the registry has a tag with that exact
//     version: labels are inherited from base images (mongo:8.3 says "24.04"). With a
//     word tag the label must be that word's variant (latest: plain), and channel tags
//     (nightly, dev) never take one
//  3. the registry tag whose digest equals the running image's digest
//  4. an image *_VERSION variable (PG_VERSION=16.10) that matches a tag and fits the
//     running tag; official images are rebuilt under the same tags, so old digests vanish
//  5. a partial version tag (16-alpine, 8.3) as-is
//  6. unknown
//
// Latest is the newest stable tag in the running version's family (same variant, and
// dates only with dates), on the running tag's numeric suffix line when that exists
// (node-red 4.0.2-20 → 4.0.5-20, postgis 16-3.4 → 17-3.5).
// Per-container Options adjust that:
//   - TagPattern: only registry tags matching it count as versions, for every step
//     above; the running tag itself is still read as a version.
//   - Prerelease: pre-release tags (rc, beta, nightly) can be Latest. Running-version
//     matching by digest or env stays stable-only, to keep HEAD requests bounded.
//   - Pin ("1.4") and HoldMajor keep Latest within that prefix or the running major
//     (both apply when both are set). The newest version they keep back is reported as
//     Held, so the UI can still mention it. HoldMajor needs a known major: with an
//     unknown running version (latest, nightly) it doesn't apply.
//   - Floating-tag digest updates (NewImage) ignore holds: they're the same tag.
//
// When the registry rate-limits the tag list but an earlier one is cached, Resolve
// works from that list and says so in TagsFrom.
package resolve

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/registry"
	"github.com/bartvs98/tidemark/internal/version"
)

// Registry is what Resolve needs from a registry client. Tags may return tags together
// with a *registry.StaleError: an older list, still usable.
type Registry interface {
	Tags(ctx context.Context, ref imageref.Ref) ([]string, error)
	Digest(ctx context.Context, ref imageref.Ref, tag string) (string, error)
}

// Source says how the running version was determined.
type Source string

const (
	SourceTag     Source = "tag"
	SourceLabel   Source = "label"
	SourceDigest  Source = "digest"
	SourceEnv     Source = "env"
	SourceUnknown Source = "unknown"
)

// Options are per-container rules; the zero value is the default behaviour.
type Options struct {
	Prerelease bool           // pre-release tags can be Latest
	TagPattern *regexp.Regexp // when set, only registry tags matching it count as versions
	Pin        string         // version prefix ("1.4", "16"): Latest must start with it
	HoldMajor  bool           // Latest stays within the running major
}

// Hold reasons for Result.HeldBy.
const (
	HeldByPin        = "pin"
	HeldByAllowMajor = "allow-major"
)

type Result struct {
	Running       string        // tag text of the running version ("v1.15.0-ls175"), empty if unknown
	Source        Source        // how Running was found
	Latest        string        // tag of the newest stable release in the same family, empty if none
	LatestInMajor string        // newest release within the running major, set when Latest is a major jump
	Class         version.Class // size of the update from Running (or the floating tag) to Latest
	Floating      bool          // the tag moves between releases (latest, 16-alpine, 8.3)
	NewImage      bool          // the floating tag now points to a different image than the one running
	Held          string        // newest version a pin or major hold keeps back, empty if none
	HeldBy        string        // HeldByPin or HeldByAllowMajor when Held is set
	TagDigest     string        // the floating tag's current digest in the registry, when it was fetched (floating tag with a local digest); empty otherwise
	// TagsFrom: the tag list was reused from an earlier check, fetched at this time,
	// because the registry rate-limited this one. Zero normally.
	TagsFrom time.Time
}

// maxDigestProbes bounds HEAD requests when matching a running image to a tag by digest.
const maxDigestProbes = 30

var (
	labelKeys      = []string{"org.opencontainers.image.version", "org.label-schema.version", "version"}
	buildVersionRe = regexp.MustCompile(`(?i)version:-\s*(\S+)`) // linuxserver build_version label
	plainTags      = map[string]bool{"latest": true, "stable": true, "release": true, "main": true, "master": true}
)

func Resolve(ctx context.Context, reg Registry, c docker.Container, opts Options) (Result, error) {
	var r Result
	ref, err := imageref.Parse(c.ImageRef)
	if err != nil {
		return r, err
	}
	rawTags, err := reg.Tags(ctx, ref)
	var stale *registry.StaleError
	if errors.As(err, &stale) {
		r.TagsFrom = stale.At
	} else if err != nil {
		return r, err
	}
	var tags []version.Version
	for _, t := range rawTags {
		if opts.TagPattern != nil && !opts.TagPattern.MatchString(t) {
			continue
		}
		if v, ok := version.Parse(t); ok {
			tags = append(tags, v)
		}
	}

	tagV, tagIsVersion := version.Parse(ref.Tag)
	r.Floating = ref.Tag != "" && (!tagIsVersion || len(tagV.Nums) < 3)
	local := localDigests(c.Image.RepoDigests, ref)

	// What the running image should look like, for tags that aren't versions:
	// "alpine" means the alpine family, "latest" the plain one.
	family := tagV
	floatingWord := ""
	if !tagIsVersion {
		floatingWord = strings.ToLower(ref.Tag)
		family = wordFamily(floatingWord)
	}

	// 1. A full version in the tag.
	var running version.Version
	have := false
	if tagIsVersion && !r.Floating {
		running, have, r.Source = tagV, true, SourceTag
	}

	// 2. The version label, if the registry has that exact tag. With a word tag it must be
	// that word's variant ("latest": plain), and a channel tag (nightly) never takes one:
	// its build is usually ahead of any release.
	if lv, ok := labelVersion(c.Image.Labels, tags); ok {
		switch {
		case !have && !tagIsVersion && family.Stable() && lv.SameVariant(family),
			!have && tagIsVersion && lv.SameFamily(tagV) && lv.HasPrefix(tagV) && fitsSuffix(lv, tagV):
			running, have, r.Source = lv, true, SourceLabel
		case have && lv.Build > running.Build && lv.SameFamily(running) && slices.Equal(lv.Nums, running.Nums):
			// Pinned "1.20.0" whose label says "1.20.0-ls166": the label knows the build.
			running, r.Source = lv, SourceLabel
		}
	}

	// 3. The registry tag with the same digest as the running image.
	if !have && len(local) > 0 && family.Stable() { // never for "nightly" and the like
		if v, ok := matchDigest(ctx, reg, ref, tags, family, tagIsVersion, floatingWord, local); ok {
			running, have, r.Source = v, true, SourceDigest
		}
	}
	if err := ctx.Err(); err != nil {
		return r, err
	}

	// 4. An image *_VERSION variable.
	if !have && family.Stable() {
		if v, ok := envVersion(c.Image.VersionEnv, tags, ref, family, tagIsVersion); ok {
			running, have, r.Source = v, true, SourceEnv
		}
	}

	// 5. A partial version tag, as is.
	if !have && tagIsVersion {
		running, have, r.Source = tagV, true, SourceTag
	}
	if !have {
		r.Source = SourceUnknown
	} else {
		r.Running = running.Raw
		family = running
	}

	// Latest release in the family, within any hold.
	cands := tags
	if !opts.Prerelease {
		cands = slices.DeleteFunc(slices.Clone(tags), func(t version.Version) bool { return !t.Stable() })
	}
	h := newHold(opts, running, have)
	latest, ok := newest(cands, family, h.allows)
	if ok {
		r.Latest = latest.Raw
		if have {
			r.Class = version.Classify(running, latest)
			if r.Class == version.Major && !h.active() {
				inMajor, ok := newest(cands, family, func(v version.Version) bool { return v.Nums[0] == running.Nums[0] })
				if ok && version.Compare(inMajor, running) > 0 {
					r.LatestInMajor = inMajor.Raw
				}
			}
		}
	}
	if h.active() {
		// Only worth mentioning if it's newer than both what we offer and what runs.
		if all, ok := newest(cands, family, nil); ok &&
			(r.Latest == "" || version.Compare(all, latest) > 0) &&
			(!have || version.Compare(all, running) > 0) {
			r.Held, r.HeldBy = all.Raw, h.reason(all)
		}
	}
	if r.Class == "" {
		r.Class = version.None
	}

	// A floating tag can move to a new image without any version change we can see.
	if r.Floating && len(local) > 0 {
		if d, err := reg.Digest(ctx, ref, ref.Tag); err == nil {
			r.TagDigest = d
			if !slices.Contains(local, d) {
				r.NewImage = true
				if r.Class == version.None {
					r.Class = version.Digest
				}
			}
		}
	}
	return r, nil
}

// Channel returns the tag, lower-cased, when it follows a pre-release channel
// (nightly, dev, edge, beta, …) rather than releases; empty otherwise. Such a build is
// usually ahead of the newest stable release, so the two aren't compared: only a new
// build under the same tag (NewImage) counts as an update.
func Channel(tag string) string {
	if tag == "" {
		return ""
	}
	if _, ok := version.Parse(tag); ok {
		return ""
	}
	word := strings.ToLower(tag)
	if plainTags[word] || wordFamily(word).Stable() {
		return ""
	}
	return word
}

// wordFamily maps a non-version tag to the family it follows: "alpine" → variant alpine,
// "latest" → plain stable releases, "nightly" → a pre-release family (never matched).
func wordFamily(word string) version.Version {
	if plainTags[word] {
		return version.Version{}
	}
	v, ok := version.Parse("0-" + word)
	if !ok {
		return version.Version{}
	}
	v.Nums, v.Suffix, v.Raw = nil, nil, ""
	return v
}

// fitsSuffix reports whether t is on running's suffix line: its suffix starts with
// running's ("16.4-3.4" and "16-3.4.2" fit "16-3.4", "16.4-3.5" doesn't). Without a
// running suffix, anything fits.
func fitsSuffix(t, running version.Version) bool {
	return len(t.Suffix) >= len(running.Suffix) && slices.Equal(t.Suffix[:len(running.Suffix)], running.Suffix)
}

func labelVersion(labels map[string]string, tags []version.Version) (version.Version, bool) {
	var candidates []string
	for _, k := range labelKeys {
		if v := strings.TrimSpace(labels[k]); v != "" {
			candidates = append(candidates, v)
		}
	}
	if m := buildVersionRe.FindStringSubmatch(labels["build_version"]); m != nil {
		candidates = append(candidates, m[1])
	}
	for _, c := range candidates {
		lv, ok := version.Parse(c)
		if !ok {
			continue
		}
		for _, t := range tags {
			if version.Matches(t, lv) {
				return t, true // the registry's spelling of the tag
			}
		}
	}
	return version.Version{}, false
}

// envAliases maps *_VERSION names that aren't part of the image name to the repository
// they name: PG_VERSION is postgres's.
var envAliases = map[string]string{"pg": "postgres"}

// envVersion looks for a *_VERSION variable naming a release the registry has, written
// like the running tag ("16.10" → "16.10-alpine"). With a version tag it must extend
// that tag; with a word tag (latest) the variable must be named after the image (or an
// alias in envAliases), so a NODE_VERSION inherited from a Node base image doesn't count.
func envVersion(env map[string]string, tags []version.Version, ref imageref.Ref, family version.Version, tagIsVersion bool) (version.Version, bool) {
	segs := strings.Split(ref.Repository, "/")
	repo := alnum(segs[len(segs)-1])
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		ev, ok := version.Parse(env[k])
		if !ok {
			continue
		}
		if tagIsVersion {
			if !ev.HasPrefix(family) {
				continue
			}
		} else if name := alnum(strings.TrimSuffix(k, "_VERSION")); name == "" ||
			!strings.Contains(repo, name) && (envAliases[name] == "" || !strings.Contains(repo, envAliases[name])) {
			continue
		}
		var best version.Version
		found := false
		for _, t := range tags {
			if !t.Stable() || !t.SameVariant(family) || !slices.Equal(t.Nums, ev.Nums) || !fitsSuffix(t, family) {
				continue
			}
			if !found || (t.RawVariant == family.RawVariant && best.RawVariant != family.RawVariant) {
				best, found = t, true
			}
		}
		if found {
			return best, true
		}
	}
	return version.Version{}, false
}

func alnum(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// matchDigest probes the most likely tags, newest first, for the running image's digest.
// Candidates are written like the running tag (same raw variant, so "16.10-alpine" for
// "16-alpine", not "16.10-alpine3.22"; same suffix line, so "16.4-3.4" for "16-3.4").
// A tag is skipped when a more precise one starts with it ("17" when "17.6" exists): it
// points to the same image. Precision is per line, so an old "9.6.24" doesn't push out
// a current "18.0"; it just sorts last.
func matchDigest(ctx context.Context, reg Registry, ref imageref.Ref, tags []version.Version,
	family version.Version, tagIsVersion bool, word string, local []string) (version.Version, bool) {

	rawVariant := family.RawVariant
	if !tagIsVersion && !plainTags[word] {
		rawVariant = word
	}
	var cands []version.Version
	for _, t := range tags {
		if !t.Stable() || t.RawVariant != rawVariant || t.Date != family.Date ||
			(tagIsVersion && (!t.HasPrefix(family) || !fitsSuffix(t, family))) {
			continue
		}
		cands = append(cands, t)
	}
	cands = slices.DeleteFunc(slices.Clone(cands), func(t version.Version) bool {
		return slices.ContainsFunc(cands, func(o version.Version) bool {
			return len(o.Nums) > len(t.Nums) && o.HasPrefix(t) && slices.Equal(o.Suffix, t.Suffix)
		})
	})
	slices.SortFunc(cands, func(a, b version.Version) int { return version.Compare(b, a) })

	for i, t := range cands {
		if i == maxDigestProbes || ctx.Err() != nil {
			break
		}
		d, err := reg.Digest(ctx, ref, t.Raw)
		if err == nil && slices.Contains(local, d) {
			return t, true
		}
		// A missing tag (or an unclassified error) only skips that tag; a rate limit,
		// timeout, network or server failure would hit every further probe too.
		switch registry.KindOf(err) {
		case registry.KindRateLimit, registry.KindTimeout, registry.KindNetwork, registry.KindRegistry:
			return version.Version{}, false
		}
	}
	return version.Version{}, false
}

// hold keeps Latest within a pinned prefix and/or a major.
type hold struct {
	pin      version.Version
	hasPin   bool
	major    int
	hasMajor bool
}

// newHold reads the options; an unparsable pin or an unknown running major means no hold.
func newHold(opts Options, running version.Version, have bool) hold {
	var h hold
	if opts.Pin != "" {
		h.pin, h.hasPin = version.Parse(opts.Pin)
	}
	if opts.HoldMajor && have {
		h.major, h.hasMajor = running.Nums[0], true
	}
	return h
}

func (h hold) active() bool { return h.hasPin || h.hasMajor }

func (h hold) allows(v version.Version) bool { return h.reason(v) == "" }

// reason names the constraint that excludes v, the pin first, or "" if v is allowed.
func (h hold) reason(v version.Version) string {
	switch {
	case h.hasPin && !v.HasPrefix(h.pin):
		return HeldByPin
	case h.hasMajor && v.Nums[0] != h.major:
		return HeldByAllowMajor
	}
	return ""
}

// newest returns the highest tag in the family, preferring the family's own
// spelling ("v1.4.0" over "1.4.0" when the running tag has a v) on ties. Among tags
// with the newest numbers it prefers the family's suffix line: node-red "4.0.2-20"
// gets "4.0.5-20", not "4.0.5-22" (another Node runtime), and "4.0.2" gets "4.0.5".
// When that line doesn't exist for the newest numbers, the highest suffix wins, so
// a revision line that ends ("2.4.1-1" → only "2.4.3-2") doesn't freeze updates.
func newest(tags []version.Version, family version.Version, keep func(version.Version) bool) (version.Version, bool) {
	best, found := highest(tags, family, keep)
	if !found {
		return best, false
	}
	sameLine := func(t version.Version) bool {
		return (keep == nil || keep(t)) && slices.Equal(t.Nums, best.Nums) && t.Pre == best.Pre &&
			(len(t.Suffix) > 0) == (len(family.Suffix) > 0) && (len(t.Suffix) == 0 || t.Suffix[0] == family.Suffix[0])
	}
	if line, ok := highest(tags, family, sameLine); ok {
		best = line
	}
	return best, true
}

// highest returns the highest tag in the family that keep allows, with newest's
// spelling tie-breaks.
func highest(tags []version.Version, family version.Version, keep func(version.Version) bool) (version.Version, bool) {
	var best version.Version
	found := false
	for _, t := range tags {
		if !t.SameVariant(family) || (keep != nil && !keep(t)) {
			continue
		}
		if !found {
			best, found = t, true
			continue
		}
		switch c := version.Compare(t, best); {
		case c > 0:
			best = t
		case c == 0 && t.Prefix == family.Prefix && best.Prefix != family.Prefix:
			best = t
		case c == 0 && t.RawVariant == family.RawVariant && best.RawVariant != family.RawVariant:
			best = t
		}
	}
	return best, found
}

// localDigests returns the manifest digests Docker recorded for this repository
// ("redis@sha256:…" → "sha256:…").
func localDigests(repoDigests []string, ref imageref.Ref) []string {
	var out []string
	for _, rd := range repoDigests {
		name, digest, ok := strings.Cut(rd, "@")
		if !ok {
			continue
		}
		if r, err := imageref.Parse(name); err == nil && r.Registry == ref.Registry && r.Repository == ref.Repository {
			out = append(out, digest)
		}
	}
	return out
}

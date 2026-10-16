// Package resolve works out, for one container, which version is running, which is
// the latest in the same family, and how big the update is.
//
// Running version, first hit wins:
//  1. the tag, when it's a full version (1.4.2, v1.15.0-ls175)
//  2. the image's version label, but only if the registry has a tag with that exact
//     version: labels are inherited from base images (mongo:8.3 says "24.04")
//  3. the registry tag whose digest equals the running image's digest
//  4. an image *_VERSION variable (PG_VERSION=16.10) that matches a tag and fits the
//     running tag; official images are rebuilt under the same tags, so old digests vanish
//  5. a partial version tag (16-alpine, 8.3) as-is
//  6. unknown
package resolve

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/version"
)

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

type Result struct {
	Running       string        // tag text of the running version ("v1.15.0-ls175"), empty if unknown
	Source        Source        // how Running was found
	Latest        string        // tag of the newest stable release in the same family, empty if none
	LatestInMajor string        // newest release within the running major, set when Latest is a major jump
	Class         version.Class // size of the update from Running (or the floating tag) to Latest
	Floating      bool          // the tag moves between releases (latest, 16-alpine, 8.3)
	NewImage      bool          // the floating tag now points to a different image than the one running
}

// maxDigestProbes bounds HEAD requests when matching a running image to a tag by digest.
const maxDigestProbes = 30

var (
	labelKeys      = []string{"org.opencontainers.image.version", "org.label-schema.version", "version"}
	buildVersionRe = regexp.MustCompile(`(?i)version:-\s*(\S+)`) // linuxserver build_version label
	plainTags      = map[string]bool{"latest": true, "stable": true, "release": true, "main": true, "master": true}
)

func Resolve(ctx context.Context, reg Registry, c docker.Container) (Result, error) {
	var r Result
	ref, err := imageref.Parse(c.ImageRef)
	if err != nil {
		return r, err
	}
	rawTags, err := reg.Tags(ctx, ref)
	if err != nil {
		return r, err
	}
	var tags []version.Version
	for _, t := range rawTags {
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

	// 2. The version label, if the registry has that exact tag.
	if lv, ok := labelVersion(c.Image.Labels, tags); ok {
		switch {
		case !have && (!tagIsVersion || (lv.SameFamily(tagV) && lv.HasPrefix(tagV))):
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

	// Latest stable release in the family.
	latest, ok := newest(tags, family, nil)
	if ok {
		r.Latest = latest.Raw
		if have {
			r.Class = version.Classify(running, latest)
			if r.Class == version.Major {
				inMajor, ok := newest(tags, family, func(v version.Version) bool { return v.Nums[0] == running.Nums[0] })
				if ok && version.Compare(inMajor, running) > 0 {
					r.LatestInMajor = inMajor.Raw
				}
			}
		}
	}
	if r.Class == "" {
		r.Class = version.None
	}

	// A floating tag can move to a new image without any version change we can see.
	if r.Floating && len(local) > 0 {
		if d, err := reg.Digest(ctx, ref, ref.Tag); err == nil && !slices.Contains(local, d) {
			r.NewImage = true
			if r.Class == version.None {
				r.Class = version.Digest
			}
		}
	}
	return r, nil
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
	v.Nums, v.Raw = nil, ""
	return v
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

// envVersion looks for a *_VERSION variable naming a release the registry has, written
// like the running tag ("16.10" → "16.10-alpine"). With a version tag it must extend
// that tag; with a word tag (latest) the variable must be named after the image, so a
// NODE_VERSION inherited from a Node base image doesn't count.
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
		} else if name := alnum(strings.TrimSuffix(k, "_VERSION")); name == "" || !strings.Contains(repo, name) {
			continue
		}
		var best version.Version
		found := false
		for _, t := range tags {
			if !t.Stable() || t.Variant != family.Variant || !slices.Equal(t.Nums, ev.Nums) {
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
// "16-alpine", not "16.10-alpine3.22") at the most precise level the registry uses.
func matchDigest(ctx context.Context, reg Registry, ref imageref.Ref, tags []version.Version,
	family version.Version, tagIsVersion bool, word string, local []string) (version.Version, bool) {

	rawVariant := family.RawVariant
	if !tagIsVersion && !plainTags[word] {
		rawVariant = word
	}
	var cands []version.Version
	precision := 0
	for _, t := range tags {
		if !t.Stable() || t.RawVariant != rawVariant || (tagIsVersion && !t.HasPrefix(family)) {
			continue
		}
		cands = append(cands, t)
		precision = max(precision, len(t.Nums))
	}
	cands = slices.DeleteFunc(cands, func(t version.Version) bool { return len(t.Nums) < precision })
	slices.SortFunc(cands, func(a, b version.Version) int { return version.Compare(b, a) })

	for i, t := range cands {
		if i == maxDigestProbes || ctx.Err() != nil {
			break
		}
		d, err := reg.Digest(ctx, ref, t.Raw)
		if err == nil && slices.Contains(local, d) {
			return t, true
		}
	}
	return version.Version{}, false
}

// newest returns the highest stable tag in the family, preferring the family's own
// spelling ("v1.4.0" over "1.4.0" when the running tag has a v) on ties.
func newest(tags []version.Version, family version.Version, keep func(version.Version) bool) (version.Version, bool) {
	var best version.Version
	found := false
	for _, t := range tags {
		if !t.Stable() || t.Variant != family.Variant || (keep != nil && !keep(t)) {
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

// Package rules reads per-container rules (ignore, allow-major, pin, prerelease,
// tag-pattern, repo) from container labels and an optional YAML config file, and
// merges them per field: container label > containers: entry > images: entry > default.
//
// Invalid values never fail a container: the rule is skipped and a short problem is
// reported in Rules.Problems. Applying the rules is up to the caller.
package rules

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Rule keys, shared by labels (prefixed with "tidemark."), the config file and Rules.From.
const (
	KeyIgnore     = "ignore"
	KeyAllowMajor = "allow-major"
	KeyPin        = "pin"
	KeyPrerelease = "prerelease"
	KeyTagPattern = "tag-pattern"
	KeyRepo       = "repo"
)

// LabelPrefix starts every rule label. tidemark.repo is the same label internal/source reads.
const LabelPrefix = "tidemark."

// Sources recorded in Rules.From.
const (
	FromLabel  = "label"
	FromConfig = "config"
)

// Rules are the effective rules for one container.
type Rules struct {
	Ignore     bool
	AllowMajor bool
	Pin        string // version prefix without "v": "1.4", "16"
	Prerelease bool
	TagPattern *regexp.Regexp
	Repo       string
	From       map[string]string // rule key → FromLabel | FromConfig, only for explicitly set rules
	Problems   []string
}

// Default is what applies when nothing is set.
func Default() Rules { return Rules{AllowMajor: true} }

// Active reports whether any effective rule differs from Default().
func (r Rules) Active() bool {
	return r.Ignore || !r.AllowMajor || r.Pin != "" || r.Prerelease || r.TagPattern != nil || r.Repo != ""
}

// Fingerprint changes whenever an effective rule changes, so stored check results
// can be invalidated. It is "" for the defaults, so existing results stay valid
// for containers without rules. Problems and From don't count.
func (r Rules) Fingerprint() string {
	if !r.Active() {
		return ""
	}
	pattern := ""
	if r.TagPattern != nil {
		pattern = r.TagPattern.String()
	}
	// Quoted values keep the string unambiguous whatever the pin, pattern or repo contain.
	return fmt.Sprintf("ignore=%t;allow-major=%t;pin=%q;prerelease=%t;tag-pattern=%q;repo=%q",
		r.Ignore, r.AllowMajor, r.Pin, r.Prerelease, pattern, r.Repo)
}

// set holds explicitly set rules from one place (labels or one config entry);
// nil means not set.
type set struct {
	ignore, allowMajor, prerelease *bool
	pin, repo                      *string
	tagPattern                     *regexp.Regexp
}

// apply fills each field of r that isn't set yet from s; done tracks what's set.
func (s set) apply(r *Rules, done map[string]bool, from string) {
	mark := func(key string) bool {
		if done[key] {
			return false
		}
		done[key] = true
		r.From[key] = from
		return true
	}
	if s.ignore != nil && mark(KeyIgnore) {
		r.Ignore = *s.ignore
	}
	if s.allowMajor != nil && mark(KeyAllowMajor) {
		r.AllowMajor = *s.allowMajor
	}
	if s.pin != nil && mark(KeyPin) {
		r.Pin = *s.pin
	}
	if s.prerelease != nil && mark(KeyPrerelease) {
		r.Prerelease = *s.prerelease
	}
	if s.tagPattern != nil && mark(KeyTagPattern) {
		r.TagPattern = s.tagPattern
	}
	if s.repo != nil && mark(KeyRepo) {
		r.Repo = *s.repo
	}
}

// parse validates raw values (rule key → value) into a set. where prefixes problems,
// e.g. "tidemark." for labels or "config containers.vaultwarden " for the file.
// Empty values count as not set.
func parse(raw map[string]string, where string) (set, []string) {
	var s set
	var problems []string
	bad := func(key, format string, args ...any) {
		problems = append(problems, where+key+": "+fmt.Sprintf(format, args...))
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys) // stable problem order
	for _, key := range keys {
		v := strings.TrimSpace(raw[key])
		if v == "" {
			continue
		}
		switch key {
		case KeyIgnore, KeyAllowMajor, KeyPrerelease:
			b, ok := parseBool(v)
			if !ok {
				bad(key, "%q is not true or false", v)
				continue
			}
			switch key {
			case KeyIgnore:
				s.ignore = &b
			case KeyAllowMajor:
				s.allowMajor = &b
			default:
				s.prerelease = &b
			}
		case KeyPin:
			p, ok := parsePin(v)
			if !ok {
				bad(key, "%q is not a version prefix", v)
				continue
			}
			s.pin = &p
		case KeyTagPattern:
			re, err := regexp.Compile(v)
			if err != nil {
				bad(key, "invalid regex: %v", err)
				continue
			}
			s.tagPattern = re
		case KeyRepo:
			s.repo = &v
		}
	}
	return s, problems
}

func parseBool(v string) (bool, bool) {
	switch strings.ToLower(v) {
	case "true", "yes", "1":
		return true, true
	case "false", "no", "0":
		return false, true
	}
	return false, false
}

var pinRe = regexp.MustCompile(`^[vV]?(\d+(?:\.\d+)*)$`)

// parsePin accepts "1", "1.4", "v2" and returns it without the v.
func parsePin(v string) (string, bool) {
	m := pinRe.FindStringSubmatch(v)
	if m == nil {
		return "", false
	}
	// Strip leading zeros per component so "01.4" and "1.4" mean the same pin.
	parts := strings.Split(m[1], ".")
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return "", false
		}
		parts[i] = strconv.FormatUint(n, 10)
	}
	return strings.Join(parts, "."), true
}

// fromLabels reads the tidemark.* rule labels.
func fromLabels(labels map[string]string) (set, []string) {
	raw := map[string]string{}
	for _, key := range []string{KeyIgnore, KeyAllowMajor, KeyPin, KeyPrerelease, KeyTagPattern, KeyRepo} {
		if v, ok := labels[LabelPrefix+key]; ok {
			raw[key] = v
		}
	}
	return parse(raw, LabelPrefix)
}

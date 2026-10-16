package changelog

import (
	"regexp"
	"strings"
	"time"

	"github.com/bartvs98/tidemark/internal/store"
)

var (
	headingRe = regexp.MustCompile(`^(#{1,4})\s+(.+?)\s*#*\s*$`)
	// A version in a heading: "## [1.4.0] - 2026-01-02", "## v1.4.0", "# Version 1.4.0 (2026-01-02)".
	headingVersionRe = regexp.MustCompile(`\[?v?(\d+(?:\.\d+)+(?:-[0-9A-Za-z][0-9A-Za-z.\-]*)?)\]?`)
	dateRe           = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
)

// parseChangelog splits a Keep-a-Changelog style file into one entry per version heading.
// Headings without a version ("Unreleased", "### Fixed") stay inside the current section.
func parseChangelog(md, fileURL string) []store.Release {
	var out []store.Release
	var cur *store.Release
	level := 0
	var body []string
	inFence := false

	flush := func() {
		if cur != nil {
			cur.Body = strings.TrimSpace(strings.Join(body, "\n"))
			out = append(out, *cur)
		}
		cur, body = nil, nil
	}

	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if !inFence {
			if m := headingRe.FindStringSubmatch(line); m != nil {
				lvl := len(m[1])
				if v := headingVersionRe.FindStringSubmatch(m[2]); v != nil && (cur == nil || lvl <= level) {
					flush()
					cur = &store.Release{Tag: v[1], Name: strings.TrimSpace(m[2]), URL: fileURL}
					if d := dateRe.FindString(m[2]); d != "" {
						cur.PublishedAt, _ = time.Parse("2006-01-02", d)
					}
					level = lvl
					continue
				}
				if cur != nil && lvl <= level {
					flush() // e.g. "## Unreleased" after a version section ends it
					continue
				}
			}
		}
		if cur != nil {
			body = append(body, line)
		}
	}
	flush()
	return out
}

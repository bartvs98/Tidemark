// Package source finds the source code repository (GitHub or GitLab) of a container's
// image, where its release notes live.
//
// Order, first hit wins:
//  1. the repo rule (tidemark.repo label or repo: in the config file; set by the
//     user, always trusted)
//  2. the image label org.opencontainers.image.source (or org.label-schema.vcs-url),
//     if it plausibly belongs to this image: labels are inherited from base images
//     (plex-meta-manager says linuxserver/docker-baseimage-ubuntu)
//  3. ghcr.io/owner/name → github.com/owner/name
//  4. a short list of official Docker Hub images (redis → redis/redis)
//  5. docker.io/namespace/name → github.com/namespace/name (a guess; may not exist)
package source

import (
	"net/url"
	"strings"

	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/rules"
)

// Repo is a repository on a supported host.
type Repo struct {
	Host string // github.com | gitlab.com
	Path string // owner/name; GitLab paths can include subgroups
}

// Key identifies the repo in the store (hosts are case-insensitive).
func (r Repo) Key() string { return strings.ToLower(r.Host + "/" + r.Path) }

func (r Repo) URL() string { return "https://" + r.Host + "/" + r.Path }

func (r Repo) IsGitHub() bool { return r.Host == "github.com" }

// How says how the repository was found.
type How string

const (
	HowLabel    How = "label"    // tidemark.repo label
	HowConfig   How = "config"   // repo: in the config file
	HowImage    How = "image"    // the image's source label
	HowRegistry How = "registry" // ghcr.io path
	HowKnown    How = "known"    // official image list
	HowGuess    How = "guess"    // Docker Hub namespace/name
)

// Official images without a source label, and where their releases are.
var known = map[string]string{
	"library/redis":     "github.com/redis/redis",
	"library/postgres":  "github.com/postgres/postgres",
	"library/mongo":     "github.com/mongodb/mongo",
	"library/mariadb":   "github.com/MariaDB/server",
	"library/nginx":     "github.com/nginx/nginx",
	"library/traefik":   "github.com/traefik/traefik",
	"library/caddy":     "github.com/caddyserver/caddy",
	"library/memcached": "github.com/memcached/memcached",
	"library/rabbitmq":  "github.com/rabbitmq/rabbitmq-server",
	"library/node":      "github.com/nodejs/node",
	"library/python":    "github.com/python/cpython",
}

// Resolve finds the repository for a container with the given effective rules. An
// unusable repo rule (unsupported host) falls through to the automatic chain.
func Resolve(c docker.Container, ref imageref.Ref, rs rules.Rules) (Repo, How, bool) {
	if r, ok := ParseURL(rs.Repo); ok {
		if rs.From[rules.KeyRepo] == rules.FromConfig {
			return r, HowConfig, true
		}
		return r, HowLabel, true
	}
	for _, key := range []string{"org.opencontainers.image.source", "org.label-schema.vcs-url"} {
		if r, ok := ParseURL(c.Image.Labels[key]); ok && plausible(r, ref) {
			return r, HowImage, true
		}
	}
	segs := strings.Split(ref.Repository, "/")
	if ref.Registry == "ghcr.io" && len(segs) >= 2 {
		return Repo{Host: "github.com", Path: segs[0] + "/" + segs[1]}, HowRegistry, true
	}
	if ref.Registry == imageref.DockerHub {
		if k, ok := known[ref.Repository]; ok {
			r, _ := ParseURL(k)
			return r, HowKnown, true
		}
		if len(segs) == 2 && segs[0] != "library" {
			return Repo{Host: "github.com", Path: ref.Repository}, HowGuess, true
		}
	}
	return Repo{}, "", false
}

// ParseURL accepts https://github.com/o/r(.git)(/…), github.com/o/r, git@github.com:o/r.git
// and the GitLab equivalents (including subgroups). Other hosts aren't supported.
func ParseURL(s string) (Repo, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Repo{}, false
	}
	if strings.HasPrefix(s, "git@") {
		s = "https://" + strings.Replace(strings.TrimPrefix(s, "git@"), ":", "/", 1)
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return Repo{}, false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	path := strings.Trim(u.Path, "/")
	var parts []string
	switch host {
	case "github.com":
		parts = strings.Split(path, "/")
		if len(parts) > 2 {
			parts = parts[:2] // drop /tree/main, /releases, …
		}
	case "gitlab.com":
		if i := strings.Index(path, "/-/"); i >= 0 {
			path = path[:i]
		}
		parts = strings.Split(path, "/")
	default:
		return Repo{}, false
	}
	if len(parts) < 2 {
		return Repo{}, false
	}
	parts[len(parts)-1] = strings.TrimSuffix(parts[len(parts)-1], ".git")
	for _, p := range parts {
		if p == "" {
			return Repo{}, false
		}
	}
	return Repo{Host: host, Path: strings.Join(parts, "/")}, true
}

// plausible: the repo's owner is the image's namespace, or the names share a word.
func plausible(r Repo, ref imageref.Ref) bool {
	segs := strings.Split(ref.Repository, "/")
	repoParts := strings.Split(r.Path, "/")
	owner, name := repoParts[0], repoParts[len(repoParts)-1]

	if len(segs) >= 2 && norm(segs[0]) == norm(owner) {
		return true
	}
	// Compare the repo name with every part of the image path: "vaultwarden/server" is
	// named after its namespace, and "server" itself says nothing.
	repoWords := words(name)
	for _, seg := range segs {
		if generic[strings.ToLower(seg)] {
			continue
		}
		if a, b := norm(name), norm(seg); a != "" && b != "" && (strings.Contains(a, b) || strings.Contains(b, a)) {
			return true
		}
		for w := range words(seg) {
			if repoWords[w] {
				return true
			}
		}
	}
	return false
}

func norm(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Words that say nothing about which app an image or repo is.
var generic = map[string]bool{
	"docker": true, "image": true, "server": true, "app": true, "base": true, "baseimage": true,
	"web": true, "ui": true, "api": true, "library": true,
}

// words splits a name on separators, ignoring generic words.
func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r == '-' || r == '_' || r == '.' }) {
		if generic[w] {
			continue
		}
		if len(w) >= 3 {
			out[w] = true
		}
	}
	return out
}

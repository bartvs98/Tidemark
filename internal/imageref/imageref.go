// Package imageref parses Docker image references like "redis", "louislam/uptime-kuma:2"
// or "ghcr.io/sergi0g/cup:v3.5.1@sha256:…" into registry, repository, tag and digest,
// applying Docker's defaults (docker.io, library/, :latest).
package imageref

import (
	"fmt"
	"strings"
)

const DockerHub = "docker.io"

type Ref struct {
	Registry   string // docker.io, ghcr.io, lscr.io, localhost:5000 …
	Repository string // library/redis, linuxserver/sonarr
	Tag        string // empty only when the reference is pinned by digest alone
	Digest     string // sha256:…, if pinned
}

func Parse(s string) (Ref, error) {
	var r Ref
	name := strings.TrimSpace(s)
	if name == "" {
		return r, fmt.Errorf("empty image reference")
	}

	if i := strings.Index(name, "@"); i >= 0 {
		r.Digest = name[i+1:]
		name = name[:i]
		if !strings.Contains(r.Digest, ":") {
			return Ref{}, fmt.Errorf("invalid digest in %q", s)
		}
	}

	// A tag is a ':' after the last '/' (an earlier ':' is a registry port).
	if i := strings.LastIndex(name, ":"); i > strings.LastIndex(name, "/") {
		r.Tag = name[i+1:]
		name = name[:i]
		if r.Tag == "" {
			return Ref{}, fmt.Errorf("empty tag in %q", s)
		}
	}

	// The first path component is a registry if it looks like a host.
	r.Registry = DockerHub
	if first, rest, ok := strings.Cut(name, "/"); ok && (strings.ContainsAny(first, ".:") || first == "localhost") {
		r.Registry = first
		name = rest
	}
	switch r.Registry {
	case "index.docker.io", "registry-1.docker.io":
		r.Registry = DockerHub
	}

	if name == "" || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.Contains(name, "//") {
		return Ref{}, fmt.Errorf("invalid repository in %q", s)
	}
	if name != strings.ToLower(name) {
		return Ref{}, fmt.Errorf("repository must be lowercase in %q", s)
	}
	if r.Registry == DockerHub && !strings.Contains(name, "/") {
		name = "library/" + name
	}
	r.Repository = name

	if r.Tag == "" && r.Digest == "" {
		r.Tag = "latest"
	}
	return r, nil
}

// String is the fully qualified form: docker.io/library/redis:latest.
func (r Ref) String() string {
	return r.Registry + "/" + r.Repository + r.suffix()
}

// Familiar is the short form people write: redis:latest, louislam/uptime-kuma:latest, ghcr.io/x/y:v1.
func (r Ref) Familiar() string {
	name := r.Registry + "/" + r.Repository
	if r.Registry == DockerHub {
		name = strings.TrimPrefix(r.Repository, "library/")
	}
	return name + r.suffix()
}

func (r Ref) suffix() string {
	s := ""
	if r.Tag != "" {
		s += ":" + r.Tag
	}
	if r.Digest != "" {
		s += "@" + r.Digest
	}
	return s
}

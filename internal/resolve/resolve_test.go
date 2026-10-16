package resolve

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/version"
)

// fakeRegistry serves tags for one repository; digests maps tag → digest.
type fakeRegistry struct {
	tags    []string
	digests map[string]string
	heads   []string // tags whose digest was requested
}

func (f *fakeRegistry) Tags(context.Context, imageref.Ref) ([]string, error) { return f.tags, nil }

func (f *fakeRegistry) Digest(_ context.Context, _ imageref.Ref, tag string) (string, error) {
	f.heads = append(f.heads, tag)
	if d, ok := f.digests[tag]; ok {
		return d, nil
	}
	return "", errors.New("not found")
}

func container(image, repoDigest string, labels map[string]string) docker.Container {
	c := docker.Container{Name: "x", ImageRef: image, Image: docker.Image{Labels: labels}}
	if repoDigest != "" {
		c.Image.RepoDigests = []string{repoDigest}
	}
	return c
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		c       docker.Container
		reg     *fakeRegistry
		want    Result
		noHeads bool // pinned tags must not cost any HEAD requests
	}{
		{
			name:    "pinned tag, patch update",
			c:       container("vaultwarden/server:1.37.2", "vaultwarden/server@sha256:a", map[string]string{"org.opencontainers.image.version": "1.37.2"}),
			reg:     &fakeRegistry{tags: []string{"1.36.0", "1.37.2", "1.37.3", "1.37.4", "1.37.4-alpine", "latest"}},
			want:    Result{Running: "1.37.2", Source: SourceTag, Latest: "1.37.4", Class: version.Patch},
			noHeads: true,
		},
		{
			name: "label from base image is ignored, digest match finds the version",
			c:    container("mongo:8.3", "mongo@sha256:832", map[string]string{"org.opencontainers.image.version": "24.04"}),
			reg: &fakeRegistry{
				tags:    []string{"8.0.10", "8.3", "8.3.1", "8.3.2", "8.3.3", "9.0.4", "9.0.4-windowsservercore-ltsc2025", "9", "latest"},
				digests: map[string]string{"8.3.3": "sha256:833", "8.3.2": "sha256:832", "8.3": "sha256:833"},
			},
			want: Result{Running: "8.3.2", Source: SourceDigest, Latest: "9.0.4", LatestInMajor: "8.3.3", Class: version.Major, Floating: true, NewImage: true},
		},
		{
			name: "latest tag with a trustworthy label",
			c:    container("homeassistant/home-assistant:latest", "homeassistant/home-assistant@sha256:old", map[string]string{"org.opencontainers.image.version": "2026.9.0"}),
			reg: &fakeRegistry{
				tags:    []string{"2026.9.0", "2026.9.4", "2026.10", "2026.10.0", "2026.10.1", "dev", "latest"},
				digests: map[string]string{"latest": "sha256:new"},
			},
			want: Result{Running: "2026.9.0", Source: SourceLabel, Latest: "2026.10.1", Class: version.Minor, Floating: true, NewImage: true},
		},
		{
			name:    "pinned linuxserver tag, label knows the build: packaging update",
			c:       container("lscr.io/linuxserver/thelounge:1.20.0", "lscr.io/linuxserver/thelounge@sha256:a", map[string]string{"org.opencontainers.image.version": "1.20.0-ls166"}),
			reg:     &fakeRegistry{tags: []string{"1.20.0", "1.20.0-ls165", "1.20.0-ls166", "1.20.0-ls168", "latest"}},
			want:    Result{Running: "1.20.0-ls166", Source: SourceLabel, Latest: "1.20.0-ls168", Class: version.Digest},
			noHeads: true,
		},
		{
			name: "variant tag: matches the same spelling, prefers it for latest",
			c:    container("postgres:16-alpine", "postgres@sha256:169", nil),
			reg: &fakeRegistry{
				tags: []string{"16-alpine", "16.9-alpine", "16.9-alpine3.21", "16.10-alpine", "16.10-alpine3.22", "16.10", "18.6-alpine3.24", "18.6-alpine", "18.6"},
				digests: map[string]string{
					"16.10-alpine": "sha256:1610", "16.9-alpine": "sha256:169", "16.9-alpine3.21": "sha256:169", "16-alpine": "sha256:1610",
				},
			},
			want: Result{Running: "16.9-alpine", Source: SourceDigest, Latest: "18.6-alpine", LatestInMajor: "16.10-alpine", Class: version.Major, Floating: true, NewImage: true},
		},
		{
			name: "latest without label: digest match among the most precise tags",
			c:    container("redis:latest", "redis@sha256:8101", nil),
			reg: &fakeRegistry{
				tags:    []string{"8", "8.10", "8.10.1", "8.10.2", "8.10.2-alpine", "latest"},
				digests: map[string]string{"8.10.2": "sha256:8102", "8.10.1": "sha256:8101", "latest": "sha256:8102"},
			},
			want: Result{Running: "8.10.1", Source: SourceDigest, Latest: "8.10.2", Class: version.Patch, Floating: true, NewImage: true},
		},
		{
			name: "variant word tag with label",
			c:    container("woodpeckerci/woodpecker-agent:alpine", "woodpeckerci/woodpecker-agent@sha256:a", map[string]string{"org.opencontainers.image.version": "0.21.0-alpine"}),
			reg: &fakeRegistry{
				tags:    []string{"0.20.0", "0.20.0-alpine", "0.21.0", "0.21.0-alpine", "alpine", "latest"},
				digests: map[string]string{"alpine": "sha256:a"},
			},
			want: Result{Running: "0.21.0-alpine", Source: SourceLabel, Latest: "0.21.0-alpine", Class: version.None, Floating: true},
		},
		{
			name: "label matches a v-prefixed tag; latest keeps the v",
			c:    container("filebrowser/filebrowser:latest", "filebrowser/filebrowser@sha256:a", map[string]string{"org.opencontainers.image.version": "v11.3.1"}),
			reg: &fakeRegistry{
				tags:    []string{"v11.3.1", "v11.3.2", "11.3.2", "v11.3.2-alpine", "latest"},
				digests: map[string]string{"latest": "sha256:b"},
			},
			want: Result{Running: "v11.3.1", Source: SourceLabel, Latest: "v11.3.2", Class: version.Patch, Floating: true, NewImage: true},
		},
		{
			name: "nightly: unknown version, no digest probing, new image detected",
			c:    container("ghcr.io/mealie-recipes/mealie:nightly", "ghcr.io/mealie-recipes/mealie@sha256:old", nil),
			reg: &fakeRegistry{
				tags:    []string{"nightly", "v2.35.9", "v2.36.0", "latest"},
				digests: map[string]string{"nightly": "sha256:new"},
			},
			want: Result{Source: SourceUnknown, Latest: "v2.36.0", Class: version.Digest, Floating: true, NewImage: true},
		},
		{
			name: "unknown and unchanged",
			c:    container("jlesage/mkvtoolnix:latest", "jlesage/mkvtoolnix@sha256:a", nil),
			reg: &fakeRegistry{
				tags:    []string{"4.22.8", "4.23.8", "latest"},
				digests: map[string]string{"latest": "sha256:a"},
			},
			want: Result{Source: SourceUnknown, Latest: "4.23.8", Class: version.None, Floating: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(context.Background(), tt.reg, tt.c)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
			if tt.noHeads && len(tt.reg.heads) > 0 {
				t.Errorf("pinned tag made HEAD requests: %v", tt.reg.heads)
			}
			if tt.name == "nightly: unknown version, no digest probing, new image detected" && !slices.Equal(tt.reg.heads, []string{"nightly"}) {
				t.Errorf("heads = %v, want only the nightly tag", tt.reg.heads)
			}
		})
	}
}

func TestResolveEnvVersion(t *testing.T) {
	tags := []string{"16.9-alpine", "16.9-alpine3.21", "16.10-alpine", "16.10", "18.6-alpine", "latest", "8.10.1", "8.10.2"}
	tests := []struct {
		name, image string
		env         map[string]string
		wantRunning string
		wantSource  Source
	}{
		{"rebuilt official image: digest gone, env knows", "postgres:16-alpine", map[string]string{"PG_VERSION": "16.9", "PG_MAJOR": "16"}, "16.9-alpine", SourceEnv},
		{"env must extend the tag", "postgres:16-alpine", map[string]string{"PG_VERSION": "18.6"}, "16-alpine", SourceTag},
		{"latest: variable named after the image", "redis:latest", map[string]string{"REDIS_VERSION": "8.10.1"}, "8.10.1", SourceEnv},
		{"latest: base-image variable ignored", "team/app:latest", map[string]string{"NODE_VERSION": "8.10.1"}, "", SourceUnknown},
		{"value must exist as a tag", "redis:latest", map[string]string{"REDIS_VERSION": "8.9.0"}, "", SourceUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := container(tt.image, "x@sha256:none", nil)
			c.Image.VersionEnv = tt.env
			got, err := Resolve(context.Background(), &fakeRegistry{tags: tags}, c)
			if err != nil {
				t.Fatal(err)
			}
			if got.Running != tt.wantRunning || got.Source != tt.wantSource {
				t.Errorf("running = %q via %s, want %q via %s", got.Running, got.Source, tt.wantRunning, tt.wantSource)
			}
		})
	}
}

func TestLocalDigestsFiltersRepository(t *testing.T) {
	ref, _ := imageref.Parse("redis:latest")
	got := localDigests([]string{"redis@sha256:a", "docker.io/library/redis@sha256:b", "ghcr.io/x/redis@sha256:c", "garbage"}, ref)
	if !slices.Equal(got, []string{"sha256:a", "sha256:b"}) {
		t.Errorf("got %v", got)
	}
}

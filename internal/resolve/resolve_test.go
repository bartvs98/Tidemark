package resolve

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/registry"
	"github.com/bartvs98/tidemark/internal/version"
)

// fakeRegistry serves tags for one repository; digests maps tag → digest.
type fakeRegistry struct {
	tags    []string
	digests map[string]string
	heads   []string // tags whose digest was requested
	tagsErr error    // returned with the tags
}

func (f *fakeRegistry) Tags(context.Context, imageref.Ref) ([]string, error) {
	return f.tags, f.tagsErr
}

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
		name      string
		c         docker.Container
		reg       *fakeRegistry
		want      Result
		noHeads   bool     // pinned tags must not cost any HEAD requests
		wantHeads []string // when set, the exact HEAD requests in order
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
			want: Result{Running: "8.3.2", Source: SourceDigest, Latest: "9.0.4", LatestInMajor: "8.3.3", Class: version.Major, Floating: true, NewImage: true, TagDigest: "sha256:833"},
		},
		{
			name: "latest tag with a trustworthy label",
			c:    container("homeassistant/home-assistant:latest", "homeassistant/home-assistant@sha256:old", map[string]string{"org.opencontainers.image.version": "2026.9.0"}),
			reg: &fakeRegistry{
				tags:    []string{"2026.9.0", "2026.9.4", "2026.10", "2026.10.0", "2026.10.1", "dev", "latest"},
				digests: map[string]string{"latest": "sha256:new"},
			},
			want: Result{Running: "2026.9.0", Source: SourceLabel, Latest: "2026.10.1", Class: version.Minor, Floating: true, NewImage: true, TagDigest: "sha256:new"},
		},
		{
			name:    "pinned linuxserver tag, label knows the build: packaging update",
			c:       container("lscr.io/linuxserver/thelounge:1.20.0", "lscr.io/linuxserver/thelounge@sha256:a", map[string]string{"org.opencontainers.image.version": "1.20.0-ls166"}),
			reg:     &fakeRegistry{tags: []string{"1.20.0", "1.20.0-ls165", "1.20.0-ls166", "1.20.0-ls168", "latest"}},
			want:    Result{Running: "1.20.0-ls166", Source: SourceLabel, Latest: "1.20.0-ls168", Class: version.Digest},
			noHeads: true,
		},
		{
			// After `docker pull` without recreating, the inventory falls back to the old image's digest.
			name: "digest only: the version comes from a digest match",
			c:    container("redis@sha256:72", "redis@sha256:72", nil),
			reg:  &fakeRegistry{tags: []string{"7.2", "7.4", "latest"}, digests: map[string]string{"7.4": "sha256:74", "7.2": "sha256:72"}},
			want: Result{Running: "7.2", Source: SourceDigest, Latest: "7.4", Class: version.Minor},
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
			want: Result{Running: "16.9-alpine", Source: SourceDigest, Latest: "18.6-alpine", LatestInMajor: "16.10-alpine", Class: version.Major, Floating: true, NewImage: true, TagDigest: "sha256:1610"},
		},
		{
			name: "latest without label: digest match among the most precise tags",
			c:    container("redis:latest", "redis@sha256:8101", nil),
			reg: &fakeRegistry{
				tags:    []string{"8", "8.10", "8.10.1", "8.10.2", "8.10.2-alpine", "latest"},
				digests: map[string]string{"8.10.2": "sha256:8102", "8.10.1": "sha256:8101", "latest": "sha256:8102"},
			},
			want: Result{Running: "8.10.1", Source: SourceDigest, Latest: "8.10.2", Class: version.Patch, Floating: true, NewImage: true, TagDigest: "sha256:8102"},
		},
		{
			name: "variant word tag with label",
			c:    container("woodpeckerci/woodpecker-agent:alpine", "woodpeckerci/woodpecker-agent@sha256:a", map[string]string{"org.opencontainers.image.version": "0.21.0-alpine"}),
			reg: &fakeRegistry{
				tags:    []string{"0.20.0", "0.20.0-alpine", "0.21.0", "0.21.0-alpine", "alpine", "latest"},
				digests: map[string]string{"alpine": "sha256:a"},
			},
			want: Result{Running: "0.21.0-alpine", Source: SourceLabel, Latest: "0.21.0-alpine", Class: version.None, Floating: true, TagDigest: "sha256:a"},
		},
		{
			name: "label matches a v-prefixed tag; latest keeps the v",
			c:    container("filebrowser/filebrowser:latest", "filebrowser/filebrowser@sha256:a", map[string]string{"org.opencontainers.image.version": "v11.3.1"}),
			reg: &fakeRegistry{
				tags:    []string{"v11.3.1", "v11.3.2", "11.3.2", "v11.3.2-alpine", "latest"},
				digests: map[string]string{"latest": "sha256:b"},
			},
			want: Result{Running: "v11.3.1", Source: SourceLabel, Latest: "v11.3.2", Class: version.Patch, Floating: true, NewImage: true, TagDigest: "sha256:b"},
		},
		{
			name: "nightly: unknown version, no digest probing, new image detected",
			c:    container("ghcr.io/mealie-recipes/mealie:nightly", "ghcr.io/mealie-recipes/mealie@sha256:old", nil),
			reg: &fakeRegistry{
				tags:    []string{"nightly", "v2.35.9", "v2.36.0", "latest"},
				digests: map[string]string{"nightly": "sha256:new"},
			},
			want: Result{Source: SourceUnknown, Latest: "v2.36.0", Class: version.Digest, Floating: true, NewImage: true, TagDigest: "sha256:new"},
		},
		{
			name: "unknown and unchanged",
			c:    container("jlesage/mkvtoolnix:latest", "jlesage/mkvtoolnix@sha256:a", nil),
			reg: &fakeRegistry{
				tags:    []string{"4.22.8", "4.23.8", "latest"},
				digests: map[string]string{"latest": "sha256:a"},
			},
			want: Result{Source: SourceUnknown, Latest: "4.23.8", Class: version.None, Floating: true, TagDigest: "sha256:a"},
		},
		{
			name: "latest: an old more precise tag doesn't keep the current one from being probed",
			c:    container("postgres:latest", "postgres@sha256:180", nil),
			reg: &fakeRegistry{
				tags:    []string{"9", "9.6", "9.6.24", "17", "17.6", "18", "18.0", "latest"},
				digests: map[string]string{"18.0": "sha256:180", "18": "sha256:180", "latest": "sha256:180"},
			},
			want:      Result{Running: "18.0", Source: SourceDigest, Latest: "18.0", Class: version.None, Floating: true, TagDigest: "sha256:180"},
			wantHeads: []string{"18.0", "latest"},
		},
		{
			name: "suffix tag: digest match on the same suffix line, latest stays on it",
			c:    container("postgis/postgis:16-3.4", "postgis/postgis@sha256:a", nil),
			reg: &fakeRegistry{
				tags:    []string{"16-3.4", "16.3-3.4", "16.4-3.4", "16-3.5", "16.4-3.5", "16-master"},
				digests: map[string]string{"16.4-3.4": "sha256:a", "16-3.4": "sha256:a"},
			},
			want:      Result{Running: "16.4-3.4", Source: SourceDigest, Latest: "16.4-3.5", Class: version.Patch, Floating: true, TagDigest: "sha256:a"},
			wantHeads: []string{"16.4-3.4", "16-3.4"},
		},
		{
			name: "word tag: a label of another variant is ignored",
			c:    container("team/app:alpine", "team/app@sha256:a", map[string]string{"org.opencontainers.image.version": "1.2.3"}),
			reg: &fakeRegistry{
				tags:    []string{"1.2.3", "1.2.4", "1.2.3-alpine", "alpine", "latest"},
				digests: map[string]string{"alpine": "sha256:a", "1.2.3-alpine": "sha256:old"},
			},
			want: Result{Source: SourceUnknown, Latest: "1.2.3-alpine", Class: version.None, Floating: true, TagDigest: "sha256:a"},
		},
		{
			name: "channel tag: a label never sets the version",
			c:    container("team/app:nightly", "team/app@sha256:old", map[string]string{"org.opencontainers.image.version": "2.36.0"}),
			reg: &fakeRegistry{
				tags:    []string{"2.35.9", "2.36.0", "nightly"},
				digests: map[string]string{"nightly": "sha256:new"},
			},
			want: Result{Source: SourceUnknown, Latest: "2.36.0", Class: version.Digest, Floating: true, NewImage: true, TagDigest: "sha256:new"},
		},
		{
			name: "latest: date tags are another family",
			c:    container("team/app:latest", "team/app@sha256:d", nil),
			reg: &fakeRegistry{
				tags:    []string{"1.2.3", "1.2.4", "2025-01-01", "2025-06-01", "latest"},
				digests: map[string]string{"2025-06-01": "sha256:d", "latest": "sha256:d"},
			},
			want:      Result{Source: SourceUnknown, Latest: "1.2.4", Class: version.None, Floating: true, TagDigest: "sha256:d"},
			wantHeads: []string{"1.2.4", "1.2.3", "latest"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(context.Background(), tt.reg, tt.c, Options{})
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
			if tt.wantHeads != nil && !slices.Equal(tt.reg.heads, tt.wantHeads) {
				t.Errorf("heads = %v, want %v", tt.reg.heads, tt.wantHeads)
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
		{"latest: PG_VERSION names postgres", "postgres:latest", map[string]string{"PG_VERSION": "8.10.2"}, "8.10.2", SourceEnv},
		{"latest: the alias only names postgres", "postgis/postgis:latest", map[string]string{"PG_VERSION": "8.10.2"}, "", SourceUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := container(tt.image, "x@sha256:none", nil)
			c.Image.VersionEnv = tt.env
			got, err := Resolve(context.Background(), &fakeRegistry{tags: tags}, c, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Running != tt.wantRunning || got.Source != tt.wantSource {
				t.Errorf("running = %q via %s, want %q via %s", got.Running, got.Source, tt.wantRunning, tt.wantSource)
			}
		})
	}
}

func TestResolveOptions(t *testing.T) {
	appTags := []string{"1.3.0", "1.4.0", "1.4.2", "1.5.0", "1.6.0-rc1", "2.0.0", "2.1.0", "2.2.0-beta.2", "latest", "nightly"}
	tests := []struct {
		name  string
		image string
		tags  []string
		opts  Options
		want  Result
	}{
		{
			name:  "zero options: newest stable, major jump",
			image: "app:1.4.0", tags: appTags,
			want: Result{Running: "1.4.0", Source: SourceTag, Latest: "2.1.0", LatestInMajor: "1.5.0", Class: version.Major},
		},
		{
			name:  "prerelease: newest pre-release wins",
			image: "app:1.4.0", tags: appTags, opts: Options{Prerelease: true},
			want: Result{Running: "1.4.0", Source: SourceTag, Latest: "2.2.0-beta.2", LatestInMajor: "1.6.0-rc1", Class: version.Major},
		},
		{
			name:  "prerelease: rc ranks below its release",
			image: "app:1.5.0", tags: []string{"1.5.0", "1.5.0-rc1", "1.5.0-rc10", "1.4.9"}, opts: Options{Prerelease: true},
			want: Result{Running: "1.5.0", Source: SourceTag, Latest: "1.5.0", Class: version.None},
		},
		{
			name:  "prerelease: running rc moves to the next rc",
			image: "app:1.5.0-rc2", tags: []string{"1.4.9", "1.5.0-rc1", "1.5.0-rc2", "1.5.0-rc10"}, opts: Options{Prerelease: true},
			want: Result{Running: "1.5.0-rc2", Source: SourceTag, Latest: "1.5.0-rc10", Class: version.Patch},
		},
		{
			name:  "running rc without prerelease: stable releases only, as before",
			image: "app:1.5.0-rc2", tags: []string{"1.4.9", "1.5.0-rc2", "1.5.0-rc10"},
			want: Result{Running: "1.5.0-rc2", Source: SourceTag, Latest: "1.4.9", Class: version.None},
		},
		{
			name:  "tag pattern filters registry tags",
			image: "app:1.4.0", tags: []string{"1.4.0", "1.4.1", "1.5.0-amd64", "20260101", "2.0.0-amd64"},
			opts: Options{TagPattern: regexp.MustCompile(`^\d+\.\d+\.\d+$`)},
			want: Result{Running: "1.4.0", Source: SourceTag, Latest: "1.4.1", Class: version.Patch},
		},
		{
			name:  "running tag outside the pattern is still read",
			image: "app:1.4.0-amd64", tags: []string{"1.4.0-amd64", "1.4.1-amd64", "1.5.0"},
			opts: Options{TagPattern: regexp.MustCompile(`^\d+\.\d+\.\d+$`)},
			want: Result{Running: "1.4.0-amd64", Source: SourceTag, Class: version.None},
		},
		{
			name:  "prerelease and tag pattern together",
			image: "app:1.4.0", tags: []string{"1.4.0", "1.5.0-rc1", "1.5.0-beta1", "1.6.0-nightly"},
			opts: Options{Prerelease: true, TagPattern: regexp.MustCompile(`^[\d.]+(-rc\d+)?$`)},
			want: Result{Running: "1.4.0", Source: SourceTag, Latest: "1.5.0-rc1", Class: version.Minor},
		},
		{
			name:  "pin keeps latest within the prefix",
			image: "app:1.4.0", tags: appTags, opts: Options{Pin: "1.4"},
			want: Result{Running: "1.4.0", Source: SourceTag, Latest: "1.4.2", Class: version.Patch, Held: "2.1.0", HeldBy: HeldByPin},
		},
		{
			name:  "pin to a major",
			image: "app:1.3.0", tags: appTags, opts: Options{Pin: "1"},
			want: Result{Running: "1.3.0", Source: SourceTag, Latest: "1.5.0", Class: version.Minor, Held: "2.1.0", HeldBy: HeldByPin},
		},
		{
			name:  "pin above the running version: offered as an update",
			image: "app:1.3.0", tags: appTags, opts: Options{Pin: "1.4"},
			want: Result{Running: "1.3.0", Source: SourceTag, Latest: "1.4.2", Class: version.Minor, Held: "2.1.0", HeldBy: HeldByPin},
		},
		{
			name:  "pin below the running version: nothing to do",
			image: "app:2.0.0", tags: appTags, opts: Options{Pin: "1.4"},
			want: Result{Running: "2.0.0", Source: SourceTag, Latest: "1.4.2", Class: version.None, Held: "2.1.0", HeldBy: HeldByPin},
		},
		{
			name:  "pin matching no tag",
			image: "app:1.4.0", tags: appTags, opts: Options{Pin: "3"},
			want: Result{Running: "1.4.0", Source: SourceTag, Class: version.None, Held: "2.1.0", HeldBy: HeldByPin},
		},
		{
			name:  "pin already on the newest: nothing held",
			image: "app:2.1.0", tags: appTags, opts: Options{Pin: "2"},
			want: Result{Running: "2.1.0", Source: SourceTag, Latest: "2.1.0", Class: version.None},
		},
		{
			name:  "hold major",
			image: "app:1.4.0", tags: appTags, opts: Options{HoldMajor: true},
			want: Result{Running: "1.4.0", Source: SourceTag, Latest: "1.5.0", Class: version.Minor, Held: "2.1.0", HeldBy: HeldByAllowMajor},
		},
		{
			name:  "hold major with prerelease",
			image: "app:1.4.0", tags: appTags, opts: Options{HoldMajor: true, Prerelease: true},
			want: Result{Running: "1.4.0", Source: SourceTag, Latest: "1.6.0-rc1", Class: version.Minor, Held: "2.2.0-beta.2", HeldBy: HeldByAllowMajor},
		},
		{
			name:  "hold major, partial version tag",
			image: "app:1", tags: appTags, opts: Options{HoldMajor: true},
			want: Result{Running: "1", Source: SourceTag, Latest: "1.5.0", Class: version.Minor, Floating: true, Held: "2.1.0", HeldBy: HeldByAllowMajor},
		},
		{
			name:  "hold major with an unknown running version: no hold",
			image: "app:latest", tags: appTags, opts: Options{HoldMajor: true},
			want: Result{Source: SourceUnknown, Latest: "2.1.0", Class: version.None, Floating: true},
		},
		{
			name:  "pin applies with an unknown running version",
			image: "app:latest", tags: appTags, opts: Options{Pin: "1.4"},
			want: Result{Source: SourceUnknown, Latest: "1.4.2", Class: version.None, Floating: true, Held: "2.1.0", HeldBy: HeldByPin},
		},
		{
			name:  "pin and hold major: both apply, pin named first",
			image: "app:1.3.0", tags: appTags, opts: Options{Pin: "1", HoldMajor: true},
			want: Result{Running: "1.3.0", Source: SourceTag, Latest: "1.5.0", Class: version.Minor, Held: "2.1.0", HeldBy: HeldByPin},
		},
		{
			name:  "pin wider than the hold: the major hold is what keeps back",
			image: "app:1.4.0", tags: []string{"1.4.0", "1.5.0", "11.0.0", "2.0.0"}, opts: Options{Pin: "1", HoldMajor: true},
			want: Result{Running: "1.4.0", Source: SourceTag, Latest: "1.5.0", Class: version.Minor, Held: "11.0.0", HeldBy: HeldByPin},
		},
		{
			name:  "invalid pin is ignored",
			image: "app:1.4.0", tags: appTags, opts: Options{Pin: "x"},
			want: Result{Running: "1.4.0", Source: SourceTag, Latest: "2.1.0", LatestInMajor: "1.5.0", Class: version.Major},
		},
		{
			name:  "numeric suffix: the newest of the suffix line",
			image: "postgis/postgis:16-3.4", tags: []string{"16-3.4", "16-3.5", "17-3.4", "17-3.5"},
			want: Result{Running: "16-3.4", Source: SourceTag, Latest: "17-3.5", LatestInMajor: "16-3.5", Class: version.Major, Floating: true},
		},
		{
			name:  "numeric suffix: stays on the running runtime line",
			image: "nodered/node-red:4.0.2-20", tags: []string{"4.0.2-20", "4.0.5", "4.0.5-18", "4.0.5-20", "4.0.5-22"},
			want: Result{Running: "4.0.2-20", Source: SourceTag, Latest: "4.0.5-20", Class: version.Patch},
		},
		{
			name:  "no suffix: stays on the unsuffixed line",
			image: "nodered/node-red:4.0.2", tags: []string{"4.0.2", "4.0.5", "4.0.5-18", "4.0.5-20", "4.0.5-22"},
			want: Result{Running: "4.0.2", Source: SourceTag, Latest: "4.0.5", Class: version.Patch},
		},
		{
			name:  "numeric suffix: a line that ends doesn't freeze updates",
			image: "app:2.4.1-1", tags: []string{"2.4.1-1", "2.4.3-2"},
			want: Result{Running: "2.4.1-1", Source: SourceTag, Latest: "2.4.3-2", Class: version.Patch},
		},
		{
			name:  "date tag: compared with dates only",
			image: "app:2025-01-01", tags: []string{"1.2.3", "1.2.4", "2025-01-01", "2025-03-01"},
			want: Result{Running: "2025-01-01", Source: SourceTag, Latest: "2025-03-01", Class: version.Minor},
		},
		{
			name:  "version tag: date tags are ignored",
			image: "app:1.2.3", tags: []string{"1.2.3", "1.2.4", "2025-01-01", "2025-03-01"},
			want: Result{Running: "1.2.3", Source: SourceTag, Latest: "1.2.4", Class: version.Patch},
		},
		{
			name:  "calendar version: a new year is minor",
			image: "homeassistant/home-assistant:2024.12.4", tags: []string{"2024.12.4", "2025.1.0"},
			want: Result{Running: "2024.12.4", Source: SourceTag, Latest: "2025.1.0", Class: version.Minor},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(context.Background(), &fakeRegistry{tags: tt.tags}, container(tt.image, "", nil), tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestResolveHoldKeepsDigestUpdates(t *testing.T) {
	c := container("app:1", "app@sha256:old", nil)
	reg := &fakeRegistry{tags: []string{"1", "1.4.0", "2.0.0"}, digests: map[string]string{"1": "sha256:new", "1.4.0": "sha256:new"}}
	got, err := Resolve(context.Background(), reg, c, Options{Pin: "1.4"})
	if err != nil {
		t.Fatal(err)
	}
	want := Result{Running: "1", Source: SourceTag, Latest: "1.4.0", Class: version.Minor, Floating: true, NewImage: true, Held: "2.0.0", HeldBy: HeldByPin, TagDigest: "sha256:new"}
	if got != want {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestLocalDigestsFiltersRepository(t *testing.T) {
	ref, _ := imageref.Parse("redis:latest")
	got := localDigests([]string{"redis@sha256:a", "docker.io/library/redis@sha256:b", "ghcr.io/x/redis@sha256:c", "garbage"}, ref)
	if !slices.Equal(got, []string{"sha256:a", "sha256:b"}) {
		t.Errorf("got %v", got)
	}
}

func TestChannel(t *testing.T) {
	for tag, want := range map[string]string{
		"nightly": "nightly", "Dev": "dev", "edge": "edge", "beta": "beta", "develop": "develop",
		"latest": "", "stable": "", "main": "", "alpine": "", "16-alpine": "", "1.4.2": "", "v2.0.0-rc1": "", "": "",
	} {
		if got := Channel(tag); got != want {
			t.Errorf("Channel(%q) = %q, want %q", tag, got, want)
		}
	}
}

// A rate-limited tag list with an older one cached still resolves, from that list.
func TestResolveStaleTags(t *testing.T) {
	at := time.Date(2026, 10, 18, 6, 0, 0, 0, time.UTC)
	limited := &registry.Error{Kind: registry.KindRateLimit, Msg: "ghcr.io/team/app: registry rate limit reached, try again later"}
	c := container("ghcr.io/team/app:1.0.0", "", nil)

	reg := &fakeRegistry{tags: []string{"1.0.0", "1.1.0"}, tagsErr: &registry.StaleError{At: at, Err: limited}}
	got, err := Resolve(context.Background(), reg, c, Options{})
	want := Result{Running: "1.0.0", Source: SourceTag, Latest: "1.1.0", Class: version.Minor, TagsFrom: at}
	if err != nil || got != want {
		t.Errorf("stale: got %+v, %v\nwant %+v", got, err, want)
	}

	// Without an older list, the failure stands.
	if _, err := Resolve(context.Background(), &fakeRegistry{tagsErr: limited}, c, Options{}); !errors.Is(err, limited) {
		t.Errorf("rate limited: err = %v", err)
	}
	reg = &fakeRegistry{tags: []string{"1.0.0", "1.1.0"}}
	if got, err := Resolve(context.Background(), reg, c, Options{}); err != nil || !got.TagsFrom.IsZero() {
		t.Errorf("fresh: TagsFrom = %v, err = %v", got.TagsFrom, err)
	}
}

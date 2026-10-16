package imageref

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in       string
		want     Ref
		familiar string
	}{
		{"redis", Ref{"docker.io", "library/redis", "latest", ""}, "redis:latest"},
		{"redis:latest", Ref{"docker.io", "library/redis", "latest", ""}, "redis:latest"},
		{"postgres:16-alpine", Ref{"docker.io", "library/postgres", "16-alpine", ""}, "postgres:16-alpine"},
		{"louislam/uptime-kuma", Ref{"docker.io", "louislam/uptime-kuma", "latest", ""}, "louislam/uptime-kuma:latest"},
		{"docker.io/redis", Ref{"docker.io", "library/redis", "latest", ""}, "redis:latest"},
		{"index.docker.io/library/redis:7", Ref{"docker.io", "library/redis", "7", ""}, "redis:7"},
		{"ghcr.io/sergi0g/cup:v3.5.1", Ref{"ghcr.io", "sergi0g/cup", "v3.5.1", ""}, "ghcr.io/sergi0g/cup:v3.5.1"},
		{"lscr.io/linuxserver/heimdall:latest", Ref{"lscr.io", "linuxserver/heimdall", "latest", ""}, "lscr.io/linuxserver/heimdall:latest"},
		{"localhost/app", Ref{"localhost", "app", "latest", ""}, "localhost/app:latest"},
		{"localhost:5000/team/app:1.2", Ref{"localhost:5000", "team/app", "1.2", ""}, "localhost:5000/team/app:1.2"},
		{"registry:5000/app", Ref{"registry:5000", "app", "latest", ""}, "registry:5000/app:latest"},
		{"redis@sha256:abc", Ref{"docker.io", "library/redis", "", "sha256:abc"}, "redis@sha256:abc"},
		{"redis:7@sha256:abc", Ref{"docker.io", "library/redis", "7", "sha256:abc"}, "redis:7@sha256:abc"},
		{"ghcr.io/a/b/c:nightly", Ref{"ghcr.io", "a/b/c", "nightly", ""}, "ghcr.io/a/b/c:nightly"},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Parse(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
		if f := got.Familiar(); f != tt.familiar {
			t.Errorf("Parse(%q).Familiar() = %q, want %q", tt.in, f, tt.familiar)
		}
	}
}

func TestParseString(t *testing.T) {
	r, _ := Parse("redis")
	if s := r.String(); s != "docker.io/library/redis:latest" {
		t.Errorf("String() = %q", s)
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{"", "  ", "redis:", "Redis", "ghcr.io/", "a//b", "redis@abc"} {
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q): expected error", in)
		}
	}
}

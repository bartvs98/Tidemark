package source

import (
	"testing"

	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
	"github.com/bartvs98/tidemark/internal/rules"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		in   string
		want Repo
		ok   bool
	}{
		{"https://github.com/home-assistant/core", Repo{"github.com", "home-assistant/core"}, true},
		{"https://github.com/mealie-recipes/mealie.git", Repo{"github.com", "mealie-recipes/mealie"}, true},
		{"https://github.com/traefik/traefik/tree/master", Repo{"github.com", "traefik/traefik"}, true},
		{"github.com/sergi0g/cup", Repo{"github.com", "sergi0g/cup"}, true},
		{"git@github.com:filebrowser/filebrowser.git", Repo{"github.com", "filebrowser/filebrowser"}, true},
		{"https://www.github.com/a/b", Repo{"github.com", "a/b"}, true},
		{"https://gitlab.com/group/sub/project/-/releases", Repo{"gitlab.com", "group/sub/project"}, true},
		{"https://bitbucket.org/a/b", Repo{}, false},
		{"https://github.com/onlyowner", Repo{}, false},
		{"", Repo{}, false},
	}
	for _, tt := range tests {
		got, ok := ParseURL(tt.in)
		if ok != tt.ok || got != tt.want {
			t.Errorf("ParseURL(%q) = %+v %v, want %+v %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		image       string
		imageSource string
		repoRule    string
		ruleFrom    string
		want        string
		how         How
	}{
		// Real images.
		{"homeassistant/home-assistant:latest", "https://github.com/home-assistant/core", "", "", "github.com/home-assistant/core", HowImage},
		{"lscr.io/linuxserver/heimdall:latest", "https://github.com/linuxserver/docker-heimdall", "", "", "github.com/linuxserver/docker-heimdall", HowImage},
		{"frooodle/s-pdf:0.46.2", "https://github.com/Stirling-Tools/Stirling-PDF", "", "", "github.com/Stirling-Tools/Stirling-PDF", HowImage},
		{"adguard/adguardhome:v0.107.79", "https://github.com/AdguardTeam/AdGuardHome", "", "", "github.com/AdguardTeam/AdGuardHome", HowImage},
		{"vaultwarden/server:1.37.2", "https://github.com/dani-garcia/vaultwarden", "", "", "github.com/dani-garcia/vaultwarden", HowImage},
		{"jlesage/mkvtoolnix:latest", "https://github.com/jlesage/docker-mkvtoolnix", "", "", "github.com/jlesage/docker-mkvtoolnix", HowImage},
		{"jokobsk/netalertx:latest", "https://github.com/jokob-sk/NetAlertX", "", "", "github.com/jokob-sk/NetAlertX", HowImage},
		// Inherited base-image label is rejected; Docker Hub guess instead.
		{"meisnate12/plex-meta-manager:v1.21.0", "https://github.com/linuxserver/docker-baseimage-ubuntu", "", "", "github.com/meisnate12/plex-meta-manager", HowGuess},
		{"ghcr.io/gethomepage/homepage:latest", "", "", "", "github.com/gethomepage/homepage", HowRegistry},
		{"redis:latest", "", "", "", "github.com/redis/redis", HowKnown},
		{"lobaro/restic-backup-docker:2.1.6", "", "", "", "github.com/lobaro/restic-backup-docker", HowGuess},
		{"redis:latest", "", "https://gitlab.com/me/my-redis-fork", rules.FromLabel, "gitlab.com/me/my-redis-fork", HowLabel},
		{"quay.io/team/app:1", "", "github.com/team/app", rules.FromConfig, "github.com/team/app", HowConfig},
		// An unsupported host in the rule falls back to the automatic chain.
		{"redis:latest", "", "https://example.com/redis", rules.FromConfig, "github.com/redis/redis", HowKnown},
		{"quay.io/team/app:1", "", "", "", "", ""},
	}
	for _, tt := range tests {
		ref, _ := imageref.Parse(tt.image)
		c := docker.Container{
			Image: docker.Image{Labels: map[string]string{"org.opencontainers.image.source": tt.imageSource}},
		}
		rs := rules.Default()
		rs.Repo, rs.From = tt.repoRule, map[string]string{}
		if tt.ruleFrom != "" {
			rs.From[rules.KeyRepo] = tt.ruleFrom
		}
		r, how, ok := Resolve(c, ref, rs)
		got := ""
		if ok {
			got = r.Host + "/" + r.Path
		}
		if got != tt.want || how != tt.how {
			t.Errorf("%s: got %q via %q, want %q via %q", tt.image, got, how, tt.want, tt.how)
		}
	}
}

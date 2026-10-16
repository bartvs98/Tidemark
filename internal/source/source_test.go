package source

import (
	"testing"

	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/imageref"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		in   string
		want Repo
		ok   bool
	}{
		{"https://github.com/home-assistant/core", Repo{"github.com", "home-assistant/core"}, true},
		{"https://github.com/mealie-recipes/Mealie.git", Repo{"github.com", "mealie-recipes/Mealie"}, true},
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
		repoLabel   string
		want        string
		how         How
	}{
		// From real images.
		{"homeassistant/home-assistant:latest", "https://github.com/home-assistant/core", "", "github.com/home-assistant/core", HowImage},
		{"lscr.io/linuxserver/heimdall:latest", "https://github.com/linuxserver/docker-heimdall", "", "github.com/linuxserver/docker-heimdall", HowImage},
		{"frooodle/s-pdf:2.6.13", "https://github.com/Stirling PDFRecipes/recipes", "", "github.com/Stirling PDFRecipes/recipes", HowImage},
		{"adguard/adguardhome:v0.107.79", "https://github.com/AdguardTeam/AdGuardHome", "", "github.com/AdguardTeam/AdGuardHome", HowImage},
		{"vaultwarden/server:1.37.2", "https://github.com/dani-garcia/vaultwarden", "", "github.com/dani-garcia/vaultwarden", HowImage},
		{"jlesage/mkvtoolnix:latest", "https://github.com/crazy-max/docker-mkvtoolnix", "", "github.com/crazy-max/docker-mkvtoolnix", HowImage},
		{"jokobsk/netalertx:latest", "https://github.com/jokob-sk/NetAlertX", "", "github.com/jokob-sk/NetAlertX", HowImage},
		// Inherited base-image label is rejected; Docker Hub guess instead.
		{"meisnate12/plex-meta-manager:v4.0.8", "https://github.com/linuxserver/docker-baseimage-ubuntu", "", "github.com/meisnate12/plex-meta-manager", HowGuess},
		{"ghcr.io/gethomepage/homepage:latest", "", "", "github.com/gethomepage/homepage", HowRegistry},
		{"redis:latest", "", "", "github.com/redis/redis", HowKnown},
		{"lobaro/restic-backup-docker:2.1.6", "", "", "github.com/lobaro/restic-backup-docker", HowGuess},
		{"redis:latest", "", "https://gitlab.com/me/my-redis-fork", "gitlab.com/me/my-redis-fork", HowLabel},
		{"quay.io/team/app:1", "", "", "", ""},
	}
	for _, tt := range tests {
		ref, _ := imageref.Parse(tt.image)
		c := docker.Container{
			Labels: map[string]string{LabelRepo: tt.repoLabel},
			Image:  docker.Image{Labels: map[string]string{"org.opencontainers.image.source": tt.imageSource}},
		}
		r, how, ok := Resolve(c, ref)
		got := ""
		if ok {
			got = r.Host + "/" + r.Path
		}
		if got != tt.want || how != tt.how {
			t.Errorf("%s: got %q via %q, want %q via %q", tt.image, got, how, tt.want, tt.how)
		}
	}
}

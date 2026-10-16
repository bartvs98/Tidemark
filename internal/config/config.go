// Package config loads Tidemark's settings from TIDEMARK_* environment variables.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	Addr     string     // TIDEMARK_ADDR, HTTP listen address
	DataDir  string     // TIDEMARK_DATA_DIR, where the SQLite database lives
	LogLevel slog.Level // TIDEMARK_LOG_LEVEL: debug | info | warn | error

	DockerHost     string // TIDEMARK_DOCKER_HOST: unix:///var/run/docker.sock or tcp://host:2375
	DockerSnapshot string // TIDEMARK_DOCKER_SNAPSHOT: read containers from a scripts/snapshot.sh file instead

	Schedule string // TIDEMARK_SCHEDULE: cron spec for update checks, "off" to only check on demand

	GitHubToken string // TIDEMARK_GITHUB_TOKEN: raises GitHub's API limit from 60 to 5000 requests/hour

	Icons    bool   // TIDEMARK_ICONS: "off" disables app icons
	IconsURL string // TIDEMARK_ICONS_URL: selfh.st icons base URL (CDN or a self-hosted mirror)
}

func Load() (Config, error) {
	c := Config{
		Addr:    env("TIDEMARK_ADDR", ":8080"),
		DataDir: env("TIDEMARK_DATA_DIR", "./data"),

		DockerHost:     env("TIDEMARK_DOCKER_HOST", "unix:///var/run/docker.sock"),
		DockerSnapshot: env("TIDEMARK_DOCKER_SNAPSHOT", ""),

		Schedule: env("TIDEMARK_SCHEDULE", "0 */6 * * *"),

		GitHubToken: env("TIDEMARK_GITHUB_TOKEN", ""),

		Icons:    !strings.EqualFold(env("TIDEMARK_ICONS", "on"), "off"),
		IconsURL: env("TIDEMARK_ICONS_URL", "https://cdn.jsdelivr.net/gh/selfhst/icons"),
	}
	if err := c.LogLevel.UnmarshalText([]byte(env("TIDEMARK_LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("TIDEMARK_LOG_LEVEL: %w", err)
	}
	return c, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

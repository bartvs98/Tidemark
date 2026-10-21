// Package config loads Tidemark's settings from TIDEMARK_* environment variables.
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartvs98/tidemark/internal/analysis"
)

type Config struct {
	Addr     string     // TIDEMARK_ADDR, HTTP listen address
	DataDir  string     // TIDEMARK_DATA_DIR, where the SQLite database lives
	LogLevel slog.Level // TIDEMARK_LOG_LEVEL: debug | info | warn | error

	// ConfigFile is TIDEMARK_CONFIG, the YAML file with per-container rules
	// (default <data dir>/config.yaml). It's optional and re-read on every check run.
	ConfigFile string

	DockerHost     string // TIDEMARK_DOCKER_HOST: unix:///var/run/docker.sock or tcp://host:2375
	DockerSnapshot string // TIDEMARK_DOCKER_SNAPSHOT: read containers from a scripts/snapshot.sh file instead

	Schedule string // TIDEMARK_SCHEDULE: cron spec for update checks, "off" to only check on demand

	GitHubToken string // TIDEMARK_GITHUB_TOKEN: raises GitHub's API limit from 60 to 5000 requests/hour

	Icons    bool   // TIDEMARK_ICONS: "off" disables app icons
	IconsURL string // TIDEMARK_ICONS_URL: selfh.st icons base URL (CDN or a self-hosted mirror)

	// Notifications go to Telegram when both the token and the chat ID are set.
	TelegramBotToken string         // TIDEMARK_TELEGRAM_BOT_TOKEN
	TelegramChatID   string         // TIDEMARK_TELEGRAM_CHAT_ID
	NotifyLevel      analysis.Level // TIDEMARK_NOTIFY_LEVEL: all (default, = none) | attention | breaking
	NotifyDigest     string         // TIDEMARK_NOTIFY_DIGEST: cron spec for one digest message; "" = after each check run
	PublicURL        string         // TIDEMARK_PUBLIC_URL: base URL for links in messages, no trailing "/"; "" = no links
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
	c.ConfigFile = env("TIDEMARK_CONFIG", filepath.Join(c.DataDir, "config.yaml"))
	if err := c.LogLevel.UnmarshalText([]byte(env("TIDEMARK_LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("TIDEMARK_LOG_LEVEL: %w", err)
	}
	c.TelegramBotToken = env("TIDEMARK_TELEGRAM_BOT_TOKEN", "")
	c.TelegramChatID = env("TIDEMARK_TELEGRAM_CHAT_ID", "")
	if (c.TelegramBotToken == "") != (c.TelegramChatID == "") {
		return Config{}, fmt.Errorf("TIDEMARK_TELEGRAM_BOT_TOKEN and TIDEMARK_TELEGRAM_CHAT_ID must be set together")
	}
	switch lvl := strings.ToLower(env("TIDEMARK_NOTIFY_LEVEL", "all")); lvl {
	case "all":
		c.NotifyLevel = analysis.None
	case "attention":
		c.NotifyLevel = analysis.Attention
	case "breaking":
		c.NotifyLevel = analysis.Breaking
	default:
		return Config{}, fmt.Errorf("TIDEMARK_NOTIFY_LEVEL: unknown level %q (use all, attention or breaking)", lvl)
	}
	c.NotifyDigest = env("TIDEMARK_NOTIFY_DIGEST", "")
	if pu := env("TIDEMARK_PUBLIC_URL", ""); pu != "" {
		u, err := url.Parse(pu)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Config{}, fmt.Errorf("TIDEMARK_PUBLIC_URL: %q is not an absolute http(s) URL", pu)
		}
		c.PublicURL = strings.TrimRight(pu, "/")
	}
	return c, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

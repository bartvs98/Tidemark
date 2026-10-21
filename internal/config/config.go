// Package config loads Tidemark's settings from TIDEMARK_* environment variables and
// defines the editable settings: their keys, defaults, validation and how env vars,
// stored values and defaults combine.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the startup-only settings plus the editable settings that env vars set.
type Config struct {
	Addr     string     // TIDEMARK_ADDR, HTTP listen address
	DataDir  string     // TIDEMARK_DATA_DIR, where the SQLite database lives
	LogLevel slog.Level // TIDEMARK_LOG_LEVEL: debug | info | warn | error

	// ConfigFile is TIDEMARK_CONFIG, the YAML file with per-container rules
	// (default <data dir>/config.yaml). It's optional and re-read on every check run.
	ConfigFile string

	DockerHost     string // TIDEMARK_DOCKER_HOST: unix:///var/run/docker.sock or tcp://host:2375
	DockerSnapshot string // TIDEMARK_DOCKER_SNAPSHOT: read containers from a scripts/snapshot.sh file instead

	// Editable is TIDEMARK_SETTINGS_EDITABLE=true (true/1/yes, any case): the settings
	// dialog may change the editable settings. Off by default, since there's no login.
	Editable bool

	// Env holds the editable settings set by env vars, normalized. A key is present only
	// when its env var is set; it then wins over a stored value.
	Env map[Key]string
}

func Load() (Config, error) {
	c := Config{
		Addr:    env("TIDEMARK_ADDR", ":8080"),
		DataDir: env("TIDEMARK_DATA_DIR", "./data"),

		DockerHost:     env("TIDEMARK_DOCKER_HOST", "unix:///var/run/docker.sock"),
		DockerSnapshot: env("TIDEMARK_DOCKER_SNAPSHOT", ""),

		Env: map[Key]string{},
	}
	c.ConfigFile = env("TIDEMARK_CONFIG", filepath.Join(c.DataDir, "config.yaml"))
	if err := c.LogLevel.UnmarshalText([]byte(env("TIDEMARK_LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("TIDEMARK_LOG_LEVEL: %w", err)
	}
	switch strings.ToLower(env("TIDEMARK_SETTINGS_EDITABLE", "")) {
	case "true", "1", "yes":
		c.Editable = true
	}
	for _, f := range Fields {
		raw := env(f.Env, "")
		if raw == "" {
			continue
		}
		v, err := Normalize(f.Key, raw)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", f.Env, err)
		}
		c.Env[f.Key] = v
	}
	// Read-only settings can't supply the other half later, so a half pair is a mistake.
	if !c.Editable && (c.Env[KeyTelegramBotToken] == "") != (c.Env[KeyTelegramChatID] == "") {
		return Config{}, fmt.Errorf("TIDEMARK_TELEGRAM_BOT_TOKEN and TIDEMARK_TELEGRAM_CHAT_ID must be set together")
	}
	return c, nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

package api

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"time"
)

// settingsJSON is what the settings dialog shows. It's read-only: everything comes from
// env vars, and secrets are only reported as set or not.
type settingsJSON struct {
	Version string       `json:"version"`
	Source  sourceJSON   `json:"source"`
	GitHub  githubJSON   `json:"github"`
	Checks  scheduleJSON `json:"checks"`
	Rules   rulesFile    `json:"rules"`
	Icons   bool         `json:"icons"`
	// Notifications is absent when notifications are off. It never holds the bot token or chat ID.
	Notifications *notificationsJSON `json:"notifications,omitempty"`
}

type githubJSON struct {
	Token bool `json:"token"`
	// RateLimit is GitHub's current quota; absent when it couldn't be looked up
	// (RateLimitError says why) or release notes are off.
	RateLimit      *rateLimitJSON `json:"rateLimit,omitempty"`
	RateLimitError string         `json:"rateLimitError,omitempty"`
}

type rateLimitJSON struct {
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	ResetAt   time.Time `json:"resetAt"`
}

type scheduleJSON struct {
	Schedule  string     `json:"schedule,omitempty"` // cron spec; absent when scheduled checks are off
	NextRunAt *time.Time `json:"nextRunAt,omitempty"`
}

type rulesFile struct {
	Path    string `json:"path,omitempty"`    // absent without a rules file
	Missing bool   `json:"missing,omitempty"` // nothing at Path (fine: labels only)
	Error   string `json:"error,omitempty"`   // the file didn't load
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	info := s.source.Info()
	out := settingsJSON{
		Version: s.version,
		Source:  sourceJSON{Kind: info.Kind, Location: info.Location, CapturedAt: info.CapturedAt},
		Checks:  scheduleJSON{Schedule: s.schedule},
		Rules:   rulesFile{Path: s.configFile},
		Icons:   s.icons != nil,
	}
	if s.configFile != "" {
		if _, err := os.Stat(s.configFile); errors.Is(err, fs.ErrNotExist) {
			out.Rules.Missing = true
		}
	}
	if s.checks != nil {
		out.Checks.NextRunAt = s.checks.Status().NextRunAt
		out.Rules.Error = s.checks.RulesError()
	}
	if s.notify != nil {
		out.Notifications = toNotificationsJSON(s.notify.Status(r.Context()))
	}
	if s.changelogs != nil {
		out.GitHub.Token = s.changelogs.HasToken()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if rl, err := s.changelogs.RateLimit(ctx); err != nil {
			out.GitHub.RateLimitError = err.Error()
		} else {
			out.GitHub.RateLimit = &rateLimitJSON{Limit: rl.Limit, Remaining: rl.Remaining, ResetAt: rl.ResetAt}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

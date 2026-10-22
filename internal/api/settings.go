package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/bartvs98/tidemark/internal/ai"
	"github.com/bartvs98/tidemark/internal/config"
	"github.com/bartvs98/tidemark/internal/settings"
)

// settingsJSON is what the settings dialog shows. Secrets are only ever reported as set
// or not, never with their value.
type settingsJSON struct {
	Version string       `json:"version"`
	Source  sourceJSON   `json:"source"`
	GitHub  githubJSON   `json:"github"`
	Checks  scheduleJSON `json:"checks"`
	Rules   rulesFile    `json:"rules"`
	Icons   bool         `json:"icons"`
	// Notifications is absent when notifications are off. It never holds the bot token or chat ID.
	Notifications *notificationsJSON `json:"notifications,omitempty"`
	// AI is absent when AI summaries are off. It never holds the API key.
	AI *aiSettingsJSON `json:"ai,omitempty"`

	Editable bool                 `json:"editable"`         // TIDEMARK_SETTINGS_EDITABLE
	Fields   map[string]fieldJSON `json:"fields,omitempty"` // by JSON name; absent without a settings manager
	Warnings []string             `json:"warnings,omitempty"`
}

// fieldJSON is one editable setting. Non-secret fields always carry value (possibly "");
// secret fields only say whether a value is set.
type fieldJSON struct {
	Value  *string `json:"value,omitempty"`
	Secret bool    `json:"secret,omitempty"`
	Set    *bool   `json:"set,omitempty"`
	Source string  `json:"source"` // env | stored | default
	Env    string  `json:"env"`
}

type aiSettingsJSON struct {
	Provider        string `json:"provider"`          // anthropic | openai | gemini
	Model           string `json:"model"`             // effective: the provider's default when none is set
	BaseURL         string `json:"baseUrl,omitempty"` // openai only
	AutoMinReleases int    `json:"autoMinReleases"`   // 0 = only on demand
	// Usage is this month's provider calls; absent when it couldn't be read.
	Usage *aiUsageJSON `json:"usage,omitempty"`
}

type aiUsageJSON struct {
	Since        time.Time `json:"since"` // start of the current month, server local time
	Calls        int       `json:"calls"`
	Failed       int       `json:"failed"`
	Extended     int       `json:"extended"` // calls that extended an earlier summary
	InputTokens  int       `json:"inputTokens"`
	OutputTokens int       `json:"outputTokens"`
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

// maxSettingsBody bounds a PATCH body; the real thing is a few hundred bytes.
const maxSettingsBody = 64 << 10

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.settingsBody(r.Context()))
}

func (s *Server) settingsBody(ctx context.Context) settingsJSON {
	info := s.source.Info()
	out := settingsJSON{
		Version: s.version,
		Source:  sourceJSON{Kind: info.Kind, Location: info.Location, CapturedAt: info.CapturedAt},
		Rules:   rulesFile{Path: s.configFile},
		Icons:   s.icons.Load() != nil,
	}
	if s.configFile != "" {
		if _, err := os.Stat(s.configFile); errors.Is(err, fs.ErrNotExist) {
			out.Rules.Missing = true
		}
	}
	if s.checks != nil {
		st := s.checks.Status()
		out.Checks = scheduleJSON{Schedule: s.checks.Schedule(), NextRunAt: st.NextRunAt}
		out.Rules.Error = s.checks.RulesError()
	}
	if s.notify != nil && s.notify.Enabled() {
		out.Notifications = toNotificationsJSON(s.notify.Status(ctx), out.Checks.NextRunAt)
	}
	if s.ai != nil {
		if p, autoMin := s.ai.Status(); p != nil {
			out.AI = &aiSettingsJSON{Provider: p.Name(), Model: p.Model(), AutoMinReleases: autoMin}
			if p.Name() == "openai" {
				out.AI.BaseURL = ai.OpenAIBaseURL
				if s.settings != nil && s.settings.Runtime().AIBaseURL != "" {
					out.AI.BaseURL = s.settings.Runtime().AIBaseURL
				}
			}
			now := time.Now()
			since := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			if u, err := s.ai.Usage(ctx, since); err != nil {
				s.log.Warn("reading AI usage failed", "err", err)
			} else {
				out.AI.Usage = &aiUsageJSON{Since: since, Calls: u.Calls, Failed: u.Failed, Extended: u.Extended,
					InputTokens: u.InputTokens, OutputTokens: u.OutputTokens}
			}
		}
	}
	if s.changelogs != nil {
		out.GitHub.Token = s.changelogs.HasToken()
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if rl, err := s.changelogs.RateLimit(ctx); err != nil {
			out.GitHub.RateLimitError = err.Error()
		} else {
			out.GitHub.RateLimit = &rateLimitJSON{Limit: rl.Limit, Remaining: rl.Remaining, ResetAt: rl.ResetAt}
		}
	}
	if s.settings != nil {
		out.Editable = s.settings.Editable()
		out.Warnings = s.settings.Warnings()
		out.Fields = map[string]fieldJSON{}
		for _, v := range s.settings.View() {
			f := fieldJSON{Source: string(v.Source), Env: v.Field.Env}
			if v.Field.Secret {
				set := v.Set
				f.Secret, f.Set = true, &set
			} else {
				value := v.Value
				f.Value = &value
			}
			out.Fields[v.Field.JSON] = f
		}
	}
	return out
}

// patchSettings changes editable settings. The body maps JSON field names to a string
// (set) or null (clear, back to the default); "" counts as null.
func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	if s.settings == nil || !s.settings.Editable() {
		writeError(w, http.StatusForbidden, settings.ErrReadOnly.Error())
		return
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxSettingsBody)).Decode(&body); err != nil || body == nil {
		writeError(w, http.StatusBadRequest, "body must be a JSON object of settings")
		return
	}
	patch := settings.Patch{}
	for name, raw := range body {
		f, ok := config.FieldByJSON(name)
		if !ok {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown setting %q", name))
			return
		}
		if string(raw) == "null" {
			patch[f.Key] = nil
			continue
		}
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%s must be a string or null", name))
			return
		}
		patch[f.Key] = &v
	}

	err := s.settings.Update(r.Context(), patch)
	var fe settings.FieldErrors
	switch {
	case errors.As(err, &fe):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "Some settings aren't valid", "fields": fe})
		return
	case errors.Is(err, settings.ErrReadOnly):
		writeError(w, http.StatusForbidden, err.Error())
		return
	case err != nil:
		s.log.Warn("applying settings failed", "err", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.settingsBody(r.Context()))
}

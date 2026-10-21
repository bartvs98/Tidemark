package config

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/robfig/cron/v3"

	"github.com/bartvs98/tidemark/internal/analysis"
)

// Key names an editable setting (also its row in the store's settings table).
type Key string

const (
	KeySchedule         Key = "schedule"
	KeyGitHubToken      Key = "github_token"
	KeyIcons            Key = "icons"
	KeyIconsURL         Key = "icons_url"
	KeyTelegramBotToken Key = "telegram_bot_token"
	KeyTelegramChatID   Key = "telegram_chat_id"
	KeyNotifyLevel      Key = "notify_level"
	KeyNotifyDigest     Key = "notify_digest"
	KeyPublicURL        Key = "public_url"
)

// Field describes an editable setting.
type Field struct {
	Key     Key
	JSON    string // API name, e.g. "githubToken"
	Env     string // e.g. "TIDEMARK_GITHUB_TOKEN"
	Default string // canonical form
	Secret  bool   // never sent back by the API, never logged
}

// Fields is every editable setting, in the order the API lists them.
var Fields = []Field{
	{Key: KeySchedule, JSON: "schedule", Env: "TIDEMARK_SCHEDULE", Default: "0 */6 * * *"},
	{Key: KeyGitHubToken, JSON: "githubToken", Env: "TIDEMARK_GITHUB_TOKEN", Secret: true},
	{Key: KeyIcons, JSON: "icons", Env: "TIDEMARK_ICONS", Default: "on"},
	{Key: KeyIconsURL, JSON: "iconsUrl", Env: "TIDEMARK_ICONS_URL", Default: "https://cdn.jsdelivr.net/gh/selfhst/icons"},
	{Key: KeyTelegramBotToken, JSON: "telegramBotToken", Env: "TIDEMARK_TELEGRAM_BOT_TOKEN", Secret: true},
	{Key: KeyTelegramChatID, JSON: "telegramChatId", Env: "TIDEMARK_TELEGRAM_CHAT_ID"},
	{Key: KeyNotifyLevel, JSON: "notifyLevel", Env: "TIDEMARK_NOTIFY_LEVEL", Default: "all"},
	{Key: KeyNotifyDigest, JSON: "notifyDigest", Env: "TIDEMARK_NOTIFY_DIGEST"},
	{Key: KeyPublicURL, JSON: "publicUrl", Env: "TIDEMARK_PUBLIC_URL"},
}

// FieldByJSON finds a field by its API name.
func FieldByJSON(name string) (Field, bool) {
	for _, f := range Fields {
		if f.JSON == name {
			return f, true
		}
	}
	return Field{}, false
}

// FieldByKey finds a field by its key.
func FieldByKey(k Key) (Field, bool) {
	for _, f := range Fields {
		if f.Key == k {
			return f, true
		}
	}
	return Field{}, false
}

var reChatID = regexp.MustCompile(`^(-?\d+|@[A-Za-z0-9_]{5,})$`)

// Normalize validates a raw value for k and returns its canonical form ("" = unset).
// Env vars and the settings dialog share it, so both accept exactly the same values.
// Messages never quote a secret value.
func Normalize(k Key, raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	switch k {
	case KeySchedule:
		if strings.EqualFold(v, "off") {
			return "off", nil
		}
		return v, validCron(v)
	case KeyNotifyDigest:
		return v, validCron(v)
	case KeyGitHubToken, KeyTelegramBotToken:
		if strings.ContainsFunc(v, unicode.IsSpace) {
			return "", errors.New("must not contain spaces")
		}
		return v, nil
	case KeyIcons:
		if strings.EqualFold(v, "off") {
			return "off", nil
		}
		return "on", nil
	case KeyIconsURL, KeyPublicURL:
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "", errors.New("not an absolute http(s) URL")
		}
		return strings.TrimRight(v, "/"), nil
	case KeyTelegramChatID:
		if !reChatID.MatchString(v) {
			return "", errors.New("must be a number (a user or group ID) or @channelname")
		}
		return v, nil
	case KeyNotifyLevel:
		lvl := strings.ToLower(v)
		switch lvl {
		case "all", "attention", "breaking":
			return lvl, nil
		}
		return "", fmt.Errorf("unknown level %q (use all, attention or breaking)", lvl)
	}
	return "", fmt.Errorf("unknown setting %q", k)
}

// validCron uses the parser the services' cron.New() uses, so what passes here schedules.
func validCron(spec string) error {
	if _, err := cron.ParseStandard(spec); err != nil {
		return fmt.Errorf("invalid cron spec: %w", err)
	}
	return nil
}

// Runtime is the effective value of every editable setting.
type Runtime struct {
	Schedule         string // cron spec; "" = scheduled checks off
	GitHubToken      string
	Icons            bool
	IconsURL         string
	TelegramBotToken string
	TelegramChatID   string
	NotifyLevel      analysis.Level // None = all
	NotifyDigest     string         // "" = after each check run
	PublicURL        string
}

// Telegram reports whether both bot token and chat ID are set (notifications on).
func (r Runtime) Telegram() bool { return r.TelegramBotToken != "" && r.TelegramChatID != "" }

// Source says where a setting's effective value comes from.
type Source string

const (
	SourceEnv     Source = "env"
	SourceStored  Source = "stored"
	SourceDefault Source = "default"
)

// Resolve merges env (wins), stored, then defaults. Values in env must already be normalized
// (Load does that). An invalid stored value is skipped (falls back to the default) and
// reported in the returned warnings; it never fails. A half Telegram pair (one of token/chat
// ID effective) leaves notifications off and adds a warning.
// values holds the effective canonical string per key, secrets included: callers must not
// expose them.
func Resolve(env, stored map[Key]string) (rt Runtime, sources map[Key]Source, values map[Key]string, warnings []string) {
	sources = map[Key]Source{}
	values = map[Key]string{}
	for _, f := range Fields {
		if v, ok := env[f.Key]; ok && v != "" {
			values[f.Key], sources[f.Key] = v, SourceEnv
			continue
		}
		if raw, ok := stored[f.Key]; ok && raw != "" {
			v, err := Normalize(f.Key, raw)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("stored %s: %v", f.Key, err))
			} else if v != "" {
				values[f.Key], sources[f.Key] = v, SourceStored
				continue
			}
		}
		values[f.Key], sources[f.Key] = f.Default, SourceDefault
	}

	rt = Runtime{
		Schedule:         values[KeySchedule],
		GitHubToken:      values[KeyGitHubToken],
		Icons:            values[KeyIcons] != "off",
		IconsURL:         values[KeyIconsURL],
		TelegramBotToken: values[KeyTelegramBotToken],
		TelegramChatID:   values[KeyTelegramChatID],
		NotifyDigest:     values[KeyNotifyDigest],
		PublicURL:        values[KeyPublicURL],
	}
	if rt.Schedule == "off" {
		rt.Schedule = ""
	}
	_ = rt.NotifyLevel.UnmarshalText([]byte(values[KeyNotifyLevel])) // "all" stays None
	switch {
	case rt.TelegramBotToken != "" && rt.TelegramChatID == "":
		warnings = append(warnings, "Telegram notifications are off: the bot token is set but the chat ID isn't")
	case rt.TelegramBotToken == "" && rt.TelegramChatID != "":
		warnings = append(warnings, "Telegram notifications are off: the chat ID is set but the bot token isn't")
	}
	return rt, sources, values, warnings
}

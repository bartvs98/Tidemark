package config

import (
	"strings"
	"testing"

	"github.com/bartvs98/tidemark/internal/analysis"
)

func TestLoadDefaults(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Editable || len(c.Env) != 0 || c.Addr != ":8080" {
		t.Errorf("config = %+v", c)
	}
	rt, _, _, warnings := Resolve(c.Env, nil)
	if rt.Schedule != "0 */6 * * *" || !rt.Icons || rt.IconsURL != "https://cdn.jsdelivr.net/gh/selfhst/icons" ||
		rt.Telegram() || rt.NotifyLevel != analysis.None || rt.NotifyDigest != "" || rt.PublicURL != "" || len(warnings) != 0 {
		t.Errorf("runtime = %+v, warnings %q", rt, warnings)
	}
}

func TestLoadEnv(t *testing.T) {
	t.Setenv("TIDEMARK_TELEGRAM_BOT_TOKEN", "123:abc")
	t.Setenv("TIDEMARK_TELEGRAM_CHAT_ID", "-100200")
	t.Setenv("TIDEMARK_NOTIFY_LEVEL", "Breaking")
	t.Setenv("TIDEMARK_NOTIFY_DIGEST", "CRON_TZ=Europe/Amsterdam 0 9 * * *")
	t.Setenv("TIDEMARK_PUBLIC_URL", "https://tidemark.example.lan/")
	t.Setenv("TIDEMARK_SCHEDULE", "OFF")
	t.Setenv("TIDEMARK_ICONS", "Off")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := map[Key]string{
		KeyTelegramBotToken: "123:abc", KeyTelegramChatID: "-100200", KeyNotifyLevel: "breaking",
		KeyNotifyDigest: "CRON_TZ=Europe/Amsterdam 0 9 * * *", KeyPublicURL: "https://tidemark.example.lan",
		KeySchedule: "off", KeyIcons: "off",
	}
	if len(c.Env) != len(want) {
		t.Errorf("env = %v", c.Env)
	}
	for k, v := range want {
		if c.Env[k] != v {
			t.Errorf("%s = %q, want %q", k, c.Env[k], v)
		}
	}
	rt, _, _, _ := Resolve(c.Env, nil)
	if !rt.Telegram() || rt.NotifyLevel != analysis.Breaking || rt.Schedule != "" || rt.Icons {
		t.Errorf("runtime = %+v", rt)
	}
}

func TestLoadEditable(t *testing.T) {
	for in, want := range map[string]bool{"": false, "true": true, "TRUE": true, "1": true, "yes": true, "Yes": true, "false": false, "0": false, "no": false} {
		t.Setenv("TIDEMARK_SETTINGS_EDITABLE", in)
		c, err := Load()
		if err != nil || c.Editable != want {
			t.Errorf("%q: editable %v, err %v", in, c.Editable, err)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"unknown level", map[string]string{"TIDEMARK_NOTIFY_LEVEL": "loud"}, "TIDEMARK_NOTIFY_LEVEL: unknown level"},
		{"token without chat", map[string]string{"TIDEMARK_TELEGRAM_BOT_TOKEN": "123:abc"}, "set together"},
		{"chat without token", map[string]string{"TIDEMARK_TELEGRAM_CHAT_ID": "42"}, "set together"},
		{"relative public url", map[string]string{"TIDEMARK_PUBLIC_URL": "tidemark.lan"}, "TIDEMARK_PUBLIC_URL"},
		{"ftp public url", map[string]string{"TIDEMARK_PUBLIC_URL": "ftp://tidemark.lan"}, "TIDEMARK_PUBLIC_URL"},
		{"bad schedule", map[string]string{"TIDEMARK_SCHEDULE": "every so often"}, "TIDEMARK_SCHEDULE: invalid cron spec"},
		// The digest and chat ID are checked even without the other Telegram settings.
		{"bad digest", map[string]string{"TIDEMARK_NOTIFY_DIGEST": "off"}, "TIDEMARK_NOTIFY_DIGEST: invalid cron spec"},
		{"bad chat id", map[string]string{"TIDEMARK_TELEGRAM_BOT_TOKEN": "123:abc", "TIDEMARK_TELEGRAM_CHAT_ID": "my chat"}, "TIDEMARK_TELEGRAM_CHAT_ID"},
		{"token with space", map[string]string{"TIDEMARK_GITHUB_TOKEN": "ghp_a 123:abc"}, "TIDEMARK_GITHUB_TOKEN: must not contain spaces"},
		{"bad icons url", map[string]string{"TIDEMARK_ICONS_URL": "cdn.example"}, "TIDEMARK_ICONS_URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "123:abc") {
				t.Errorf("error leaks the token: %v", err)
			}
		})
	}
}

// With editable settings the other half of the Telegram pair may come from the dialog.
func TestLoadHalfPairEditable(t *testing.T) {
	t.Setenv("TIDEMARK_SETTINGS_EDITABLE", "true")
	t.Setenv("TIDEMARK_TELEGRAM_BOT_TOKEN", "123:abc")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Env[KeyTelegramBotToken] != "123:abc" {
		t.Errorf("env = %v", c.Env)
	}
}

func TestNormalize(t *testing.T) {
	ok := []struct {
		key       Key
		raw, want string
	}{
		{KeySchedule, "", ""},
		{KeySchedule, "  ", ""},
		{KeySchedule, "Off", "off"},
		{KeySchedule, "0 */6 * * *", "0 */6 * * *"},
		{KeySchedule, "@daily", "@daily"},
		{KeySchedule, "@every 3h", "@every 3h"},
		{KeySchedule, " CRON_TZ=Europe/Amsterdam 0 9 * * * ", "CRON_TZ=Europe/Amsterdam 0 9 * * *"},
		{KeyGitHubToken, "ghp_abc", "ghp_abc"},
		{KeyIcons, "OFF", "off"},
		{KeyIcons, "on", "on"},
		{KeyIcons, "whatever", "on"},
		{KeyIconsURL, "https://icons.lan/selfhst/", "https://icons.lan/selfhst"},
		{KeyTelegramBotToken, "123:abc", "123:abc"},
		{KeyTelegramChatID, "-1001234", "-1001234"},
		{KeyTelegramChatID, "42", "42"},
		{KeyTelegramChatID, "@my_channel", "@my_channel"},
		{KeyNotifyLevel, "ATTENTION", "attention"},
		{KeyNotifyLevel, "all", "all"},
		{KeyNotifyDigest, "0 9 * * 1", "0 9 * * 1"},
		{KeyPublicURL, "http://tidemark.lan:8080/", "http://tidemark.lan:8080"},
		{KeyAIProvider, "Anthropic", "anthropic"},
		{KeyAIProvider, "OFF", "off"},
		{KeyAIProvider, "gemini", "gemini"},
		{KeyAIAPIKey, "sk-ant-abc", "sk-ant-abc"},
		{KeyAIModel, "llama3.2:3b", "llama3.2:3b"},
		{KeyAIBaseURL, "http://ollama:11434/v1/", "http://ollama:11434/v1"},
		{KeyAIAutoMin, "0", "0"},
		{KeyAIAutoMin, "007", "7"},
		{KeyAIAutoMin, "500", "500"},
	}
	for _, tt := range ok {
		got, err := Normalize(tt.key, tt.raw)
		if err != nil || got != tt.want {
			t.Errorf("Normalize(%s, %q) = %q, %v; want %q", tt.key, tt.raw, got, err, tt.want)
		}
	}

	bad := []struct {
		key       Key
		raw, want string
	}{
		{KeySchedule, "every so often", "invalid cron spec: "},
		{KeySchedule, "0 0 */6 * * *", "invalid cron spec: "}, // seconds field: not the standard parser
		{KeyNotifyDigest, "off", "invalid cron spec: "},
		{KeyGitHubToken, "ghp a", "must not contain spaces"},
		{KeyTelegramBotToken, "123:a\tb", "must not contain spaces"},
		{KeyIconsURL, "/icons", "not an absolute http(s) URL"},
		{KeyPublicURL, "ftp://x.lan", "not an absolute http(s) URL"},
		{KeyPublicURL, "https://", "not an absolute http(s) URL"},
		{KeyTelegramChatID, "12a", "must be a number (a user or group ID) or @channelname"},
		{KeyTelegramChatID, "@abc", "must be a number"},
		{KeyNotifyLevel, "loud", `unknown level "loud" (use all, attention or breaking)`},
		{KeyAIProvider, "Claude", `unknown provider "claude" (use off, anthropic, openai or gemini)`},
		{KeyAIAPIKey, "sk a", "must not contain spaces"},
		{KeyAIModel, "gpt 5", "must not contain spaces"},
		{KeyAIModel, strings.Repeat("m", 201), "too long"},
		{KeyAIBaseURL, "ollama:11434", "not an absolute http(s) URL"},
		{KeyAIAutoMin, "501", "must be a whole number from 0 to 500"},
		{KeyAIAutoMin, "-1", "must be a whole number"},
		{KeyAIAutoMin, "3.5", "must be a whole number"},
		{Key("nope"), "x", "unknown setting"},
	}
	for _, tt := range bad {
		_, err := Normalize(tt.key, tt.raw)
		if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
			t.Errorf("Normalize(%s, %q) err = %v, want %q", tt.key, tt.raw, err, tt.want)
		}
		if err != nil && strings.Contains(err.Error(), "TIDEMARK_") {
			t.Errorf("message names the env var: %v", err)
		}
	}
}

func TestFields(t *testing.T) {
	if len(Fields) != 14 {
		t.Fatalf("%d fields", len(Fields))
	}
	for _, f := range Fields {
		if got, ok := FieldByJSON(f.JSON); !ok || got.Key != f.Key {
			t.Errorf("FieldByJSON(%q) = %v, %v", f.JSON, got, ok)
		}
		if got, err := Normalize(f.Key, f.Default); err != nil || got != f.Default {
			t.Errorf("default of %s isn't canonical: %q, %v", f.Key, got, err)
		}
	}
	if _, ok := FieldByJSON("schedule_x"); ok {
		t.Error("unknown name found")
	}
}

func TestResolvePrecedence(t *testing.T) {
	env := map[Key]string{KeySchedule: "@daily"}
	stored := map[Key]string{KeySchedule: "@hourly", KeyIconsURL: "https://icons.lan", KeyNotifyLevel: "attention"}
	rt, sources, values, warnings := Resolve(env, stored)
	if rt.Schedule != "@daily" || sources[KeySchedule] != SourceEnv {
		t.Errorf("schedule = %q from %s", rt.Schedule, sources[KeySchedule])
	}
	if rt.IconsURL != "https://icons.lan" || sources[KeyIconsURL] != SourceStored {
		t.Errorf("icons url = %q from %s", rt.IconsURL, sources[KeyIconsURL])
	}
	if rt.NotifyLevel != analysis.Attention || values[KeyNotifyLevel] != "attention" {
		t.Errorf("level = %v", rt.NotifyLevel)
	}
	if sources[KeyPublicURL] != SourceDefault || values[KeyIcons] != "on" || sources[KeyIcons] != SourceDefault {
		t.Errorf("sources = %v, values = %v", sources, values)
	}
	if len(sources) != len(Fields) || len(values) != len(Fields) || len(warnings) != 0 {
		t.Errorf("sources %v, values %v, warnings %q", sources, values, warnings)
	}
}

func TestResolveInvalidStored(t *testing.T) {
	rt, sources, _, warnings := Resolve(nil, map[Key]string{KeySchedule: "every so often", KeyGitHubToken: "secret token"})
	if rt.Schedule != "0 */6 * * *" || sources[KeySchedule] != SourceDefault || rt.GitHubToken != "" {
		t.Errorf("runtime = %+v, sources %v", rt, sources)
	}
	if len(warnings) != 2 || !strings.HasPrefix(warnings[0], "stored schedule: invalid cron spec: ") ||
		warnings[1] != "stored github_token: must not contain spaces" {
		t.Errorf("warnings = %q", warnings)
	}
}

func TestResolveHalfTelegramPair(t *testing.T) {
	rt, _, _, warnings := Resolve(map[Key]string{KeyTelegramBotToken: "123:abc"}, nil)
	if rt.Telegram() || len(warnings) != 1 || !strings.Contains(warnings[0], "chat ID isn't") {
		t.Errorf("telegram %v, warnings %q", rt.Telegram(), warnings)
	}
	rt, _, _, warnings = Resolve(nil, map[Key]string{KeyTelegramChatID: "42"})
	if rt.Telegram() || len(warnings) != 1 || !strings.Contains(warnings[0], "bot token isn't") {
		t.Errorf("telegram %v, warnings %q", rt.Telegram(), warnings)
	}
	// One half from env, the other stored: on.
	rt, _, _, warnings = Resolve(map[Key]string{KeyTelegramBotToken: "123:abc"}, map[Key]string{KeyTelegramChatID: "42"})
	if !rt.Telegram() || len(warnings) != 0 {
		t.Errorf("telegram %v, warnings %q", rt.Telegram(), warnings)
	}
}

func TestResolveAI(t *testing.T) {
	rt, _, _, warnings := Resolve(nil, nil)
	if rt.AIProvider != "" || rt.AI() || rt.AIAutoMin != 0 || len(warnings) != 0 {
		t.Errorf("default: %+v, warnings %q", rt, warnings)
	}
	rt, _, _, warnings = Resolve(map[Key]string{KeyAIProvider: "anthropic"}, map[Key]string{KeyAIAPIKey: "sk-x", KeyAIAutoMin: "5"})
	if !rt.AI() || rt.AIProvider != "anthropic" || rt.AIAPIKey != "sk-x" || rt.AIAutoMin != 5 || len(warnings) != 0 {
		t.Errorf("anthropic: %+v, warnings %q", rt, warnings)
	}
	rt, _, _, warnings = Resolve(nil, map[Key]string{KeyAIProvider: "gemini"})
	if rt.AI() || len(warnings) != 1 || warnings[0] != "AI summaries are off: the provider is set but the API key isn't" {
		t.Errorf("no key: %v, warnings %q", rt.AI(), warnings)
	}
	// A local OpenAI-compatible server needs no key.
	rt, _, _, warnings = Resolve(nil, map[Key]string{KeyAIProvider: "openai", KeyAIBaseURL: "http://ollama:11434/v1"})
	if !rt.AI() || len(warnings) != 0 {
		t.Errorf("ollama: %v, warnings %q", rt.AI(), warnings)
	}
	rt, _, _, warnings = Resolve(nil, map[Key]string{KeyAIProvider: "anthropic", KeyAIAPIKey: "sk-x", KeyAIBaseURL: "http://x.lan"})
	if !rt.AI() || len(warnings) != 1 || warnings[0] != "the AI base URL is only used with the openai provider" {
		t.Errorf("base url: %v, warnings %q", rt.AI(), warnings)
	}
	if strings.Contains(strings.Join(warnings, " "), "sk-x") {
		t.Error("warning shows the key")
	}
}

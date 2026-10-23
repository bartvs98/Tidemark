package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bartvs98/tidemark/internal/ai"
	"github.com/bartvs98/tidemark/internal/changelog"
	"github.com/bartvs98/tidemark/internal/check"
	"github.com/bartvs98/tidemark/internal/config"
	"github.com/bartvs98/tidemark/internal/docker"
	"github.com/bartvs98/tidemark/internal/settings"
	"github.com/bartvs98/tidemark/internal/store"
)

func TestSettings(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	src := docker.NewSnapshot("../docker/testdata/snapshot.json")
	checks := check.New(src, tagsRegistry{}, nil, log)
	if err := checks.SetSchedule(t.Context(), "0 */6 * * *"); err != nil {
		t.Fatal(err)
	}
	h := New(Deps{
		Version:    "sha-abc1234",
		Log:        log,
		Source:     src,
		Checks:     checks,
		ConfigFile: "/data/config.yaml",
	}).Handler(builtSPA)

	rec := do(t, h, "/api/settings")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var body settingsJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Version != "sha-abc1234" || body.Source.Kind != "snapshot" || body.Checks.Schedule != "0 */6 * * *" || body.Checks.NextRunAt == nil ||
		body.Rules.Path != "/data/config.yaml" || !body.Rules.Missing || body.Icons || body.GitHub.Token || body.GitHub.RateLimit != nil {
		t.Errorf("settings = %+v", body)
	}
	// Without a settings manager: read-only, no per-field state.
	if body.Editable || body.Fields != nil || body.Warnings != nil {
		t.Errorf("settings = %+v", body)
	}
}

func TestSettingsGitHub(t *testing.T) {
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"resources":{"core":{"limit":5000,"remaining":4812,"reset":1792351465}}}`)
	}))
	defer github.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(Deps{
		Log:        log,
		Source:     docker.NewSnapshot("../docker/testdata/snapshot.json"),
		Changelogs: changelog.New(nil, "ghp_topsecret", log, changelog.WithBaseURLs(github.URL, github.URL, github.URL)),
	}).Handler(builtSPA)

	rec := do(t, h, "/api/settings")
	var body settingsJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.GitHub.Token || body.GitHub.RateLimit == nil || body.GitHub.RateLimit.Remaining != 4812 || body.Checks.Schedule != "" {
		t.Errorf("settings = %+v", body)
	}
	// Secrets are only ever reported as set or not.
	if strings.Contains(rec.Body.String(), "topsecret") {
		t.Errorf("body contains the token: %s", rec.Body.String())
	}
}

const (
	envGitHubToken    = "ghp_ENV-secret-token"
	storedBotToken    = "999:STORED-secret-bot"
	patchedGitHubTok  = "ghp_PATCHED-secret"
	patchedBotToken   = "777:PATCHED-secret-bot"
	storedAIKey       = "sk-ant-STORED-secret-key"
	patchedAIKey      = "sk-PATCHED-secret-key"
	storedChatID      = "-100555"
	invalidStoredCron = "every so often"
)

type settingsFixture struct {
	h      http.Handler
	db     *store.Store
	checks *check.Service
	m      *settings.Manager
}

// newSettingsFixture serves settings with a manager: the GitHub token comes from an env
// var, the Telegram pair, AI (anthropic + key) and an invalid schedule are stored.
func newSettingsFixture(t *testing.T, editable bool) settingsFixture {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.SaveSettings(t.Context(), map[string]string{
		"telegram_bot_token": storedBotToken, "telegram_chat_id": storedChatID, "schedule": invalidStoredCron,
		"ai_provider": "anthropic", "ai_api_key": storedAIKey, "ai_auto_min_releases": "4",
	}, nil); err != nil {
		t.Fatal(err)
	}
	src := docker.NewSnapshot("../docker/testdata/snapshot.json")
	checks := check.New(src, tagsRegistry{}, db, log)
	env := map[config.Key]string{config.KeyGitHubToken: envGitHubToken}
	aiSvc := ai.New(db, log)
	m := settings.New(db, env, editable, settings.Appliers{
		Schedule: func(spec string) error { return checks.SetSchedule(t.Context(), spec) },
		AI: func(rt config.Runtime) error {
			if !rt.AI() {
				aiSvc.Configure(nil, 0)
				return nil
			}
			p, err := ai.NewProvider(rt.AIProvider, rt.AIAPIKey, rt.AIModel, rt.AIBaseURL, rt.AIEffort)
			if err != nil {
				return err
			}
			aiSvc.Configure(p, rt.AIAutoMin)
			return nil
		},
	}, log)
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	srv := New(Deps{Version: "t", Log: log, Source: src, Checks: checks, Store: db, Settings: m, AI: aiSvc})
	return settingsFixture{h: srv.Handler(fstest.MapFS{}), db: db, checks: checks, m: m}
}

func noSecrets(t *testing.T, body string) {
	t.Helper()
	for _, secret := range []string{envGitHubToken, storedBotToken, patchedGitHubTok, patchedBotToken, storedAIKey, patchedAIKey} {
		if strings.Contains(body, secret) {
			t.Errorf("body contains %q: %s", secret, body)
		}
	}
}

func TestSettingsFields(t *testing.T) {
	f := newSettingsFixture(t, false)
	now := time.Now()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	for _, c := range []store.AICall{
		{At: month.Add(-time.Second), InputTokens: 999, OutputTokens: 999}, // last month
		{At: now, Trigger: "manual", InputTokens: 1200, OutputTokens: 300},
		{At: now, Trigger: "auto", InputTokens: 50, Error: "rate limited by anthropic"},
	} {
		if err := f.db.AddAICall(t.Context(), c); err != nil {
			t.Fatal(err)
		}
	}
	rec := do(t, f.h, "/api/settings")
	noSecrets(t, rec.Body.String())
	var body struct {
		Editable bool                       `json:"editable"`
		Fields   map[string]json.RawMessage `json:"fields"`
		Warnings []string                   `json:"warnings"`
		Checks   scheduleJSON               `json:"checks"`
		AI       *aiSettingsJSON            `json:"ai"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Editable || len(body.Fields) != len(config.Fields) {
		t.Fatalf("body = %s", rec.Body)
	}
	want := map[string]string{
		"schedule":          `{"value":"0 */6 * * *","source":"default","env":"TIDEMARK_SCHEDULE"}`,
		"githubToken":       `{"secret":true,"set":true,"source":"env","env":"TIDEMARK_GITHUB_TOKEN"}`,
		"telegramBotToken":  `{"secret":true,"set":true,"source":"stored","env":"TIDEMARK_TELEGRAM_BOT_TOKEN"}`,
		"telegramChatId":    `{"value":"-100555","source":"stored","env":"TIDEMARK_TELEGRAM_CHAT_ID"}`,
		"notifyDigest":      `{"value":"","source":"default","env":"TIDEMARK_NOTIFY_DIGEST"}`,
		"icons":             `{"value":"on","source":"default","env":"TIDEMARK_ICONS"}`,
		"aiProvider":        `{"value":"anthropic","source":"stored","env":"TIDEMARK_AI_PROVIDER"}`,
		"aiApiKey":          `{"secret":true,"set":true,"source":"stored","env":"TIDEMARK_AI_API_KEY"}`,
		"aiModel":           `{"value":"","source":"default","env":"TIDEMARK_AI_MODEL"}`,
		"aiAutoMinReleases": `{"value":"4","source":"stored","env":"TIDEMARK_AI_AUTO_MIN_RELEASES"}`,
	}
	for name, w := range want {
		if got := string(body.Fields[name]); got != w {
			t.Errorf("%s = %s, want %s", name, got, w)
		}
	}
	if len(body.Warnings) != 1 || !strings.HasPrefix(body.Warnings[0], "stored schedule: invalid cron spec") {
		t.Errorf("warnings = %q", body.Warnings)
	}
	if body.Checks.Schedule != "0 */6 * * *" {
		t.Errorf("checks = %+v", body.Checks)
	}
	if body.AI == nil || body.AI.Usage == nil {
		t.Fatalf("ai = %+v", body.AI)
	}
	if u := *body.AI.Usage; !u.Since.Equal(month) || u.Calls != 2 || u.Failed != 1 || u.InputTokens != 1250 || u.OutputTokens != 300 {
		t.Errorf("usage = %+v, want since %v", u, month)
	}
	if !strings.Contains(rec.Body.String(), `"usage":{"since":"`+month.Format(time.RFC3339)+`","calls":2,"failed":1,"extended":0,"inputTokens":1250,"outputTokens":300}`) {
		t.Errorf("usage JSON: %s", rec.Body)
	}
	if a := withoutUsage(*body.AI); a != (aiSettingsJSON{Provider: "anthropic", Model: ai.DefaultAnthropicModel, Effort: "low", AutoMinReleases: 4}) {
		t.Errorf("ai = %+v", body.AI)
	}
}

func TestPatchSettingsReadOnly(t *testing.T) {
	f := newSettingsFixture(t, false)
	rec := send(t, f.h, http.MethodPatch, "/api/settings", `{"schedule":"@daily"}`)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "TIDEMARK_SETTINGS_EDITABLE=true") {
		t.Errorf("got %d %s", rec.Code, rec.Body)
	}
	// Without a manager at all, too.
	h := New(Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Source: docker.NewSnapshot("../docker/testdata/snapshot.json")}).Handler(fstest.MapFS{})
	if rec := send(t, h, http.MethodPatch, "/api/settings", `{}`); rec.Code != http.StatusForbidden {
		t.Errorf("no manager: %d", rec.Code)
	}
}

func TestPatchSettingsNeedsHeader(t *testing.T) {
	f := newSettingsFixture(t, true)
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/settings", strings.NewReader(`{"schedule":"@daily"}`)))
	if rec.Code != http.StatusForbidden || f.checks.Schedule() == "@daily" {
		t.Errorf("status = %d, schedule %q", rec.Code, f.checks.Schedule())
	}
}

func TestPatchSettingsBadRequest(t *testing.T) {
	f := newSettingsFixture(t, true)
	for _, body := range []string{
		`not json`,
		`["schedule"]`,
		`null`,
		`{"nope":"x"}`,
		`{"schedule":5}`,
		`{"icons":true}`,
		`{"schedule":`,
		`{"schedule":"` + strings.Repeat("x", maxSettingsBody) + `"}`,
	} {
		if rec := send(t, f.h, http.MethodPatch, "/api/settings", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%.40s: got %d %s", body, rec.Code, rec.Body)
		}
	}
}

func TestPatchSettingsInvalid(t *testing.T) {
	f := newSettingsFixture(t, true)
	rec := send(t, f.h, http.MethodPatch, "/api/settings", `{"schedule":"every so often","githubToken":"`+patchedGitHubTok+`","publicUrl":"https://ok.lan"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	noSecrets(t, rec.Body.String())
	var body struct {
		Error  string            `json:"error"`
		Fields map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "Some settings aren't valid" || !strings.HasPrefix(body.Fields["schedule"], "invalid cron spec") ||
		body.Fields["githubToken"] != "set by TIDEMARK_GITHUB_TOKEN, change it there" || len(body.Fields) != 2 {
		t.Errorf("body = %s", rec.Body)
	}
	if got, _ := f.db.Settings(t.Context()); got["public_url"] != "" {
		t.Errorf("stored despite errors: %v", got)
	}
}

func TestPatchSettings(t *testing.T) {
	f := newSettingsFixture(t, true)
	rec := send(t, f.h, http.MethodPatch, "/api/settings",
		`{"schedule":"@daily","telegramBotToken":"`+patchedBotToken+`","notifyLevel":"Attention","telegramChatId":null}`)
	// Clearing the chat ID while setting the token leaves half a pair.
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"telegramChatId":"needed together with the bot token"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	noSecrets(t, rec.Body.String())

	rec = send(t, f.h, http.MethodPatch, "/api/settings",
		`{"schedule":"@daily","telegramBotToken":"`+patchedBotToken+`","notifyLevel":"Attention","publicUrl":"",`+
			`"aiProvider":"OpenAI","aiApiKey":"`+patchedAIKey+`","aiModel":"gpt-test","aiBaseUrl":"http://ollama.lan:11434/v1/"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body)
	}
	noSecrets(t, rec.Body.String())
	var body settingsJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Editable || *body.Fields["schedule"].Value != "@daily" || body.Fields["schedule"].Source != "stored" ||
		*body.Fields["notifyLevel"].Value != "attention" || !*body.Fields["telegramBotToken"].Set || body.Warnings != nil {
		t.Errorf("body = %s", rec.Body)
	}
	if body.AI == nil || body.AI.Usage == nil || body.AI.Usage.Calls != 0 {
		t.Fatalf("ai = %+v", body.AI)
	}
	if a := withoutUsage(*body.AI); a != (aiSettingsJSON{Provider: "openai", Model: "gpt-test", BaseURL: "http://ollama.lan:11434/v1", AutoMinReleases: 4}) {
		t.Errorf("ai = %+v", body.AI)
	}
	if body.Checks.Schedule != "@daily" || f.checks.Schedule() != "@daily" {
		t.Errorf("schedule not applied: %+v, %q", body.Checks, f.checks.Schedule())
	}
	got, _ := f.db.Settings(t.Context())
	if got["telegram_bot_token"] != patchedBotToken || got["schedule"] != "@daily" || got["ai_api_key"] != patchedAIKey {
		t.Errorf("stored = %v", got)
	}

	// Provider off: no ai block.
	rec = send(t, f.h, http.MethodPatch, "/api/settings", `{"aiProvider":"off"}`)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"ai":`) {
		t.Errorf("ai off: %d %s", rec.Code, rec.Body)
	}

	// Clear the schedule: back to the default.
	rec = send(t, f.h, http.MethodPatch, "/api/settings", `{"schedule":null}`)
	if rec.Code != http.StatusOK || f.checks.Schedule() != "0 */6 * * *" {
		t.Errorf("got %d, schedule %q", rec.Code, f.checks.Schedule())
	}
}

func withoutUsage(a aiSettingsJSON) aiSettingsJSON {
	a.Usage = nil
	return a
}

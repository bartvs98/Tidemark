package settings

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartvs98/tidemark/internal/analysis"
	"github.com/bartvs98/tidemark/internal/config"
	"github.com/bartvs98/tidemark/internal/store"
)

type recorder struct {
	schedules []string
	tokens    []string
	icons     []string
	notify    []config.Runtime
	notifyErr error
	ai        []config.Runtime
	aiErr     error
}

func (r *recorder) appliers() Appliers {
	return Appliers{
		Schedule:    func(spec string) error { r.schedules = append(r.schedules, spec); return nil },
		GitHubToken: func(token string) { r.tokens = append(r.tokens, token) },
		Icons: func(on bool, url string) {
			if !on {
				url = "off"
			}
			r.icons = append(r.icons, url)
		},
		Notify: func(rt config.Runtime) error { r.notify = append(r.notify, rt); return r.notifyErr },
		AI:     func(rt config.Runtime) error { r.ai = append(r.ai, rt); return r.aiErr },
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func newManager(t *testing.T, st *store.Store, env map[config.Key]string, editable bool) (*Manager, *recorder, *bytes.Buffer) {
	t.Helper()
	r := &recorder{}
	var logs bytes.Buffer
	m := New(st, env, editable, r.appliers(), slog.New(slog.NewTextHandler(&logs, nil)))
	if err := m.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	return m, r, &logs
}

func ptr(s string) *string { return &s }

func view(m *Manager, k config.Key) FieldView {
	for _, v := range m.View() {
		if v.Field.Key == k {
			return v
		}
	}
	return FieldView{}
}

func TestStartAppliesDefaultsAndStored(t *testing.T) {
	st := openStore(t)
	if err := st.SaveSettings(t.Context(), map[string]string{"schedule": "@daily", "github_token": "ghp_stored", "notify_digest": "nope"}, nil); err != nil {
		t.Fatal(err)
	}
	m, r, logs := newManager(t, st, map[config.Key]string{config.KeyIcons: "off"}, false)
	if len(r.schedules) != 1 || r.schedules[0] != "@daily" || r.tokens[0] != "ghp_stored" || r.icons[0] != "off" || len(r.notify) != 1 {
		t.Errorf("applied %+v", r)
	}
	if v := view(m, config.KeyGitHubToken); v.Value != "" || !v.Set || v.Source != config.SourceStored {
		t.Errorf("token view = %+v", v)
	}
	if v := view(m, config.KeyIcons); v.Value != "off" || v.Source != config.SourceEnv {
		t.Errorf("icons view = %+v", v)
	}
	if v := view(m, config.KeyNotifyDigest); v.Value != "" || v.Source != config.SourceDefault {
		t.Errorf("digest view = %+v", v)
	}
	if w := m.Warnings(); len(w) != 1 || !strings.HasPrefix(w[0], "stored notify_digest: invalid cron spec") {
		t.Errorf("warnings = %q", w)
	}
	if !strings.Contains(logs.String(), "stored notify_digest") || strings.Contains(logs.String(), "ghp_stored") {
		t.Errorf("logs = %s", logs)
	}
	if len(m.View()) != len(config.Fields) {
		t.Errorf("%d fields", len(m.View()))
	}
}

func TestStartApplierError(t *testing.T) {
	r := &recorder{notifyErr: errors.New("boom")}
	m := New(openStore(t), nil, false, r.appliers(), nil)
	if err := m.Start(t.Context()); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v", err)
	}
}

func TestUpdateReadOnly(t *testing.T) {
	m, _, _ := newManager(t, openStore(t), nil, false)
	if err := m.Update(t.Context(), Patch{config.KeySchedule: ptr("@daily")}); !errors.Is(err, ErrReadOnly) {
		t.Errorf("err = %v", err)
	}
}

func TestUpdate(t *testing.T) {
	st := openStore(t)
	m, r, logs := newManager(t, st, nil, true)
	ctx := t.Context()
	err := m.Update(ctx, Patch{
		config.KeySchedule:         ptr("OFF"),
		config.KeyGitHubToken:      ptr("ghp_new"),
		config.KeyTelegramBotToken: ptr("123:secret"),
		config.KeyTelegramChatID:   ptr("-100"),
		config.KeyNotifyLevel:      ptr("Breaking"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := st.Settings(ctx)
	if got["schedule"] != "off" || got["github_token"] != "ghp_new" || got["notify_level"] != "breaking" || len(got) != 5 {
		t.Errorf("stored = %v", got)
	}
	if n := len(r.schedules); n != 2 || r.schedules[1] != "" || r.tokens[1] != "ghp_new" || len(r.icons) != 2 {
		t.Errorf("applied %+v", r)
	}
	rt := r.notify[len(r.notify)-1]
	if !rt.Telegram() || rt.NotifyLevel != analysis.Breaking || rt != m.Runtime() {
		t.Errorf("notify got %+v", rt)
	}
	if v := view(m, config.KeySchedule); v.Value != "off" || v.Source != config.SourceStored {
		t.Errorf("schedule view = %+v", v)
	}
	if strings.Contains(logs.String(), "ghp_new") || strings.Contains(logs.String(), "123:secret") {
		t.Errorf("a secret was logged: %s", logs)
	}

	// Clear: nil and "" both remove the stored value.
	if err := m.Update(ctx, Patch{config.KeySchedule: nil, config.KeyGitHubToken: ptr(""), config.KeyNotifyLevel: ptr("  ")}); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Settings(ctx)
	if _, ok := got["schedule"]; ok || got["github_token"] != "" || len(got) != 2 {
		t.Errorf("stored after clear = %v", got)
	}
	if r.schedules[2] != "0 */6 * * *" || r.tokens[2] != "" {
		t.Errorf("applied after clear %+v", r)
	}
	if v := view(m, config.KeyGitHubToken); v.Set || v.Source != config.SourceDefault {
		t.Errorf("token view = %+v", v)
	}
}

func TestUpdateInvalid(t *testing.T) {
	st := openStore(t)
	m, r, _ := newManager(t, st, map[config.Key]string{config.KeyIconsURL: "https://icons.lan"}, true)
	err := m.Update(t.Context(), Patch{
		config.KeySchedule:     ptr("every so often"),
		config.KeyIconsURL:     ptr("https://other.lan"),
		config.KeyPublicURL:    ptr("https://ok.lan"),
		config.KeyNotifyDigest: ptr("off"),
	})
	var fe FieldErrors
	if !errors.As(err, &fe) {
		t.Fatalf("err = %v", err)
	}
	if !strings.HasPrefix(fe["schedule"], "invalid cron spec") || fe["iconsUrl"] != "set by TIDEMARK_ICONS_URL, change it there" ||
		!strings.HasPrefix(fe["notifyDigest"], "invalid cron spec") || len(fe) != 3 {
		t.Errorf("errors = %v", fe)
	}
	if got, _ := st.Settings(t.Context()); len(got) != 0 {
		t.Errorf("stored despite errors: %v", got)
	}
	if len(r.schedules) != 1 {
		t.Errorf("applied despite errors: %+v", r)
	}
}

func TestUpdateTelegramPair(t *testing.T) {
	st := openStore(t)
	m, _, _ := newManager(t, st, nil, true)
	ctx := t.Context()
	var fe FieldErrors
	err := m.Update(ctx, Patch{config.KeyTelegramBotToken: ptr("123:abc")})
	if !errors.As(err, &fe) || fe["telegramChatId"] != "needed together with the bot token" || len(fe) != 1 {
		t.Errorf("token only: %v", err)
	}
	err = m.Update(ctx, Patch{config.KeyTelegramChatID: ptr("42")})
	if !errors.As(err, &fe) || fe["telegramBotToken"] != "needed together with the chat ID" {
		t.Errorf("chat only: %v", err)
	}
	if got, _ := st.Settings(ctx); len(got) != 0 {
		t.Errorf("stored a half pair: %v", got)
	}
	if err := m.Update(ctx, Patch{config.KeyTelegramBotToken: ptr("123:abc"), config.KeyTelegramChatID: ptr("42")}); err != nil {
		t.Fatal(err)
	}
	// Clearing one half of a working pair is refused too.
	if err := m.Update(ctx, Patch{config.KeyTelegramChatID: nil}); !errors.As(err, &fe) || fe["telegramChatId"] == "" {
		t.Errorf("clear chat: %v", err)
	}
	if err := m.Update(ctx, Patch{config.KeyTelegramChatID: nil, config.KeyTelegramBotToken: nil}); err != nil {
		t.Errorf("clear both: %v", err)
	}
}

// A half pair from an env var doesn't block saving unrelated settings.
func TestUpdateHalfPairFromEnv(t *testing.T) {
	m, _, _ := newManager(t, openStore(t), map[config.Key]string{config.KeyTelegramBotToken: "123:abc"}, true)
	if err := m.Update(t.Context(), Patch{config.KeySchedule: ptr("@daily")}); err != nil {
		t.Errorf("err = %v", err)
	}
	if err := m.Update(t.Context(), Patch{config.KeyTelegramChatID: ptr("42")}); err != nil {
		t.Errorf("err = %v", err)
	}
	if !m.Runtime().Telegram() {
		t.Error("telegram off with env token + stored chat ID")
	}
}

func TestUpdateApplierError(t *testing.T) {
	st := openStore(t)
	m, r, _ := newManager(t, st, nil, true)
	r.notifyErr = errors.New("boom")
	err := m.Update(t.Context(), Patch{config.KeyPublicURL: ptr("https://t.lan")})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v", err)
	}
	var fe FieldErrors
	if errors.As(err, &fe) {
		t.Error("an applier error is not a field error")
	}
	if got, _ := st.Settings(context.Background()); got["public_url"] != "https://t.lan" {
		t.Errorf("not stored: %v", got)
	}
}

func TestUpdateAI(t *testing.T) {
	st := openStore(t)
	m, r, logs := newManager(t, st, nil, true)
	if len(r.ai) != 1 || r.ai[0].AI() {
		t.Fatalf("start applied %+v", r.ai)
	}
	err := m.Update(t.Context(), Patch{
		config.KeyAIProvider: ptr("Anthropic"),
		config.KeyAIAPIKey:   ptr("sk-ant-secret"),
		config.KeyAIAutoMin:  ptr("3"),
	})
	if err != nil {
		t.Fatal(err)
	}
	rt := r.ai[len(r.ai)-1]
	if !rt.AI() || rt.AIProvider != "anthropic" || rt.AIAPIKey != "sk-ant-secret" || rt.AIAutoMin != 3 {
		t.Errorf("ai got %+v", rt)
	}
	if v := view(m, config.KeyAIAPIKey); v.Value != "" || !v.Set {
		t.Errorf("key view = %+v", v)
	}
	if strings.Contains(logs.String(), "sk-ant-secret") {
		t.Errorf("the key was logged: %s", logs)
	}

	// openai has no default model, so it can't be stored without one.
	var fe FieldErrors
	err = m.Update(t.Context(), Patch{config.KeyAIProvider: ptr("openai")})
	if !errors.As(err, &fe) || fe["aiModel"] != "needed for the openai provider" || len(fe) != 1 {
		t.Errorf("openai without model: %v", err)
	}
	if err := m.Update(t.Context(), Patch{config.KeyAIProvider: ptr("openai"), config.KeyAIModel: ptr("gpt-x")}); err != nil {
		t.Errorf("openai with model: %v", err)
	}

	r.aiErr = errors.New("boom")
	err = m.Update(t.Context(), Patch{config.KeyAIAutoMin: ptr("0")})
	if err == nil || !strings.HasPrefix(err.Error(), "AI summaries: boom") || errors.As(err, &fe) {
		t.Errorf("err = %v", err)
	}
}

func TestFieldErrorsText(t *testing.T) {
	e := FieldErrors{"schedule": "bad", "iconsUrl": "worse"}
	if e.Error() != "invalid settings: iconsUrl: worse; schedule: bad" {
		t.Errorf("%q", e.Error())
	}
}

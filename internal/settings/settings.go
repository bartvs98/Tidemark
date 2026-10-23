// Package settings keeps the editable settings: it merges env vars, values stored from the
// settings dialog and defaults (config.Resolve), validates changes and pushes the effective
// values into the running services, so a change applies without a restart.
package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/bartvs98/tidemark/internal/config"
	"github.com/bartvs98/tidemark/internal/store"
)

// Appliers push effective settings into the running services. Called by Manager.Start and
// after each successful Update with the full new Runtime (each applier decides whether
// anything changed). A nil applier is skipped.
type Appliers struct {
	Schedule    func(spec string) error // "" = off
	GitHubToken func(token string)
	Icons       func(on bool, url string)
	Notify      func(rt config.Runtime) error // uses Telegram(), TelegramBotToken/ChatID, NotifyLevel, NotifyDigest, PublicURL
	AI          func(rt config.Runtime) error // uses AI(), AIProvider, AIAPIKey, AIModel, AIBaseURL, AIEffort, AIAutoMin
}

// ErrReadOnly is returned by Update unless TIDEMARK_SETTINGS_EDITABLE is on.
var ErrReadOnly = errors.New("settings are read-only; set TIDEMARK_SETTINGS_EDITABLE=true to edit them here")

// FieldErrors maps a field's JSON name to its message; returned (as error) when validation
// fails. Nothing is stored then.
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	var parts []string
	for _, name := range slices.Sorted(maps.Keys(e)) {
		parts = append(parts, name+": "+e[name])
	}
	return "invalid settings: " + strings.Join(parts, "; ")
}

// Patch maps a key to its new raw value. nil removes the stored value (back to the
// default; "Clear" for secrets); a non-nil "" is treated the same as nil.
type Patch map[config.Key]*string

// FieldView is one field's state for the API.
type FieldView struct {
	Field  config.Field
	Value  string // canonical effective value; "" for secrets (never filled)
	Set    bool   // secrets: whether a value is in effect
	Source config.Source
}

type Manager struct {
	st       *store.Store
	env      map[config.Key]string
	editable bool
	a        Appliers
	log      *slog.Logger

	update sync.Mutex // one Update (store + apply) at a time

	mu       sync.RWMutex
	rt       config.Runtime
	sources  map[config.Key]config.Source
	values   map[config.Key]string // secrets included: never expose
	warnings []string
}

func New(st *store.Store, env map[config.Key]string, editable bool, a Appliers, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{st: st, env: env, editable: editable, a: a, log: log}
}

// Start loads stored values, resolves, logs warnings and applies. An applier error is
// returned (startup fails).
func (m *Manager) Start(ctx context.Context) error {
	m.update.Lock()
	defer m.update.Unlock()
	stored, err := m.stored(ctx)
	if err != nil {
		return err
	}
	rt := m.resolve(stored)
	for _, w := range m.Warnings() {
		m.log.Warn("settings: " + w)
	}
	return m.apply(rt)
}

func (m *Manager) Editable() bool { return m.editable }

func (m *Manager) Runtime() config.Runtime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rt
}

// View lists every field's state, in config.Fields order. Secret values are never in it.
func (m *Manager) View() []FieldView {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]FieldView, 0, len(config.Fields))
	for _, f := range config.Fields {
		v := FieldView{Field: f, Source: m.sources[f.Key]}
		if v.Source == "" {
			v.Source = config.SourceDefault
		}
		if f.Secret {
			v.Set = m.values[f.Key] != ""
		} else {
			v.Value = m.values[f.Key]
		}
		out = append(out, v)
	}
	return out
}

// Warnings are the problems found by the last Resolve (invalid stored values, half a
// Telegram pair). They never contain a secret.
func (m *Manager) Warnings() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Clone(m.warnings)
}

// Update validates every key (config.Normalize; a key set by an env var can't be changed
// here), checks the Telegram pair and the openai model when the patch touches them, stores the patch, re-resolves
// and calls every applier. Applier errors are joined and returned after storing.
func (m *Manager) Update(ctx context.Context, p Patch) error {
	if !m.editable {
		return ErrReadOnly
	}
	m.update.Lock()
	defer m.update.Unlock()

	errs := FieldErrors{}
	set := map[string]string{}
	var remove []string
	telegram, aiModel := false, false
	for k, raw := range p {
		f, ok := config.FieldByKey(k)
		if !ok {
			errs[string(k)] = "unknown setting"
			continue
		}
		if _, ok := m.env[k]; ok {
			errs[f.JSON] = fmt.Sprintf("set by %s, change it there", f.Env)
			continue
		}
		telegram = telegram || k == config.KeyTelegramBotToken || k == config.KeyTelegramChatID
		aiModel = aiModel || k == config.KeyAIProvider || k == config.KeyAIModel
		if raw == nil {
			remove = append(remove, string(k))
			continue
		}
		v, err := config.Normalize(k, *raw)
		switch {
		case err != nil:
			errs[f.JSON] = err.Error()
		case v == "":
			remove = append(remove, string(k))
		default:
			set[string(k)] = v
		}
	}
	if len(errs) > 0 {
		return errs
	}

	stored, err := m.stored(ctx)
	if err != nil {
		return err
	}
	for k, v := range set {
		stored[config.Key(k)] = v
	}
	for _, k := range remove {
		delete(stored, config.Key(k))
	}
	// Only when the patch touches the pair, so an unrelated save isn't blocked by a half
	// pair that was already there (e.g. the token in an env var, the chat ID not set yet).
	// Likewise the openai provider, which has no default model: storing it without one
	// would fail the AI applier at every start.
	if telegram || aiModel {
		rt, _, _, _ := config.Resolve(m.env, stored)
		switch {
		case !telegram:
		case rt.TelegramBotToken != "" && rt.TelegramChatID == "":
			errs["telegramChatId"] = "needed together with the bot token"
		case rt.TelegramBotToken == "" && rt.TelegramChatID != "":
			errs["telegramBotToken"] = "needed together with the chat ID"
		}
		if aiModel && rt.AIProvider == "openai" && rt.AIModel == "" {
			errs["aiModel"] = "needed for the openai provider"
		}
		if len(errs) > 0 {
			return errs
		}
	}

	if err := m.st.SaveSettings(ctx, set, remove); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	keys := slices.Sorted(maps.Keys(p))
	m.log.Info("settings changed", "keys", keys) // names only, never values
	return m.apply(m.resolve(stored))
}

func (m *Manager) stored(ctx context.Context) (map[config.Key]string, error) {
	raw, err := m.st.Settings(ctx)
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	out := make(map[config.Key]string, len(raw))
	for k, v := range raw {
		out[config.Key(k)] = v
	}
	return out, nil
}

func (m *Manager) resolve(stored map[config.Key]string) config.Runtime {
	rt, sources, values, warnings := config.Resolve(m.env, stored)
	m.mu.Lock()
	m.rt, m.sources, m.values, m.warnings = rt, sources, values, warnings
	m.mu.Unlock()
	return rt
}

func (m *Manager) apply(rt config.Runtime) error {
	var errs []error
	if m.a.Schedule != nil {
		if err := m.a.Schedule(rt.Schedule); err != nil {
			errs = append(errs, fmt.Errorf("check schedule: %w", err))
		}
	}
	if m.a.GitHubToken != nil {
		m.a.GitHubToken(rt.GitHubToken)
	}
	if m.a.Icons != nil {
		m.a.Icons(rt.Icons, rt.IconsURL)
	}
	if m.a.Notify != nil {
		if err := m.a.Notify(rt); err != nil {
			errs = append(errs, fmt.Errorf("notifications: %w", err))
		}
	}
	if m.a.AI != nil {
		if err := m.a.AI(rt); err != nil {
			errs = append(errs, fmt.Errorf("AI summaries: %w", err))
		}
	}
	return errors.Join(errs...)
}

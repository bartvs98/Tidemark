package ai

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultAnthropicModel = "claude-haiku-5-5"
	DefaultGeminiModel    = "gemini-flash-latest"
	OpenAIBaseURL         = "https://api.openai.com/v1"
	GeminiBaseURL         = "https://generativelanguage.googleapis.com/v1beta/openai"

	anthropicBaseURL = "https://api.anthropic.com/"
	httpTimeout      = 2 * time.Minute
)

// Prompt is what a provider sends.
type Prompt struct {
	System string
	User   string
	Schema map[string]any // JSON schema of the output (additionalProperties false everywhere)
}

// Usage is what one provider call cost, as the provider reports it (0 when it doesn't).
type Usage struct{ InputTokens, OutputTokens int }

// Provider runs one completion and returns the model's JSON object (raw) and its usage,
// also on errors when the provider reported it.
type Provider interface {
	Name() string   // anthropic | openai | gemini
	Model() string  // effective model
	Effort() string // effective reasoning effort (low | medium | high); "" = not sent
	Complete(ctx context.Context, p Prompt) (json.RawMessage, Usage, error)
}

// NewProvider builds the provider for the settings. model "" = provider default; openai has no
// default model. baseURL only applies to openai. effort "" = provider default: low for
// anthropic and gemini; for openai nothing is sent (not every compatible server knows it).
func NewProvider(name, apiKey, model, baseURL, effort string) (Provider, error) {
	switch name {
	case "anthropic":
		if apiKey == "" {
			return nil, errors.New("set TIDEMARK_AI_API_KEY for the anthropic provider")
		}
		c := newAnthropic(apiKey, model, anthropicBaseURL, 2)
		c.effort = cmp.Or(effort, "low")
		return c, nil
	case "openai":
		if model == "" {
			return nil, errors.New("set TIDEMARK_AI_MODEL for the openai provider")
		}
		if baseURL == "" {
			baseURL = OpenAIBaseURL
		}
		c := newOpenAI("openai", apiKey, model, baseURL)
		c.effort = effort
		return c, nil
	case "gemini":
		if apiKey == "" {
			return nil, errors.New("set TIDEMARK_AI_API_KEY for the gemini provider")
		}
		if model == "" {
			model = DefaultGeminiModel
		}
		c := newOpenAI("gemini", apiKey, model, GeminiBaseURL)
		// Gemini thinks by default, which adds 20-30 s per call; with an overloaded model
		// that pushed summaries past httpTimeout. Low effort answers in seconds.
		c.effort = cmp.Or(effort, "low")
		return c, nil
	}
	return nil, fmt.Errorf("unknown AI provider %q (want anthropic, openai or gemini)", name)
}

// ── Errors ───────────────────────────────────────────────

var (
	errDeclined = errors.New("the model declined")
	errCutOff   = errors.New("the answer was cut off")
)

// statusError is the short message for a failed HTTP status.
func statusError(provider, model string, status int) error {
	switch {
	case status == 401 || status == 403:
		return errors.New("the API key was rejected")
	case status == 404:
		return fmt.Errorf("model not found: %s", model)
	case status == 429:
		return fmt.Errorf("rate limited by %s", provider)
	case status >= 500:
		return fmt.Errorf("%s error (status %d)", provider, status)
	}
	return fmt.Errorf("%s rejected the request (status %d)", provider, status)
}

func unreachable(provider string) error { return fmt.Errorf("couldn't reach %s", provider) }

// scrub replaces the key in an error message, in case it slipped in anyway.
func scrub(err error, key string) error {
	if err == nil || key == "" || !strings.Contains(err.Error(), key) {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), key, "***"))
}

// netError is the message for a request that got no answer. A cancelled context stays
// recognisable (errors.Is) without the *url.Error, whose message contains the URL.
func netError(ctx context.Context, provider string, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%s: %w", unreachable(provider), ctx.Err())
	}
	var ue *url.Error
	if errors.As(err, &ue) && ue.Timeout() {
		return fmt.Errorf("couldn't reach %s (timed out)", provider)
	}
	return unreachable(provider)
}

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicClient talks to the Claude API through the official SDK, with structured
// output (json_schema) so the answer is always the summary object.
type anthropicClient struct {
	key, model string
	client     anthropic.Client
}

// newAnthropic builds the client. The SDK's environment defaults (ANTHROPIC_* variables,
// credential files) are switched off: only the configured key goes to the configured host.
func newAnthropic(key, model, baseURL string, retries int) *anthropicClient {
	if model == "" {
		model = DefaultAnthropicModel
	}
	return &anthropicClient{key: key, model: model, client: anthropic.NewClient(
		option.WithoutEnvironmentDefaults(),
		option.WithBaseURL(baseURL),
		option.WithAPIKey(key),
		option.WithHTTPClient(&http.Client{Timeout: httpTimeout}),
		option.WithMaxRetries(retries),
	)}
}

func (c *anthropicClient) Name() string  { return "anthropic" }
func (c *anthropicClient) Model() string { return c.model }

func (c *anthropicClient) Complete(ctx context.Context, p Prompt) (json.RawMessage, error) {
	msg, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{{Text: p.System}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(p.User))},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffortLow,
			Format: anthropic.JSONOutputFormatParam{Schema: p.Schema},
		},
	})
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return nil, statusError("anthropic", c.model, apiErr.StatusCode)
		}
		return nil, scrub(netError(ctx, "anthropic", err), c.key)
	}
	switch msg.StopReason {
	case anthropic.StopReasonRefusal:
		return nil, errDeclined
	case anthropic.StopReasonMaxTokens:
		return nil, errCutOff
	}
	for _, b := range msg.Content {
		if b.Type == "text" {
			return json.RawMessage(stripFence(b.Text)), nil
		}
	}
	return nil, errors.New("the model's answer was empty")
}

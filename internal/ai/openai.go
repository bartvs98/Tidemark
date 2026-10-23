package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// openAIClient talks to any OpenAI-compatible chat completions endpoint (OpenAI,
// OpenRouter, Ollama, LM Studio, Gemini's compatibility layer) with plain net/http.
type openAIClient struct {
	name, key, model, base string
	effort                 string // reasoning_effort to send; "" = leave it out (not every server knows it)
	client                 *http.Client
}

func newOpenAI(name, key, model, baseURL string) *openAIClient {
	return &openAIClient{name: name, key: key, model: model, base: strings.TrimRight(baseURL, "/"),
		client: &http.Client{Timeout: httpTimeout}}
}

func (c *openAIClient) Name() string   { return c.name }
func (c *openAIClient) Model() string  { return c.model }
func (c *openAIClient) Effort() string { return c.effort }

func (c *openAIClient) Complete(ctx context.Context, p Prompt) (json.RawMessage, Usage, error) {
	raw, u, err := c.complete(ctx, p)
	return raw, u, scrub(err, c.key)
}

func (c *openAIClient) complete(ctx context.Context, p Prompt) (json.RawMessage, Usage, error) {
	req := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": p.System},
			{"role": "user", "content": p.User},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "release_summary",
				"strict": true,
				"schema": p.Schema,
			},
		},
	}
	if c.effort != "" {
		req["reasoning_effort"] = c.effort
	}
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, Usage{}, fmt.Errorf("invalid %s base URL", c.name)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, Usage{}, netError(ctx, c.name, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, Usage{}, netError(ctx, c.name, err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, Usage{}, statusError(c.name, c.model, resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
				Refusal *string `json:"refusal"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Usage{}, fmt.Errorf("%s sent an answer Tidemark can't read", c.name)
	}
	u := Usage{InputTokens: out.Usage.PromptTokens, OutputTokens: out.Usage.CompletionTokens}
	if len(out.Choices) == 0 {
		return nil, u, fmt.Errorf("%s sent an answer Tidemark can't read", c.name)
	}
	ch := out.Choices[0]
	switch {
	case ch.Message.Refusal != nil && *ch.Message.Refusal != "":
		return nil, u, errDeclined
	case ch.FinishReason == "length":
		return nil, u, errCutOff
	case ch.FinishReason == "content_filter":
		return nil, u, errDeclined
	case ch.Message.Content == nil || strings.TrimSpace(*ch.Message.Content) == "":
		return nil, u, errors.New("the model's answer was empty")
	}
	return json.RawMessage(stripFence(*ch.Message.Content)), u, nil
}

// stripFence removes a ```json … ``` fence some models put around the object.
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:] // drop the info string ("json")
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
}

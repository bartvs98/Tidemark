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
	client                 *http.Client
}

func newOpenAI(name, key, model, baseURL string) *openAIClient {
	return &openAIClient{name: name, key: key, model: model, base: strings.TrimRight(baseURL, "/"),
		client: &http.Client{Timeout: httpTimeout}}
}

func (c *openAIClient) Name() string  { return c.name }
func (c *openAIClient) Model() string { return c.model }

func (c *openAIClient) Complete(ctx context.Context, p Prompt) (json.RawMessage, error) {
	raw, err := c.complete(ctx, p)
	return raw, scrub(err, c.key)
}

func (c *openAIClient) complete(ctx context.Context, p Prompt) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{
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
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("invalid %s base URL", c.name)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, netError(ctx, c.name, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, netError(ctx, c.name, err)
	}
	if resp.StatusCode/100 != 2 {
		return nil, statusError(c.name, c.model, resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
				Refusal *string `json:"refusal"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil || len(out.Choices) == 0 {
		return nil, fmt.Errorf("%s sent an answer Tidemark can't read", c.name)
	}
	ch := out.Choices[0]
	switch {
	case ch.Message.Refusal != nil && *ch.Message.Refusal != "":
		return nil, errDeclined
	case ch.FinishReason == "length":
		return nil, errCutOff
	case ch.FinishReason == "content_filter":
		return nil, errDeclined
	case ch.Message.Content == nil || strings.TrimSpace(*ch.Message.Content) == "":
		return nil, errors.New("the model's answer was empty")
	}
	return json.RawMessage(stripFence(*ch.Message.Content)), nil
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

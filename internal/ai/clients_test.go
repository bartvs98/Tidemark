package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testKey = "sk-secret-key-123"

var testPrompt = Prompt{System: "sys", User: "user msg", Schema: outputSchema()}

const answer = `{"changes":["c"],"actions":[],"verdict":"none","reasons":[]}`

// server records the last request and answers with status and body.
func server(t *testing.T, status int, body string) (*httptest.Server, *map[string]any, *http.Header) {
	t.Helper()
	var got map[string]any
	var hdr http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		got = map[string]any{"_path": r.URL.Path}
		json.Unmarshal(b, &got)
		got["_path"] = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &got, &hdr
}

func anthropicMsg(stop, text string) string {
	b, _ := json.Marshal(map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": DefaultAnthropicModel,
		"content":     []map[string]any{{"type": "text", "text": text}},
		"stop_reason": stop, "usage": map[string]int{"input_tokens": 100, "cache_creation_input_tokens": 5,
			"cache_read_input_tokens": 20, "output_tokens": 7},
	})
	return string(b)
}

func TestAnthropicRequestAndSuccess(t *testing.T) {
	srv, got, hdr := server(t, 200, anthropicMsg("end_turn", answer))
	c := newAnthropic(testKey, "", srv.URL+"/", 0)
	if c.Name() != "anthropic" || c.Model() != DefaultAnthropicModel {
		t.Errorf("name/model %s %s", c.Name(), c.Model())
	}
	raw, u, err := c.Complete(context.Background(), testPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != answer {
		t.Errorf("raw = %s", raw)
	}
	if u != (Usage{InputTokens: 125, OutputTokens: 7}) { // cache tokens count as input
		t.Errorf("usage = %+v", u)
	}
	g := *got
	if g["_path"] != "/v1/messages" || hdr.Get("X-Api-Key") != testKey || hdr.Get("Authorization") != "" {
		t.Errorf("path %v, key header %q, auth %q", g["_path"], hdr.Get("X-Api-Key"), hdr.Get("Authorization"))
	}
	if g["model"] != DefaultAnthropicModel || g["max_tokens"] != float64(4096) {
		t.Errorf("model/max_tokens: %v %v", g["model"], g["max_tokens"])
	}
	if _, ok := g["thinking"]; ok {
		t.Error("thinking must not be set")
	}
	if _, ok := g["temperature"]; ok {
		t.Error("temperature must not be set")
	}
	oc, _ := g["output_config"].(map[string]any)
	format, _ := oc["format"].(map[string]any)
	if oc["effort"] != "low" || format["type"] != "json_schema" || format["schema"] == nil {
		t.Errorf("output_config = %v", g["output_config"])
	}
	msgs, _ := g["messages"].([]any)
	if len(msgs) != 1 || !strings.Contains(mustJSON(g["system"]), "sys") || !strings.Contains(mustJSON(msgs[0]), "user msg") {
		t.Errorf("system/messages: %v %v", g["system"], msgs)
	}
}

func TestAnthropicStopReasonsAndErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{200, anthropicMsg("refusal", ""), "the model declined"},
		{200, anthropicMsg("max_tokens", "{"), "the answer was cut off"},
		{401, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key ` + testKey + `"}}`, "the API key was rejected"},
		{404, `{"type":"error","error":{"type":"not_found_error","message":"model"}}`, "model not found: m1"},
		{429, `{"type":"error","error":{"type":"rate_limit_error","message":"slow"}}`, "rate limited by anthropic"},
		{529, `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`, "anthropic error (status 529)"},
	} {
		srv, _, _ := server(t, tc.status, tc.body)
		_, _, err := newAnthropic(testKey, "m1", srv.URL+"/", 0).Complete(context.Background(), testPrompt)
		if err == nil || err.Error() != tc.want {
			t.Errorf("%d: err = %v, want %q", tc.status, err, tc.want)
		}
	}
	srv, _, _ := server(t, 200, "")
	srv.Close()
	_, _, err := newAnthropic(testKey, "", srv.URL+"/", 0).Complete(context.Background(), testPrompt)
	if err == nil || !strings.HasPrefix(err.Error(), "couldn't reach anthropic") || strings.Contains(err.Error(), testKey) {
		t.Errorf("network: %v", err)
	}
}

func openAIAnswer(finish, content string) string {
	b, _ := json.Marshal(map[string]any{"choices": []map[string]any{{
		"message": map[string]any{"role": "assistant", "content": content}, "finish_reason": finish}}})
	return string(b)
}

func TestOpenAIRequestAndSuccess(t *testing.T) {
	srv, got, hdr := server(t, 200, openAIAnswer("stop", "```json\n"+answer+"\n```"))
	c := newOpenAI("openai", testKey, "gpt-x", srv.URL+"/v1/")
	raw, u, err := c.Complete(context.Background(), testPrompt)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != answer {
		t.Errorf("fence not stripped: %s", raw)
	}
	if u != (Usage{}) { // no usage in the answer
		t.Errorf("usage without one = %+v", u)
	}
	g := *got
	if g["_path"] != "/v1/chat/completions" || hdr.Get("Authorization") != "Bearer "+testKey || g["model"] != "gpt-x" {
		t.Errorf("path %v auth %q model %v", g["_path"], hdr.Get("Authorization"), g["model"])
	}
	rf, _ := g["response_format"].(map[string]any)
	js, _ := rf["json_schema"].(map[string]any)
	if rf["type"] != "json_schema" || js["name"] != "release_summary" || js["strict"] != true || js["schema"] == nil {
		t.Errorf("response_format = %v", rf)
	}
	msgs, _ := g["messages"].([]any)
	if len(msgs) != 2 || mustJSON(msgs[0]) != `{"content":"sys","role":"system"}` || mustJSON(msgs[1]) != `{"content":"user msg","role":"user"}` {
		t.Errorf("messages = %v", msgs)
	}

	// A local server needs no key: no Authorization header at all.
	srv, _, hdr = server(t, 200, openAIAnswer("stop", answer))
	if _, _, err := newOpenAI("openai", "", "llama", srv.URL).Complete(context.Background(), testPrompt); err != nil {
		t.Fatal(err)
	}
	if _, ok := (*hdr)["Authorization"]; ok {
		t.Error("Authorization sent without a key")
	}
}

func TestOpenAIUsage(t *testing.T) {
	withUsage := func(finish, content string) string {
		var m map[string]any
		json.Unmarshal([]byte(openAIAnswer(finish, content)), &m)
		m["usage"] = map[string]int{"prompt_tokens": 1234, "completion_tokens": 56, "total_tokens": 1290}
		return mustJSON(m)
	}
	srv, _, _ := server(t, 200, withUsage("stop", answer))
	_, u, err := newOpenAI("openai", testKey, "m", srv.URL).Complete(context.Background(), testPrompt)
	if err != nil || u != (Usage{InputTokens: 1234, OutputTokens: 56}) {
		t.Errorf("usage = %+v, %v", u, err)
	}
	// Also reported when the answer is a failure.
	srv, _, _ = server(t, 200, withUsage("length", "{"))
	_, u, err = newOpenAI("openai", testKey, "m", srv.URL).Complete(context.Background(), testPrompt)
	if err == nil || u != (Usage{InputTokens: 1234, OutputTokens: 56}) {
		t.Errorf("cut off: usage = %+v, %v", u, err)
	}
}

func TestOpenAIErrors(t *testing.T) {
	refusal, _ := json.Marshal(map[string]any{"choices": []map[string]any{{
		"message": map[string]any{"content": nil, "refusal": "no"}, "finish_reason": "stop"}}})
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{200, string(refusal), "the model declined"},
		{200, openAIAnswer("length", "{"), "the answer was cut off"},
		{200, `{"choices":[]}`, "gemini sent an answer Tidemark can't read"},
		{401, `{"error":{"message":"Incorrect API key provided: ` + testKey + `"}}`, "the API key was rejected"},
		{403, `{}`, "the API key was rejected"},
		{404, `{}`, "model not found: m1"},
		{429, `{}`, "rate limited by gemini"},
		{503, `{}`, "gemini error (status 503)"},
	} {
		srv, _, _ := server(t, tc.status, tc.body)
		_, _, err := newOpenAI("gemini", testKey, "m1", srv.URL).Complete(context.Background(), testPrompt)
		if err == nil || err.Error() != tc.want {
			t.Errorf("%d %s: err = %v, want %q", tc.status, tc.body, err, tc.want)
		}
	}
	srv, _, _ := server(t, 200, "")
	srv.Close()
	_, _, err := newOpenAI("openai", testKey, "m", srv.URL).Complete(context.Background(), testPrompt)
	if err == nil || err.Error() != "couldn't reach openai" {
		t.Errorf("network: %v", err)
	}
}

func TestScrub(t *testing.T) {
	err := scrub(io.ErrUnexpectedEOF, testKey)
	if err != io.ErrUnexpectedEOF {
		t.Error("unrelated error changed")
	}
	err = scrub(&jsonErr{"bad key " + testKey}, testKey)
	if strings.Contains(err.Error(), testKey) || !strings.Contains(err.Error(), "***") {
		t.Errorf("scrub: %v", err)
	}
}

type jsonErr struct{ s string }

func (e *jsonErr) Error() string { return e.s }

func TestNewProvider(t *testing.T) {
	for _, tc := range []struct {
		name, key, model, base string
		wantName, wantModel    string
		wantErr                string
	}{
		{"anthropic", "k", "", "", "anthropic", DefaultAnthropicModel, ""},
		{"anthropic", "k", "claude-x", "", "anthropic", "claude-x", ""},
		{"anthropic", "", "", "", "", "", "set TIDEMARK_AI_API_KEY for the anthropic provider"},
		{"openai", "k", "", "", "", "", "set TIDEMARK_AI_MODEL for the openai provider"},
		{"openai", "", "llama3", "http://ollama:11434/v1", "openai", "llama3", ""},
		{"gemini", "k", "", "http://ignored", "gemini", DefaultGeminiModel, ""},
		{"off", "k", "", "", "", "", `unknown AI provider "off" (want anthropic, openai or gemini)`},
	} {
		p, err := NewProvider(tc.name, tc.key, tc.model, tc.base)
		if tc.wantErr != "" {
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("%s: err = %v, want %q", tc.name, err, tc.wantErr)
			}
			continue
		}
		if err != nil || p.Name() != tc.wantName || p.Model() != tc.wantModel {
			t.Errorf("%s: %v %v", tc.name, p, err)
		}
	}
	p, _ := NewProvider("openai", "", "m", "")
	if p.(*openAIClient).base != OpenAIBaseURL {
		t.Error("openai default base URL")
	}
	p, _ = NewProvider("gemini", "k", "", "http://ignored")
	if p.(*openAIClient).base != GeminiBaseURL {
		t.Error("gemini base URL")
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

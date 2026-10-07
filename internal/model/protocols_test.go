package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// conversation exercises every message kind: system, user, assistant tool call, tool result.
func conversation() []Message {
	return []Message{
		Text("system", "be brief"),
		Text("user", "hi"),
		{Role: "assistant", ToolCalls: []ToolCall{{
			ID: "call_1", Type: "function", Function: FunctionCall{Name: "send_message", Arguments: `{"text":"hello"}`},
		}}},
		Text("tool", `{"ok":true}`),
		Text("user", "thanks"),
	}
}

func TestToResponsesRequest(t *testing.T) {
	msgs := conversation()
	msgs[3].ToolCallID = "call_1"
	got := toResponsesRequest(Request{Model: "gpt-x", Messages: msgs, MaxTokens: 1, Tools: []Tool{{
		Type: "function", Function: FunctionSpec{Name: "send_message", Parameters: json.RawMessage(`{"type":"object"}`)},
	}}})
	if got.Instructions != "be brief" || got.Store || got.MaxOutputTokens != 16 {
		t.Fatalf("unexpected request header fields: %+v", got)
	}
	want := []responsesItem{
		{Role: "user", Content: "hi"},
		{Type: "function_call", CallID: "call_1", Name: "send_message", Arguments: `{"text":"hello"}`},
		{Type: "function_call_output", CallID: "call_1", Output: `{"ok":true}`},
		{Role: "user", Content: "thanks"},
	}
	if len(got.Input) != len(want) {
		t.Fatalf("input = %+v", got.Input)
	}
	for i := range want {
		if got.Input[i] != want[i] {
			t.Errorf("input[%d] = %+v, want %+v", i, got.Input[i], want[i])
		}
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "send_message" || got.Tools[0].Type != "function" {
		t.Fatalf("tools = %+v", got.Tools)
	}
}

func TestToAnthropicRequestMergesRoles(t *testing.T) {
	msgs := conversation()
	msgs[3].ToolCallID = "call_1"
	got := toAnthropicRequest(Request{Model: "qwen-x", Messages: msgs})
	if got.System != "be brief" || got.MaxTokens != defaultMaxTokens {
		t.Fatalf("unexpected header fields: %+v", got)
	}
	// user, assistant(tool_use), user(tool_result + text): roles must alternate.
	if len(got.Messages) != 3 {
		t.Fatalf("expected 3 alternating messages, got %+v", got.Messages)
	}
	roles := []string{"user", "assistant", "user"}
	for i, m := range got.Messages {
		if m.Role != roles[i] {
			t.Fatalf("message %d role = %s", i, m.Role)
		}
	}
	tu := got.Messages[1].Content[0]
	if tu.Type != "tool_use" || tu.ID != "call_1" || string(tu.Input) != `{"text":"hello"}` {
		t.Fatalf("tool_use = %+v", tu)
	}
	last := got.Messages[2].Content
	if len(last) != 2 || last[0].Type != "tool_result" || last[0].ToolUseID != "call_1" || last[1].Text != "thanks" {
		t.Fatalf("merged user message = %+v", last)
	}
}

func TestFromResponses(t *testing.T) {
	var r responsesResponse
	json.Unmarshal([]byte(`{"status":"completed","output":[
		{"type":"reasoning"},
		{"type":"message","content":[{"type":"output_text","text":"hi "},{"type":"output_text","text":"there"}]},
		{"type":"function_call","call_id":"c1","name":"send_message","arguments":"{\"text\":\"x\"}"}
	],"usage":{"input_tokens":10,"output_tokens":3}}`), &r)
	got := fromResponsesResponse(r)
	if got.Message.Text() != "hi there" || got.FinishReason != "tool_calls" || got.Usage.PromptTokens != 10 {
		t.Fatalf("got %+v", got)
	}
	if tc := got.Message.ToolCalls; len(tc) != 1 || tc[0].ID != "c1" || tc[0].Function.Arguments != `{"text":"x"}` {
		t.Fatalf("tool calls = %+v", tc)
	}
}

func TestFromAnthropic(t *testing.T) {
	var r anthropicResponse
	json.Unmarshal([]byte(`{"content":[
		{"type":"thinking","thinking":"hmm"},
		{"type":"text","text":"ok"},
		{"type":"tool_use","id":"t1","name":"send_message","input":{"text":"x"}}
	],"stop_reason":"tool_use","usage":{"input_tokens":5,"output_tokens":2}}`), &r)
	got := fromAnthropicResponse(r)
	if got.Message.Text() != "ok" || got.FinishReason != "tool_calls" || got.Usage.CompletionTokens != 2 {
		t.Fatalf("got %+v", got)
	}
	if tc := got.Message.ToolCalls; len(tc) != 1 || tc[0].ID != "t1" || tc[0].Function.Arguments != `{"text":"x"}` {
		t.Fatalf("tool calls = %+v", tc)
	}
}

// protocolServer serves each model only on its real endpoint and answers others like
// OpenCode Go does.
func protocolServer(t *testing.T, served map[string]string) (*httptest.Server, func() []string) {
	var mu sync.Mutex
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		hits = append(hits, r.URL.Path)
		mu.Unlock()
		if served[body.Model] != r.URL.Path {
			// OpenCode Go uses both shapes: a 400, or a 401 ModelError on some endpoints.
			if r.URL.Path == "/responses" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"type":"error","error":{"type":"ModelError","message":"Model x is not supported for format openai"}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"type":"error","error":{"message":"Model does not support this protocol."}}`))
			return
		}
		switch r.URL.Path {
		case "/chat/completions":
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"cc"},"finish_reason":"stop"}]}`))
		case "/responses":
			w.Write([]byte(`{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"resp"}]}]}`))
		case "/messages":
			if r.Header.Get("X-Api-Key") != "k" || r.Header.Get("Anthropic-Version") == "" {
				t.Errorf("missing Anthropic headers")
			}
			w.Write([]byte(`{"content":[{"type":"text","text":"msg"}],"stop_reason":"end_turn"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := hits
		hits = nil
		return out
	}
}

func TestChatPicksAndRemembersProtocol(t *testing.T) {
	srv, hits := protocolServer(t, map[string]string{
		"kimi-k2.6":   "/chat/completions",
		"muse-spark":  "/responses",
		"qwen3.8-max": "/messages",
		"oddball":     "/messages", // guessed wrong; must fall back
	})
	c := New(srv.URL, "barn/test", func(context.Context) (string, error) { return "k", nil })
	ctx := context.Background()

	for model, want := range map[string]string{"kimi-k2.6": "cc", "muse-spark": "resp", "qwen3.8-max": "msg"} {
		resp, err := c.Chat(ctx, Request{Model: model, Messages: []Message{Text("user", "hi")}})
		if err != nil || resp.Message.Text() != want {
			t.Fatalf("%s: got %q, %v", model, resp.Message.Text(), err)
		}
		if h := hits(); len(h) != 1 {
			t.Fatalf("%s: expected the guessed protocol to work first time, hits %v", model, h)
		}
	}

	resp, err := c.Chat(ctx, Request{Model: "oddball", Messages: []Message{Text("user", "hi")}})
	if err != nil || resp.Message.Text() != "msg" {
		t.Fatalf("oddball: got %q, %v", resp.Message.Text(), err)
	}
	if h := hits(); len(h) != 3 {
		t.Fatalf("expected fallback through all protocols, hits %v", h)
	}
	if _, err := c.Chat(ctx, Request{Model: "oddball", Messages: []Message{Text("user", "hi")}}); err != nil {
		t.Fatal(err)
	}
	if h := hits(); len(h) != 1 || h[0] != "/messages" {
		t.Fatalf("expected the working protocol to be remembered, hits %v", h)
	}
}

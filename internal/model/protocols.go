package model

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// openbot keeps conversations in the OpenAI chat-completions shape (Message). OpenCode Go serves
// each model through one of three APIs, so requests are translated per protocol.

// errWrongProtocol means the provider doesn't serve this model through the API we called.
var errWrongProtocol = errors.New("model does not support this protocol")

type protocol struct {
	name string
	send func(ctx context.Context, c *Client, key string, req Request) (Response, error)
}

var (
	chatCompletions   = &protocol{name: "chat-completions", send: sendChatCompletions}
	responses         = &protocol{name: "responses", send: sendResponses}
	anthropicMessages = &protocol{name: "messages", send: sendAnthropicMessages}

	protocols = []*protocol{chatCompletions, responses, anthropicMessages}
)

// guessProtocol picks the likely API from the model id (per OpenCode Go's docs). Wrong guesses
// are corrected at runtime by falling back to the other protocols.
func guessProtocol(model string) *protocol {
	switch {
	case hasAnyPrefix(model, "gpt-", "grok-", "muse-"):
		return responses
	case hasAnyPrefix(model, "minimax-", "qwen"):
		return anthropicMessages
	default:
		return chatCompletions
	}
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// ---- /chat/completions ----

func sendChatCompletions(ctx context.Context, c *Client, key string, req Request) (Response, error) {
	var out struct {
		Choices []struct {
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
		Usage chatUsage `json:"usage"`
	}
	if err := c.do(ctx, key, req.Session, http.MethodPost, "/chat/completions", nil, toChatRequest(req), &out); err != nil {
		return Response{}, err
	}
	if len(out.Choices) == 0 {
		return Response{}, errors.New("provider returned no choices")
	}
	ch := out.Choices[0]
	return Response{Message: ch.Message, FinishReason: ch.FinishReason, Usage: out.Usage.usage()}, nil
}

// chatRequest is Request on the wire: user messages with images send their content as parts.
type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	Tools     []Tool        `json:"tools,omitempty"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role             string     `json:"role"`
	Content          any        `json:"content"` // string, nil, or []chatPart
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

type chatPart struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	ImageURL map[string]string `json:"image_url,omitempty"`
}

func toChatRequest(req Request) chatRequest {
	out := chatRequest{Model: req.Model, Tools: req.Tools, MaxTokens: req.MaxTokens}
	for _, m := range req.Messages {
		cm := chatMessage{Role: m.Role, ReasoningContent: m.ReasoningContent, ToolCalls: m.ToolCalls, ToolCallID: m.ToolCallID}
		switch {
		case len(m.Images) > 0:
			parts := []chatPart{{Type: "text", Text: m.Text()}}
			for _, img := range m.Images {
				parts = append(parts, chatPart{Type: "image_url", ImageURL: map[string]string{"url": img.dataURL()}})
			}
			cm.Content = parts
		case m.Content != nil:
			cm.Content = *m.Content
		}
		out.Messages = append(out.Messages, cm)
	}
	return out
}

// ---- /responses (OpenAI Responses API) ----

type responsesRequest struct {
	Model           string          `json:"model"`
	Instructions    string          `json:"instructions,omitempty"`
	Input           []responsesItem `json:"input"`
	Tools           []responsesTool `json:"tools,omitempty"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
	Store           bool            `json:"store"`
}

type responsesItem struct {
	Type      string `json:"type,omitempty"`
	Role      string `json:"role,omitempty"`
	Content   any    `json:"content,omitempty"` // string, or []map for text with images
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

func toResponsesRequest(req Request) responsesRequest {
	out := responsesRequest{Model: req.Model, Store: false}
	var instructions []string
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			instructions = append(instructions, m.Text())
		case "user":
			var content any = m.Text()
			if len(m.Images) > 0 {
				parts := []map[string]string{{"type": "input_text", "text": m.Text()}}
				for _, img := range m.Images {
					parts = append(parts, map[string]string{"type": "input_image", "image_url": img.dataURL()})
				}
				content = parts
			}
			out.Input = append(out.Input, responsesItem{Role: "user", Content: content})
		case "assistant":
			if text := m.Text(); text != "" {
				out.Input = append(out.Input, responsesItem{Role: "assistant", Content: text})
			}
			for _, tc := range m.ToolCalls {
				out.Input = append(out.Input, responsesItem{
					Type: "function_call", CallID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
				})
			}
		case "tool":
			out.Input = append(out.Input, responsesItem{Type: "function_call_output", CallID: m.ToolCallID, Output: m.Text()})
		}
	}
	out.Instructions = strings.Join(instructions, "\n\n")
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, responsesTool{
			Type: "function", Name: t.Function.Name, Description: t.Function.Description, Parameters: t.Function.Parameters,
		})
	}
	if req.MaxTokens > 0 {
		out.MaxOutputTokens = max(req.MaxTokens, 16) // the Responses API rejects very small limits
	}
	return out
}

type responsesResponse struct {
	Status string `json:"status"`
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"output"`
	Usage struct {
		InputTokens        int `json:"input_tokens"`
		OutputTokens       int `json:"output_tokens"`
		InputTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"input_tokens_details"`
		OutputTokensDetails struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
}

func fromResponsesResponse(r responsesResponse) Response {
	var text strings.Builder
	msg := Message{Role: "assistant"}
	for _, item := range r.Output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" {
					text.WriteString(c.Text)
				}
			}
		case "function_call":
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				ID: item.CallID, Type: "function", Function: FunctionCall{Name: item.Name, Arguments: item.Arguments},
			})
		}
	}
	t := text.String()
	msg.Content = &t
	finish := "stop"
	if len(msg.ToolCalls) > 0 {
		finish = "tool_calls"
	} else if r.Status == "incomplete" {
		finish = "length"
	}
	return Response{
		Message:      msg,
		FinishReason: finish,
		Usage: Usage{
			PromptTokens: r.Usage.InputTokens, CachedTokens: r.Usage.InputTokensDetails.CachedTokens,
			CompletionTokens: r.Usage.OutputTokens, ReasoningTokens: r.Usage.OutputTokensDetails.ReasoningTokens,
		},
	}
}

func sendResponses(ctx context.Context, c *Client, key string, req Request) (Response, error) {
	var out responsesResponse
	if err := c.do(ctx, key, req.Session, http.MethodPost, "/responses", nil, toResponsesRequest(req), &out); err != nil {
		return Response{}, err
	}
	return fromResponsesResponse(out), nil
}

// ---- /messages (Anthropic Messages API) ----

// defaultMaxTokens is required by the Messages API; it's an upper bound, not a target.
const defaultMaxTokens = 16384

type anthropicRequest struct {
	Model     string             `json:"model"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	MaxTokens int                `json:"max_tokens"`
}

type anthropicMessage struct {
	Role    string           `json:"role"`
	Content []anthropicBlock `json:"content"`
}

type anthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	Source    *anthropicImage `json:"source,omitempty"`
}

type anthropicImage struct {
	Type      string `json:"type"` // "base64"
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func toAnthropicRequest(req Request) anthropicRequest {
	out := anthropicRequest{Model: req.Model, MaxTokens: req.MaxTokens}
	if out.MaxTokens == 0 {
		out.MaxTokens = defaultMaxTokens
	}
	var system []string
	// The Messages API requires alternating roles, so consecutive blocks for the same role
	// (e.g. several tool results followed by a new user message) are merged.
	add := func(role string, blocks ...anthropicBlock) {
		if len(blocks) == 0 {
			return
		}
		if n := len(out.Messages); n > 0 && out.Messages[n-1].Role == role {
			out.Messages[n-1].Content = append(out.Messages[n-1].Content, blocks...)
			return
		}
		out.Messages = append(out.Messages, anthropicMessage{Role: role, Content: blocks})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			system = append(system, m.Text())
		case "user":
			blocks := []anthropicBlock{{Type: "text", Text: m.Text()}}
			for _, img := range m.Images {
				blocks = append(blocks, anthropicBlock{Type: "image", Source: &anthropicImage{
					Type: "base64", MediaType: img.MediaType, Data: base64.StdEncoding.EncodeToString(img.Data)}})
			}
			add("user", blocks...)
		case "assistant":
			var blocks []anthropicBlock
			if text := m.Text(); text != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: text})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(tc.Function.Arguments)
				if !json.Valid(input) || len(input) == 0 {
					input = json.RawMessage(`{}`)
				}
				blocks = append(blocks, anthropicBlock{Type: "tool_use", ID: tc.ID, Name: tc.Function.Name, Input: input})
			}
			add("assistant", blocks...)
		case "tool":
			add("user", anthropicBlock{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Text()})
		}
	}
	out.System = strings.Join(system, "\n\n")
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, anthropicTool{
			Name: t.Function.Name, Description: t.Function.Description, InputSchema: t.Function.Parameters,
		})
	}
	return out
}

type anthropicResponse struct {
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"` // excludes cache reads and writes
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
}

func fromAnthropicResponse(r anthropicResponse) Response {
	var text strings.Builder
	msg := Message{Role: "assistant"}
	for _, b := range r.Content {
		switch b.Type {
		case "text":
			text.WriteString(b.Text)
		case "tool_use":
			args := string(b.Input)
			if args == "" {
				args = "{}"
			}
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				ID: b.ID, Type: "function", Function: FunctionCall{Name: b.Name, Arguments: args},
			})
		}
	}
	t := text.String()
	msg.Content = &t
	finish := "stop"
	switch r.StopReason {
	case "tool_use":
		finish = "tool_calls"
	case "max_tokens":
		finish = "length"
	}
	return Response{
		Message:      msg,
		FinishReason: finish,
		Usage: Usage{
			PromptTokens:     r.Usage.InputTokens + r.Usage.CacheReadInputTokens + r.Usage.CacheCreationInputTokens,
			CachedTokens:     r.Usage.CacheReadInputTokens,
			CacheWriteTokens: r.Usage.CacheCreationInputTokens,
			CompletionTokens: r.Usage.OutputTokens,
		},
	}
}

func sendAnthropicMessages(ctx context.Context, c *Client, key string, req Request) (Response, error) {
	header := http.Header{
		"X-Api-Key":         {key},
		"Anthropic-Version": {"2023-06-01"},
	}
	var out anthropicResponse
	if err := c.do(ctx, key, req.Session, http.MethodPost, "/messages", header, toAnthropicRequest(req), &out); err != nil {
		return Response{}, err
	}
	return fromAnthropicResponse(out), nil
}

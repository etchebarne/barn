// Package model is a minimal client for OpenAI-compatible chat completion APIs (OpenCode Go).
package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Message is a chat message in OpenAI format. ReasoningContent is a provider extension some
// models return and expect to be echoed back on later requests.
type Message struct {
	Role             string     `json:"role"`
	Content          *string    `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
}

func Text(role, content string) Message { return Message{Role: role, Content: &content} }

func (m Message) Text() string {
	if m.Content == nil {
		return ""
	}
	return *m.Content
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function FunctionSpec `json:"function"`
}

type FunctionSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type Request struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	Tools     []Tool    `json:"tools,omitempty"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type Response struct {
	Message      Message
	FinishReason string
	Usage        Usage
}

// APIError is a non-2xx response from the provider.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("provider returned %d: %s", e.Status, e.Message)
}

// ErrInvalidKey is returned when the provider rejects the API key.
var ErrInvalidKey = errors.New("the provider rejected the API key")

// ErrNoKey is returned when no API key is configured.
var ErrNoKey = errors.New("no API key configured")

// KeyFunc returns the current API key. It's called per request so key changes apply immediately.
type KeyFunc func(ctx context.Context) (string, error)

type Client struct {
	baseURL string
	key     KeyFunc
	http    *http.Client

	modelsMu  sync.Mutex
	models    []string
	modelsExp time.Time
}

func New(baseURL string, key KeyFunc) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		http:    &http.Client{Timeout: 5 * time.Minute},
	}
}

// Chat sends a non-streaming chat completion request.
func (c *Client) Chat(ctx context.Context, req Request) (Response, error) {
	key, err := c.key(ctx)
	if err != nil {
		return Response{}, err
	}
	return c.chat(ctx, key, req)
}

func (c *Client) chat(ctx context.Context, key string, req Request) (Response, error) {
	var out struct {
		Choices []struct {
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}
	if err := c.do(ctx, key, http.MethodPost, "/chat/completions", req, &out); err != nil {
		return Response{}, err
	}
	if len(out.Choices) == 0 {
		return Response{}, errors.New("provider returned no choices")
	}
	ch := out.Choices[0]
	return Response{Message: ch.Message, FinishReason: ch.FinishReason, Usage: out.Usage}, nil
}

// VerifyKey checks a key by sending a minimal request with it. Returns ErrInvalidKey if the
// provider rejects it.
func (c *Client) VerifyKey(ctx context.Context, key string) error {
	models, err := c.Models(ctx)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return errors.New("provider returned no models")
	}
	_, err = c.chat(ctx, key, Request{
		Model:     pickCheapModel(models),
		Messages:  []Message{Text("user", "ok")},
		MaxTokens: 1,
	})
	return err
}

func pickCheapModel(models []string) string {
	for _, m := range models {
		if strings.Contains(m, "flash") {
			return m
		}
	}
	return models[0]
}

// Models lists available model ids. The list is public and cached for 10 minutes.
func (c *Client) Models(ctx context.Context) ([]string, error) {
	c.modelsMu.Lock()
	defer c.modelsMu.Unlock()
	if time.Now().Before(c.modelsExp) {
		return c.models, nil
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.do(ctx, "", http.MethodGet, "/models", nil, &out); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		models = append(models, m.ID)
	}
	c.models, c.modelsExp = models, time.Now().Add(10*time.Minute)
	return models, nil
}

func (c *Client) do(ctx context.Context, key, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrInvalidKey
	}
	if resp.StatusCode/100 != 2 {
		return &APIError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return json.Unmarshal(raw, out)
}

func errorMessage(raw []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	s := strings.TrimSpace(string(raw))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

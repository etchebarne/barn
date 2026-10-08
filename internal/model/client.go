// Package model is a minimal client for OpenAI-compatible chat completion APIs (OpenCode Go).
package model

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/etchebarne/openbot/internal/ids"
)

// Message is a chat message in OpenAI format. ReasoningContent is a provider extension some
// models return and expect to be echoed back on later requests.
type Message struct {
	Role             string     `json:"role"`
	Content          *string    `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	// ImageRefs are attachment ids of images on a user message (kept in the stored context);
	// Images holds their bytes for one request only and is never stored.
	ImageRefs []string `json:"openbot_images,omitempty"`
	Images    []Image  `json:"-"`
}

// Image is an image shown to the model with a user message.
type Image struct {
	MediaType string // e.g. "image/png"
	Data      []byte
}

func (i Image) dataURL() string {
	return "data:" + i.MediaType + ";base64," + base64.StdEncoding.EncodeToString(i.Data)
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
	// Session identifies the conversation; sent as x-opencode-session so the provider can route
	// and cache prompts. Keep it stable for the life of a conversation.
	Session string `json:"-"`

	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	Tools     []Tool    `json:"tools,omitempty"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

// Usage is what a call cost, as the provider reported it.
type Usage struct {
	// PromptTokens is all input, including what was read from or written to the cache.
	PromptTokens int
	// CachedTokens of the input were read from the provider's prompt cache (billed cheaper).
	CachedTokens int
	// CacheWriteTokens of the input were written to the cache (Anthropic-style caching).
	CacheWriteTokens int
	CompletionTokens int
	// ReasoningTokens of the output were the model's hidden reasoning.
	ReasoningTokens int
}

// chatUsage is usage in the Chat Completions format, with the cache fields providers add.
type chatUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens     int `json:"cached_tokens"`
		CacheWriteTokens int `json:"cache_write_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
	// DeepSeek reports cache hits separately.
	PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
	// Some gateways pass Anthropic's fields through.
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

func (u chatUsage) usage() Usage {
	return Usage{
		PromptTokens:     u.PromptTokens,
		CachedTokens:     max(u.PromptTokensDetails.CachedTokens, u.PromptCacheHitTokens, u.CacheReadInputTokens),
		CacheWriteTokens: max(u.PromptTokensDetails.CacheWriteTokens, u.CacheCreationInputTokens),
		CompletionTokens: u.CompletionTokens,
		ReasoningTokens:  u.CompletionTokensDetails.ReasoningTokens,
	}
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

// ErrTrainsOnData is returned when the model's upstream provider trains on request data and
// the OpenCode workspace's privacy settings don't allow that.
var ErrTrainsOnData = errors.New("this model trains on request data, which your OpenCode privacy settings don't allow")

// ErrNoKey is returned when no API key is configured.
var ErrNoKey = errors.New("no API key configured")

// KeyFunc returns the current API key. It's called per request so key changes apply immediately.
type KeyFunc func(ctx context.Context) (string, error)

type Client struct {
	baseURL   string
	userAgent string
	key       KeyFunc
	http      *http.Client

	modelsMu  sync.Mutex
	models    []string
	modelsExp time.Time

	protoMu sync.Mutex
	proto   map[string]*protocol // model id -> protocol that worked

	visionMu sync.Mutex
	noVision map[string]bool // models that rejected images

	// RetryDelays are the waits before retrying a temporary failure (overloaded, rate limited,
	// a dropped connection). Tests shorten them.
	RetryDelays []time.Duration
}

var defaultRetryDelays = []time.Duration{2 * time.Second, 5 * time.Second, 12 * time.Second}

func hasImages(req Request) bool {
	for _, m := range req.Messages {
		if len(m.Images) > 0 {
			return true
		}
	}
	return false
}

func (c *Client) cantSee(model string) bool {
	c.visionMu.Lock()
	defer c.visionMu.Unlock()
	return c.noVision[model]
}

// withoutImages drops the images, noting in each message that the model can't see them.
func withoutImages(req Request) Request {
	msgs := make([]Message, len(req.Messages))
	copy(msgs, req.Messages)
	for i, m := range msgs {
		if len(m.Images) == 0 {
			continue
		}
		text := m.Text() + "\n(You can't see images with this model; the attached images are saved as files at the paths above.)"
		m.Content, m.Images = &text, nil
		msgs[i] = m
	}
	req.Messages = msgs
	return req
}

// transient reports whether a failed model call is worth retrying as is.
func transient(err error) bool {
	var api *APIError
	if errors.As(err, &api) {
		switch api.Status {
		case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout, 529:
			return true
		}
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET)
}

// New creates a client. userAgent identifies openbot to the provider, e.g. "openbot/0.1.0".
func New(baseURL, userAgent string, key KeyFunc) *Client {
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		userAgent: userAgent,
		key:       key,
		http:      &http.Client{Timeout: 5 * time.Minute},
		proto:     map[string]*protocol{},
	}
}

// Chat sends a non-streaming request in whichever API format the model speaks.
func (c *Client) Chat(ctx context.Context, req Request) (Response, error) {
	key, err := c.key(ctx)
	if err != nil {
		return Response{}, err
	}
	delays := c.RetryDelays
	if delays == nil {
		delays = defaultRetryDelays
	}
	if hasImages(req) && c.cantSee(req.Model) {
		req = withoutImages(req)
	}
	for attempt := 0; ; attempt++ {
		resp, err := c.chat(ctx, key, req)
		var api *APIError
		if hasImages(req) && errors.As(err, &api) && api.Status >= 400 && api.Status < 500 &&
			api.Status != http.StatusTooManyRequests && api.Status != http.StatusUnauthorized && api.Status != http.StatusForbidden {
			// Most likely a model that can't take images: try once more without them.
			if resp, err2 := c.chat(ctx, key, withoutImages(req)); err2 == nil {
				c.visionMu.Lock()
				if c.noVision == nil {
					c.noVision = map[string]bool{}
				}
				c.noVision[req.Model] = true
				c.visionMu.Unlock()
				return resp, nil
			}
		}
		if err == nil || attempt >= len(delays) || !transient(err) || ctx.Err() != nil {
			return resp, err
		}
		select {
		case <-time.After(delays[attempt]):
		case <-ctx.Done():
			return resp, err
		}
	}
}

// chat sends req using the model's known or guessed protocol. If the provider says the model
// doesn't support it, the other protocols are tried and the one that works is remembered.
func (c *Client) chat(ctx context.Context, key string, req Request) (Response, error) {
	first := c.protocolFor(req.Model)
	resp, err := first.send(ctx, c, key, req)
	if !errors.Is(err, errWrongProtocol) {
		return resp, err
	}
	for _, p := range protocols {
		if p == first {
			continue
		}
		resp, err = p.send(ctx, c, key, req)
		if errors.Is(err, errWrongProtocol) {
			continue
		}
		if err == nil || !errors.Is(err, ErrInvalidKey) {
			c.rememberProtocol(req.Model, p)
		}
		return resp, err
	}
	return Response{}, fmt.Errorf("%s: no supported API format (tried chat completions, responses, and messages)", req.Model)
}

func (c *Client) protocolFor(model string) *protocol {
	c.protoMu.Lock()
	defer c.protoMu.Unlock()
	if p, ok := c.proto[model]; ok {
		return p
	}
	return guessProtocol(model)
}

func (c *Client) rememberProtocol(model string, p *protocol) {
	c.protoMu.Lock()
	defer c.protoMu.Unlock()
	c.proto[model] = p
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
		Session:   "openbot-verify-" + ids.New(),
		Model:     pickCheapModel(models),
		Messages:  []Message{Text("user", "ok")},
		MaxTokens: 1,
	})
	return err
}

// ProbeModel checks that a model is usable with the configured key by sending a one-token
// request. It returns ErrTrainsOnData if the workspace's privacy settings block the model.
func (c *Client) ProbeModel(ctx context.Context, model string) error {
	_, err := c.Chat(ctx, Request{
		Session:   "openbot-probe-" + ids.New(),
		Model:     model,
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
	if err := c.do(ctx, "", "", http.MethodGet, "/models", nil, nil, &out); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		models = append(models, m.ID)
	}
	c.models, c.modelsExp = models, time.Now().Add(10*time.Minute)
	return models, nil
}

func (c *Client) do(ctx context.Context, key, session, method, path string, header http.Header, body, out any) error {
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
	req.Header.Set("User-Agent", c.userAgent)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	if session != "" {
		req.Header.Set("x-opencode-session", session)
	}
	for k, v := range header {
		req.Header[k] = v
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
	if resp.StatusCode/100 == 2 {
		return json.Unmarshal(raw, out)
	}
	msg := errorMessage(raw)
	switch {
	// Checked before the status code: OpenCode answers a model sent to the wrong API with a
	// 400 or a 401, depending on the endpoint.
	case strings.Contains(msg, "does not support this protocol"), strings.Contains(msg, "is not supported for format"):
		return errWrongProtocol
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrInvalidKey
	case strings.Contains(msg, "trains on request data"):
		return fmt.Errorf("%w (%s)", ErrTrainsOnData, msg)
	default:
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
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

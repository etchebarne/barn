package connectors

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MCP connects to a Model Context Protocol server over Streamable HTTP and exposes its tools.
// Tools the server marks read-only run directly; everything else needs approval.
type MCP struct{}

func (MCP) Name() string        { return "mcp" }
func (MCP) DisplayName() string { return "MCP server" }
func (MCP) Description() string {
	return "Use the tools of any MCP server reachable over HTTP (Streamable HTTP transport)."
}
func (MCP) CredentialFields() []Field {
	return []Field{{Key: "authorization", Label: "Authorization header", Secret: true, Optional: true,
		Help: `Sent as the Authorization header, e.g. "Bearer <token>". Leave empty if the server needs none.`}}
}
func (MCP) ConfigFields() []Field {
	return []Field{{Key: "url", Label: "Server URL", Help: "The server's MCP endpoint, e.g. https://mcp.example.com/mcp"}}
}
func (MCP) SignalTypes() []SignalType { return nil }

const mcpProtocol = "2025-06-18"

type mcpTool struct {
	Name        string          `json:"name"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations struct {
		Title        string `json:"title"`
		ReadOnlyHint bool   `json:"readOnlyHint"`
	} `json:"annotations"`
}

// schemaLabels reads argument titles from a JSON schema's properties.
func schemaLabels(schema json.RawMessage) map[string]string {
	var s struct {
		Properties map[string]struct {
			Title string `json:"title"`
		} `json:"properties"`
	}
	if json.Unmarshal(schema, &s) != nil {
		return nil
	}
	labels := map[string]string{}
	for k, p := range s.Properties {
		if p.Title != "" {
			labels[k] = p.Title
		}
	}
	return labels
}

var mcpCache = struct {
	sync.Mutex
	tools map[string]mcpCacheEntry // account id + url
}{tools: map[string]mcpCacheEntry{}}

type mcpCacheEntry struct {
	tools   []Tool
	expires time.Time
}

// mcpSession is one initialized connection (the server may hand out a session id).
type mcpSession struct {
	url, auth, sessionID string
	nextID               int
}

func (MCP) connect(ctx context.Context, acct Account) (*mcpSession, error) {
	u := strings.TrimSpace(acct.Config["url"])
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		return nil, userErr("the server URL must start with https:// or http://")
	}
	s := &mcpSession{url: u, auth: acct.Credentials["authorization"]}
	if err := mcpInitialize(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

// mcpConn is one MCP transport: Streamable HTTP (mcpSession) or stdio (stdioConn).
type mcpConn interface {
	rpc(ctx context.Context, method string, params any, out any) error
	notify(ctx context.Context, method string) error
}

func mcpInitialize(ctx context.Context, c mcpConn) error {
	var init map[string]any
	if err := c.rpc(ctx, "initialize", map[string]any{
		"protocolVersion": mcpProtocol,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": "barn", "version": "1"},
	}, &init); err != nil {
		return err
	}
	_ = c.notify(ctx, "notifications/initialized")
	return nil
}

func mcpListTools(ctx context.Context, c mcpConn) ([]Tool, error) {
	var out struct {
		Tools []mcpTool `json:"tools"`
	}
	if err := c.rpc(ctx, "tools/list", map[string]any{}, &out); err != nil {
		return nil, err
	}
	tools := make([]Tool, 0, len(out.Tools))
	for _, t := range out.Tools {
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		title := t.Title
		if title == "" {
			title = t.Annotations.Title
		}
		tools = append(tools, Tool{Name: t.Name, Description: truncateStr(t.Description, 1000), Parameters: schema,
			External: !t.Annotations.ReadOnlyHint, Title: title, Labels: schemaLabels(schema)})
	}
	return tools, nil
}

func mcpCallTool(ctx context.Context, c mcpConn, tool string, args json.RawMessage) (any, error) {
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := c.rpc(ctx, "tools/call", map[string]any{"name": tool, "arguments": args}, &out); err != nil {
		return nil, err
	}
	var text []string
	for _, c := range out.Content {
		if c.Type == "text" {
			text = append(text, c.Text)
		} else {
			text = append(text, "["+c.Type+" content]")
		}
	}
	joined := truncateStr(strings.Join(text, "\n"), 32000)
	if out.IsError {
		return nil, userErr("%s", joined)
	}
	return map[string]string{"content": joined}, nil
}

// mcpResponse decodes a JSON-RPC response for request id.
func mcpResponse(method string, raw []byte, out any) error {
	var msg struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return fmt.Errorf("MCP %s: %w", method, err)
	}
	if msg.Error != nil {
		return userErr("MCP %s: %s", method, msg.Error.Message)
	}
	return json.Unmarshal(msg.Result, out)
}

func (s *mcpSession) post(ctx context.Context, msg map[string]any) (*http.Response, error) {
	b, _ := json.Marshal(msg)
	req, err := http.NewRequestWithContext(ctx, "POST", s.url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", mcpProtocol)
	if s.auth != "" {
		req.Header.Set("Authorization", s.auth)
	}
	if s.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", s.sessionID)
	}
	return httpClient.Do(req)
}

func (s *mcpSession) notify(ctx context.Context, method string) error {
	resp, err := s.post(ctx, map[string]any{"jsonrpc": "2.0", "method": method})
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// rpc sends a request and reads the matching response, whether the server answers with JSON or
// with a server-sent event stream.
func (s *mcpSession) rpc(ctx context.Context, method string, params any, out any) error {
	s.nextID++
	id := s.nextID
	resp, err := s.post(ctx, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		s.sessionID = sid
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return userErr("the MCP server rejected the credentials (%d)", resp.StatusCode)
	}
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("MCP %s: %d: %s", method, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var msg struct {
		ID     int             `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		// An event's data may span several "data:" lines; they're joined with newlines and the
		// event ends at a blank line.
		found := false
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64<<10), 8<<20)
		var data []string
		for sc.Scan() && !found {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "data:"):
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			case line == "":
				if len(data) > 0 && json.Unmarshal([]byte(strings.Join(data, "\n")), &msg) == nil && msg.ID == id {
					found = true
				}
				data = nil
			}
		}
		if !found && len(data) > 0 && json.Unmarshal([]byte(strings.Join(data, "\n")), &msg) == nil && msg.ID == id {
			found = true // stream ended without a trailing blank line
		}
		if !found {
			return fmt.Errorf("MCP %s: no response in the event stream", method)
		}
	} else if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&msg); err != nil {
		return fmt.Errorf("MCP %s: %w", method, err)
	}
	if msg.Error != nil {
		return userErr("MCP %s: %s", method, msg.Error.Message)
	}
	return json.Unmarshal(msg.Result, out)
}

func (m MCP) Tools(ctx context.Context, acct Account) ([]Tool, error) {
	// Keyed by credentials too, so changing them never serves tools listed with the old ones.
	sum := sha256.Sum256([]byte(acct.Credentials["authorization"]))
	key := acct.ID + "|" + acct.Config["url"] + "|" + hex.EncodeToString(sum[:8])
	mcpCache.Lock()
	entry, ok := mcpCache.tools[key]
	mcpCache.Unlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.tools, nil
	}
	tools, err := m.listTools(ctx, acct)
	if err != nil {
		return nil, err
	}
	mcpCache.Lock()
	mcpCache.tools[key] = mcpCacheEntry{tools: tools, expires: time.Now().Add(5 * time.Minute)}
	mcpCache.Unlock()
	return tools, nil
}

// listTools asks the server for its tools (uncached).
func (m MCP) listTools(ctx context.Context, acct Account) ([]Tool, error) {
	s, err := m.connect(ctx, acct)
	if err != nil {
		return nil, err
	}
	return mcpListTools(ctx, s)
}

func (m MCP) Call(ctx context.Context, acct Account, tool string, args json.RawMessage) (any, error) {
	s, err := m.connect(ctx, acct)
	if err != nil {
		return nil, err
	}
	return mcpCallTool(ctx, s, tool, args)
}

func (m MCP) Verify(ctx context.Context, acct Account) error {
	// Always ask the server: a cached list would let wrong credentials pass.
	_, err := m.listTools(ctx, acct)
	return err
}

package connectors

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"
)

// MCPLocal runs an MCP server that talks over stdio (the usual `npx …` / `uvx …` servers). barn
// runs it in a container of its own, apart from agents' sandboxes, so agents can't read the
// secrets in its environment.
type MCPLocal struct{}

func (MCPLocal) Name() string        { return "mcp_local" }
func (MCPLocal) DisplayName() string { return "Local MCP server" }
func (MCPLocal) Description() string {
	return "Run an MCP server that talks over stdio, like `npx -y @modelcontextprotocol/server-memory`, in barn's MCP container."
}
func (MCPLocal) CredentialFields() []Field {
	return []Field{{Key: "env", Label: "Environment variables", Secret: true, Optional: true,
		Help: "Secrets the server reads, as KEY=value pairs separated by spaces, e.g. GITHUB_TOKEN=ghp_… (values can't contain spaces)."}}
}
func (MCPLocal) ConfigFields() []Field {
	return []Field{{Key: "command", Label: "Command",
		Help: "The command that starts the server, e.g. npx -y @modelcontextprotocol/server-memory. It runs in a Debian container with Node.js, Python and uv."}}
}
func (MCPLocal) SignalTypes() []SignalType { return nil }

// VerifyTimeout leaves time for npx/uvx to download the server on first run.
func (MCPLocal) VerifyTimeout() time.Duration { return 2 * time.Minute }

// LocalProcess is a running server process: write requests to it, read responses from it.
type LocalProcess interface {
	io.ReadWriter
	Stderr() string
	Close() error
	Done() <-chan struct{}
}

// LocalRunner starts a command with env in barn's MCP container.
type LocalRunner func(ctx context.Context, command string, env map[string]string) (LocalProcess, error)

var localRunner struct {
	sync.Mutex
	run LocalRunner
}

// SetLocalRunner enables local MCP servers (barn sets it when Docker is available).
func SetLocalRunner(r LocalRunner) {
	localRunner.Lock()
	defer localRunner.Unlock()
	localRunner.run = r
}

func runner() (LocalRunner, error) {
	localRunner.Lock()
	defer localRunner.Unlock()
	if localRunner.run == nil {
		return nil, userErr("local MCP servers need Docker on the barn server (it isn't available)")
	}
	return localRunner.run, nil
}

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func parseEnv(s string) (map[string]string, error) {
	env := map[string]string{}
	for _, pair := range strings.Fields(s) {
		k, v, ok := strings.Cut(pair, "=")
		if !ok || !envKeyRe.MatchString(k) {
			return nil, userErr("environment variables must look like KEY=value (got %q)", strings.SplitN(pair, "=", 2)[0])
		}
		env[k] = v
	}
	return env, nil
}

func (MCPLocal) spec(acct Account) (string, map[string]string, error) {
	command := strings.TrimSpace(acct.Config["command"])
	if command == "" {
		return "", nil, userErr("the command is required")
	}
	env, err := parseEnv(acct.Credentials["env"])
	return command, env, err
}

// start launches the server and runs the MCP handshake.
func (m MCPLocal) start(ctx context.Context, acct Account) (*stdioConn, error) {
	run, err := runner()
	if err != nil {
		return nil, err
	}
	command, env, err := m.spec(acct)
	if err != nil {
		return nil, err
	}
	proc, err := run(ctx, command, env)
	if err != nil {
		return nil, fmt.Errorf("starting the server: %w", err)
	}
	c := newStdioConn(proc)
	if err := mcpInitialize(ctx, c); err != nil {
		c.close()
		return nil, err
	}
	return c, nil
}

func (m MCPLocal) Verify(ctx context.Context, acct Account) error {
	c, err := m.start(ctx, acct)
	if err != nil {
		return err
	}
	defer c.close()
	_, err = mcpListTools(ctx, c)
	return err
}

func (m MCPLocal) Tools(ctx context.Context, acct Account) ([]Tool, error) {
	// Tools are listed every turn, so cache them for long: a server that's idle can then stay
	// stopped until an agent actually calls it.
	key := "local|" + localKey(acct)
	mcpCache.Lock()
	entry, ok := mcpCache.tools[key]
	mcpCache.Unlock()
	if ok && time.Now().Before(entry.expires) {
		return entry.tools, nil
	}
	c, err := m.conn(ctx, acct)
	if err != nil {
		return nil, err
	}
	tools, err := mcpListTools(ctx, c)
	if err != nil {
		return nil, err
	}
	mcpCache.Lock()
	mcpCache.tools[key] = mcpCacheEntry{tools: tools, expires: time.Now().Add(time.Hour)}
	mcpCache.Unlock()
	return tools, nil
}

func (m MCPLocal) Call(ctx context.Context, acct Account, tool string, args json.RawMessage) (any, error) {
	c, err := m.conn(ctx, acct)
	if err != nil {
		return nil, err
	}
	return mcpCallTool(ctx, c, tool, args)
}

// ---------------------------------------------------------------- process pool

// Running servers are kept per account (and command + env, so edits start a fresh one) and
// stopped after a while without calls.
const localIdle = 10 * time.Minute

var localPool = struct {
	sync.Mutex
	conns  map[string]*stdioConn
	reaper bool
}{conns: map[string]*stdioConn{}}

func localKey(acct Account) string {
	h := sha256.Sum256([]byte(acct.Config["command"] + "\x00" + acct.Credentials["env"]))
	return acct.ID + "|" + hex.EncodeToString(h[:8])
}

func (m MCPLocal) conn(ctx context.Context, acct Account) (*stdioConn, error) {
	key := localKey(acct)
	localPool.Lock()
	if c, ok := localPool.conns[key]; ok && c.alive() {
		c.touch()
		localPool.Unlock()
		return c, nil
	}
	if !localPool.reaper {
		localPool.reaper = true
		go reapLocal()
	}
	localPool.Unlock()

	c, err := m.start(ctx, acct)
	if err != nil {
		return nil, err
	}
	localPool.Lock()
	defer localPool.Unlock()
	if old, ok := localPool.conns[key]; ok && old.alive() {
		c.close() // someone else started it meanwhile
		old.touch()
		return old, nil
	}
	localPool.conns[key] = c
	return c, nil
}

func reapLocal() {
	for range time.Tick(time.Minute) {
		localPool.Lock()
		for key, c := range localPool.conns {
			if !c.alive() || c.idleFor() > localIdle {
				go c.close()
				delete(localPool.conns, key)
			}
		}
		localPool.Unlock()
	}
}

// CloseLocal stops an account's running servers (when it's edited or deleted).
func CloseLocal(accountID string) {
	localPool.Lock()
	defer localPool.Unlock()
	for key, c := range localPool.conns {
		if strings.HasPrefix(key, accountID+"|") {
			go c.close()
			delete(localPool.conns, key)
		}
	}
}

// ---------------------------------------------------------------- stdio transport

// stdioConn speaks newline-delimited JSON-RPC with a server process, one request at a time.
type stdioConn struct {
	proc      LocalProcess
	mu        sync.Mutex // one request at a time
	writeMu   sync.Mutex // requests and our replies to the server's own requests
	nextID    int
	responses chan json.RawMessage
	dead      chan struct{}
	used      struct {
		sync.Mutex
		at time.Time
	}
	closeOnce sync.Once
}

func newStdioConn(proc LocalProcess) *stdioConn {
	c := &stdioConn{proc: proc, responses: make(chan json.RawMessage, 16), dead: make(chan struct{})}
	c.touch()
	go c.read()
	return c
}

func (c *stdioConn) read() {
	defer close(c.dead)
	sc := bufio.NewScanner(c.proc)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		line := sc.Bytes()
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(line, &msg) != nil {
			continue // servers sometimes print logs to stdout
		}
		switch {
		case msg.Method != "" && len(msg.ID) > 0:
			// A request from the server (ping, roots/list, sampling…). barn only answers ping.
			reply := map[string]any{"jsonrpc": "2.0", "id": msg.ID}
			if msg.Method == "ping" {
				reply["result"] = map[string]any{}
			} else {
				reply["error"] = map[string]any{"code": -32601, "message": "barn doesn't support " + msg.Method}
			}
			// Reply from elsewhere: the server may be busy writing to us and not reading yet.
			go func() { _ = c.write(reply) }()
		case msg.Method != "":
			// A notification (logging, progress, list changed): nothing to do.
		default:
			c.responses <- append(json.RawMessage(nil), line...)
		}
	}
}

func (c *stdioConn) write(msg any) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.proc.Write(append(b, '\n'))
	return err
}

func (c *stdioConn) rpc(ctx context.Context, method string, params any, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.touch()
	c.nextID++
	id := c.nextID
	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return c.stopped(method)
	}
	for {
		select {
		case raw := <-c.responses:
			var got struct {
				ID int `json:"id"`
			}
			if json.Unmarshal(raw, &got) != nil || got.ID != id {
				continue // a late answer to a request that timed out
			}
			return mcpResponse(method, raw, out)
		case <-c.dead:
			return c.stopped(method)
		case <-ctx.Done():
			// The server is stuck; don't let the next call wait behind it.
			go c.close()
			return fmt.Errorf("MCP %s: the server didn't answer in time", method)
		}
	}
}

func (c *stdioConn) notify(_ context.Context, method string) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method})
}

// stopped describes a server that exited, with the end of what it printed to stderr.
func (c *stdioConn) stopped(method string) error {
	msg := "the server stopped"
	if tail := strings.TrimSpace(c.proc.Stderr()); tail != "" {
		if len(tail) > 600 {
			tail = "…" + tail[len(tail)-600:]
		}
		msg += ": " + tail
	}
	return userErr("MCP %s: %s", method, msg)
}

func (c *stdioConn) alive() bool {
	select {
	case <-c.dead:
		return false
	default:
		return true
	}
}

func (c *stdioConn) touch() {
	c.used.Lock()
	c.used.at = time.Now()
	c.used.Unlock()
}

func (c *stdioConn) idleFor() time.Duration {
	c.used.Lock()
	defer c.used.Unlock()
	return time.Since(c.used.at)
}

func (c *stdioConn) close() {
	c.closeOnce.Do(func() { _ = c.proc.Close() })
}

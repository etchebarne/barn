package connectors

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

// fakeProc is an in-memory stdio MCP server.
type fakeProc struct {
	in       *io.PipeWriter // what openbot writes
	out      *io.PipeReader // what openbot reads
	stderr   string
	done     chan struct{}
	once     sync.Once
	closed   bool
	closedMu sync.Mutex
}

func (p *fakeProc) Write(b []byte) (int, error) { return p.in.Write(b) }
func (p *fakeProc) Read(b []byte) (int, error)  { return p.out.Read(b) }
func (p *fakeProc) Stderr() string              { return p.stderr }
func (p *fakeProc) Done() <-chan struct{}       { return p.done }
func (p *fakeProc) Close() error {
	p.once.Do(func() {
		p.closedMu.Lock()
		p.closed = true
		p.closedMu.Unlock()
		p.in.Close()
	})
	return nil
}

// startFake runs a server that pings openbot once before answering each tools/call, and exits
// with stderr output when asked to call "crash".
func startFake(t *testing.T, env map[string]string) *fakeProc {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	p := &fakeProc{in: inW, out: outR, done: make(chan struct{})}
	go func() {
		defer close(p.done)
		defer outW.Close()
		sc := bufio.NewScanner(inR)
		send := func(v any) { b, _ := json.Marshal(v); outW.Write(append(b, '\n')) }
		pinged := 0
		for sc.Scan() {
			var msg struct {
				ID     *int            `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
				Result json.RawMessage `json:"result"`
			}
			json.Unmarshal(sc.Bytes(), &msg)
			if msg.Method == "" { // our ping's answer
				pinged++
				continue
			}
			if msg.ID == nil {
				continue
			}
			fmt.Fprintln(outW, "some log line that isn't JSON")
			var result any
			switch msg.Method {
			case "initialize":
				result = map[string]any{"protocolVersion": mcpProtocol, "capabilities": map[string]any{}}
			case "tools/list":
				result = map[string]any{"tools": []map[string]any{
					{"name": "remember", "description": "Store a note", "inputSchema": map[string]any{"type": "object"}},
					{"name": "recall", "description": "Read notes", "annotations": map[string]any{"readOnlyHint": true}},
				}}
			case "tools/call":
				var c struct {
					Name string `json:"name"`
				}
				json.Unmarshal(msg.Params, &c)
				if c.Name == "crash" {
					p.stderr = "Error: Cannot find module 'left-pad'"
					return
				}
				send(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "ping"})
				result = map[string]any{"content": []map[string]any{{"type": "text", "text": "token=" + env["TOKEN"]}}}
			}
			send(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": result})
		}
		_ = pinged
	}()
	return p
}

func TestMCPLocal(t *testing.T) {
	ctx := context.Background()
	m := MCPLocal{}
	acct := Account{ID: "local1", Config: map[string]string{"command": "npx -y fake-server"},
		Credentials: map[string]string{"env": "TOKEN=abc DEBUG=1"}}

	SetLocalRunner(nil)
	if err := m.Verify(ctx, acct); err == nil || !strings.Contains(err.Error(), "need Docker") {
		t.Fatalf("without Docker: %v", err)
	}

	var mu sync.Mutex
	var procs []*fakeProc
	SetLocalRunner(func(_ context.Context, command string, env map[string]string) (LocalProcess, error) {
		if command != "npx -y fake-server" || env["TOKEN"] != "abc" || env["DEBUG"] != "1" {
			t.Errorf("command %q env %v", command, env)
		}
		p := startFake(t, env)
		mu.Lock()
		procs = append(procs, p)
		mu.Unlock()
		return p, nil
	})
	t.Cleanup(func() { SetLocalRunner(nil); CloseLocal("local1") })
	started := func() int { mu.Lock(); defer mu.Unlock(); return len(procs) }

	if err := m.Verify(ctx, acct); err != nil {
		t.Fatal(err)
	}
	if started() != 1 || !procs[0].closed {
		t.Fatal("verify should start a server and stop it again")
	}

	tools, err := m.Tools(ctx, acct)
	if err != nil || len(tools) != 2 || !tools[0].External || tools[1].External {
		t.Fatalf("tools = %+v %v", tools, err)
	}
	out, err := m.Call(ctx, acct, "remember", json.RawMessage(`{}`))
	if err != nil || out.(map[string]string)["content"] != "token=abc" {
		t.Fatalf("call = %v %v", out, err)
	}
	m.Tools(ctx, acct) // cached
	if started() != 2 {
		t.Fatalf("listing and calling should share one running server, started %d", started())
	}

	// A crash reports the server's stderr, and the next call starts it again.
	if _, err := m.Call(ctx, acct, "crash", nil); err == nil || !strings.Contains(err.Error(), "Cannot find module") {
		t.Fatalf("crash: %v", err)
	}
	if _, err := m.Call(ctx, acct, "remember", nil); err != nil || started() != 3 {
		t.Fatalf("restart: %v (started %d)", err, started())
	}

	// Editing or deleting the account stops its server.
	CloseLocal("local1")
	mu.Lock()
	last := procs[len(procs)-1]
	mu.Unlock()
	<-last.done
	if !last.closed {
		t.Fatal("CloseLocal should stop the server")
	}

	for _, bad := range []string{"lower case=1", "NOVALUE", "1X=2"} {
		if _, err := parseEnv(bad); err == nil {
			t.Errorf("parseEnv(%q) should fail", bad)
		}
	}
}

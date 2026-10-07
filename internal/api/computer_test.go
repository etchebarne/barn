package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/etchebarne/openbot/internal/runtime"
	"github.com/etchebarne/openbot/internal/sandbox"
	"github.com/etchebarne/openbot/internal/store"
)

func TestComputerUnavailable(t *testing.T) {
	c, st := setupWithKey(t)
	a, _, _ := st.CreateAgentWithDM(context.Background(), store.Agent{Name: "a", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})
	if resp, _ := c.do("GET", "/api/agents/"+a.ID+"/sandbox/files?path=/home/agent", "", false); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("without Docker: %d", resp.StatusCode)
	}
}

// Integration test against the local Docker daemon; skipped without Docker or with -short.
func TestComputer(t *testing.T) {
	if testing.Short() {
		t.Skip("needs Docker")
	}
	sbx := sandbox.New(sandbox.Options{Image: "debian:bookworm-slim", Memory: "256m", CPUs: "1"})
	if !sbx.Available() {
		t.Skip("Docker isn't available")
	}
	ts, st := newTestServerWith(t, fakeProvider(t).URL, func(rt *runtime.Manager) { rt.Sandboxes = sbx })
	c := newClient(t, ts)
	c.do("POST", "/api/auth/setup", `{"username":"martin","password":"a long enough password"}`, true)
	ctx := context.Background()
	a, _, _ := st.CreateAgentWithDM(ctx, store.Agent{Name: "a", Instructions: "x", Model: "model-a", Language: "auto", TrustMode: "ask"})
	t.Cleanup(func() {
		if id, err := st.SandboxFor(context.Background(), a.ID); err == nil {
			sbx.Remove(context.Background(), id)
		}
	})
	base := "/api/agents/" + a.ID + "/sandbox"

	put := func(path, body string) int {
		req, _ := http.NewRequest("PUT", c.base+base+"/files?path="+path, strings.NewReader(body))
		req.Header.Set("X-Openbot-CSRF", "1")
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := c.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := put("/home/agent/docs/hello.txt", "hi there"); code != http.StatusNoContent {
		t.Fatalf("upload: %d", code)
	}
	if resp, _ := c.do("POST", base+"/folders", `{"path":"/home/agent/aaa"}`, true); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("mkdir: %d", resp.StatusCode)
	}
	resp, listing := c.do("GET", base+"/files?path=/home/agent/", "", false)
	entries, _ := listing["entries"].([]any)
	if resp.StatusCode != http.StatusOK || listing["path"] != "/home/agent" || len(entries) < 2 {
		t.Fatalf("list: %d %v", resp.StatusCode, listing)
	}
	// Folders come first, by name.
	if first := entries[0].(map[string]any); first["name"] != "aaa" || first["kind"] != "dir" {
		t.Fatalf("order: %v", entries)
	}
	if resp, _ := c.do("GET", base+"/files?path=/nope", "", false); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing folder: %d", resp.StatusCode)
	}

	// Download: text is never served as a page.
	dl, err := c.http.Get(c.base + base + "/files/download?path=/home/agent/docs/hello.txt&inline=true")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(dl.Body)
	dl.Body.Close()
	if string(body) != "hi there" || dl.Header.Get("Content-Type") != "application/octet-stream" ||
		!strings.HasPrefix(dl.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("download: %q %v", body, dl.Header)
	}

	if resp, _ := c.do("POST", base+"/move", `{"from":"/home/agent/docs/hello.txt","to":"/home/agent/aaa/hello.txt"}`, true); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("move: %d", resp.StatusCode)
	}
	put("/home/agent/docs/again.txt", "x")
	if resp, _ := c.do("POST", base+"/move", `{"from":"/home/agent/docs/again.txt","to":"/home/agent/aaa/hello.txt"}`, true); resp.StatusCode != http.StatusConflict {
		t.Fatalf("move over a file: %d", resp.StatusCode)
	}
	if resp, _ := c.do("DELETE", base+"/files?path=/home/agent/aaa", "", true); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	if resp, _ := c.do("DELETE", base+"/files?path=relative", "", true); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("relative path: %d", resp.StatusCode)
	}

	// Terminal: same-origin WebSocket with the session cookie.
	wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	u := strings.Replace(c.base, "http", "ws", 1) + base + "/terminal?cols=100&rows=30"
	header := http.Header{"Origin": {c.base}}
	conn, _, err := websocket.Dial(wctx, u, &websocket.DialOptions{HTTPClient: c.http, HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	conn.Write(wctx, websocket.MessageText, []byte(`{"type":"resize","cols":120,"rows":40}`))
	time.Sleep(300 * time.Millisecond)
	conn.Write(wctx, websocket.MessageBinary, []byte("stty size; echo done-$((1+1))\n"))
	var out strings.Builder
	for !strings.Contains(out.String(), "done-2") {
		_, data, err := conn.Read(wctx)
		if err != nil {
			t.Fatalf("terminal: %v (got %q)", err, out.String())
		}
		out.Write(data)
	}
	if !strings.Contains(out.String(), "40 120") {
		t.Fatalf("resize didn't apply: %q", out.String())
	}
	conn.Write(wctx, websocket.MessageBinary, []byte("exit\n"))
	for {
		if _, _, err := conn.Read(wctx); err != nil {
			if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
				t.Fatalf("expected a normal close when the shell exits: %v", err)
			}
			break
		}
	}

	// Other sites can't open a terminal.
	_, _, err = websocket.Dial(wctx, u, &websocket.DialOptions{HTTPClient: c.http, HTTPHeader: http.Header{"Origin": {"https://evil.example"}}})
	if err == nil {
		t.Fatal("a cross-origin terminal must be refused")
	}
}

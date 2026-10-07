package runtime

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/sandbox"
	"github.com/etchebarne/barn/internal/store"
)

type fakeSandbox struct {
	mu    sync.Mutex
	calls []string
	stdin []string
}

func (f *fakeSandbox) Available() bool                       { return true }
func (f *fakeSandbox) Status(context.Context, string) string { return "running" }
func (f *fakeSandbox) Restart(context.Context, string) error { return nil }
func (f *fakeSandbox) Exec(_ context.Context, id, command, workdir string, stdin []byte, timeout time.Duration) (sandbox.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, id+"|"+workdir+"|"+command+"|"+timeout.String())
	f.stdin = append(f.stdin, string(stdin))
	return sandbox.Result{Output: "ok\n"}, nil
}

func TestSandboxTools(t *testing.T) {
	var f fixture
	f = setup(t,
		func(req model.Request) model.Message {
			names := map[string]bool{}
			for _, tool := range req.Tools {
				names[tool.Function.Name] = true
			}
			for _, n := range []string{toolRunCommand, toolReadFile, toolWriteFile, toolListFiles} {
				if !names[n] {
					t.Errorf("expected tool %s when sandboxes are available", n)
				}
			}
			if !strings.Contains(req.Messages[0].Text(), "# Your computer") {
				t.Errorf("system prompt should describe the computer")
			}
			m := toolCall(toolRunCommand, map[string]any{"command": "npm test", "timeout_seconds": 5000})
			m.ToolCalls = append(m.ToolCalls,
				toolCall(toolWriteFile, map[string]string{"path": "it's here/a.txt", "content": "hello"}).ToolCalls[0])
			m.ToolCalls[1].ID = "call_2"
			return m
		},
		func(model.Request) model.Message { return model.Text("assistant", "") },
	)
	sbx := &fakeSandbox{}
	f.rt.Sandboxes = sbx
	f.userSays(t, "run the tests")
	f.waitIdle(t)

	sbx.mu.Lock()
	defer sbx.mu.Unlock()
	if len(sbx.calls) != 2 {
		t.Fatalf("expected 2 sandbox calls, got %v", sbx.calls)
	}
	run := strings.Split(sbx.calls[0], "|")
	if run[1] != "/home/agent" || run[2] != "npm test" || run[3] != maxCommandTimeout.String() {
		t.Fatalf("run_command: %v (timeouts are capped)", run)
	}
	write := strings.Split(sbx.calls[1], "|")
	if write[2] != `mkdir -p -- '/home/agent/it'\''s here' && cat > '/home/agent/it'\''s here/a.txt'` || sbx.stdin[1] != "hello" {
		t.Fatalf("write_file should quote paths and stream content on stdin: %v %q", write, sbx.stdin[1])
	}
	a, _ := f.store.GetAgent(context.Background(), f.agent.ID)
	if a.SandboxID == nil || !strings.HasPrefix(sbx.calls[0], *a.SandboxID+"|") {
		t.Fatal("the agent should get its own sandbox on first use")
	}
}

func TestNoSandboxTools(t *testing.T) {
	f := setup(t, func(req model.Request) model.Message {
		for _, tool := range req.Tools {
			if tool.Function.Name == toolRunCommand {
				t.Error("no run_command without sandboxes")
			}
		}
		if !strings.Contains(req.Messages[0].Text(), "sandboxes aren't set up") {
			t.Error("the agent should be told it has no computer")
		}
		return model.Text("assistant", "")
	})
	f.userSays(t, "hi")
	f.waitIdle(t)
}

func TestSharedSandboxes(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	other, _, _ := f.store.CreateAgentWithDM(ctx, store.Agent{Name: "Builder", Instructions: "x", Model: "test-model", Language: "auto", TrustMode: "ask"})
	if err := f.store.ShareSandbox(ctx, other.ID, f.agent.ID); err != nil {
		t.Fatal(err)
	}
	a, _ := f.store.GetAgent(ctx, f.agent.ID)
	b, _ := f.store.GetAgent(ctx, other.ID)
	if a.SandboxID == nil || b.SandboxID == nil || *a.SandboxID != *b.SandboxID {
		t.Fatalf("expected one shared sandbox, got %v %v", a.SandboxID, b.SandboxID)
	}
	f.rt.Sandboxes = &fakeSandbox{}
	_, shared, err := f.rt.SandboxState(ctx, f.agent.ID)
	if err != nil || len(shared) != 1 || shared[0] != other.ID {
		t.Fatalf("SandboxState shared = %v, %v", shared, err)
	}
}

// End to end against real Docker: an agent runs a command and sees the output.
func TestSandboxEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("needs Docker")
	}
	sbx := sandbox.New(sandbox.Options{Image: "debian:bookworm-slim", SharedDir: t.TempDir(), Memory: "256m", CPUs: "1"})
	if !sbx.Available() {
		t.Skip("Docker isn't available")
	}
	var f fixture
	var mu sync.Mutex
	var output string
	f = setup(t,
		func(model.Request) model.Message {
			return toolCall(toolRunCommand, map[string]string{"command": "echo $((6*7)) && uname -s"})
		},
		func(req model.Request) model.Message {
			mu.Lock()
			output = req.Messages[len(req.Messages)-1].Text()
			mu.Unlock()
			return model.Text("assistant", "")
		},
	)
	result := func() string {
		mu.Lock()
		defer mu.Unlock()
		return output
	}
	f.rt.Sandboxes = sbx
	t.Cleanup(func() {
		if a, _ := f.store.GetAgent(context.Background(), f.agent.ID); a.SandboxID != nil {
			sbx.Remove(context.Background(), *a.SandboxID)
		}
	})
	f.userSays(t, "what's 6*7?")
	deadline := time.Now().Add(90 * time.Second)
	for result() == "" && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if out := result(); !strings.Contains(out, `42\nLinux`) || !strings.Contains(out, `"exit_code":0`) {
		t.Fatalf("unexpected tool result: %s", out)
	}
}

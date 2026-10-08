package runtime

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/sandbox"
)

func TestDelegate(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	running, maxRunning := 0, 0
	f := setup(t)
	f.rt.Sandboxes = &fakeSandbox{exec: func(command string, env map[string]string) sandbox.Result {
		return sandbox.Result{Output: "found 3 TODOs in " + command + "\n"}
	}}
	var parentResult string
	f.llm.concurrent = true
	both := make(chan struct{})
	var bothOnce sync.Once
	f.llm.handler = func(req model.Request) model.Message {
		sys := req.Messages[0].Text()
		last := req.Messages[len(req.Messages)-1]
		if strings.HasPrefix(sys, "You are a helper working for") {
			job := req.Messages[1].Text()
			switch last.Role {
			case "user":
				mu.Lock()
				running++
				maxRunning = max(maxRunning, running)
				if running == 2 {
					bothOnce.Do(func() { close(both) })
				}
				mu.Unlock()
				// Wait (briefly) for the other helper: they should be running at the same time.
				select {
				case <-both:
				case <-time.After(3 * time.Second):
				}
				// Helpers can't message anyone.
				m := toolCall(toolRunCommand, map[string]string{"command": "grep -rn TODO " + job})
				m.ToolCalls = append(m.ToolCalls, sendCall("x", "hi").ToolCalls...)
				m.ToolCalls[1].ID = "send"
				return m
			default:
				mu.Lock()
				running--
				mu.Unlock()
				if !strings.Contains(req.Messages[len(req.Messages)-1].Text(), "helpers can't use send_message") {
					t.Errorf("a helper's send_message should be refused: %s", req.Messages[len(req.Messages)-1].Text())
				}
				return model.Text("assistant", "Report for "+job+": 3 TODOs.")
			}
		}
		switch {
		case last.Role == "user" && strings.Contains(last.Text(), "audit"):
			m := toolCall(toolDelegate, map[string]string{"task": "src/"})
			m.ToolCalls = append(m.ToolCalls, toolCall(toolDelegate, map[string]string{"task": "docs/"}).ToolCalls...)
			m.ToolCalls[1].ID = "call_2"
			return m
		case last.Role == "tool":
			mu.Lock()
			parentResult = last.Text()
			mu.Unlock()
		}
		return model.Text("assistant", "")
	}
	f.userSays(t, "audit the repo")
	f.waitIdle(t)

	mu.Lock()
	if maxRunning != 2 {
		t.Errorf("two delegate calls in one step should run in parallel (max running: %d)", maxRunning)
	}
	mu.Unlock()
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(parentResult, "Report for docs/: 3 TODOs.") {
		t.Fatalf("the parent should get the helper's report: %s", parentResult)
	}
	// The helpers' steps stay out of the parent's context.
	entries, _ := f.store.Context(ctx, f.agent.ID)
	for _, e := range entries {
		if strings.Contains(string(e.Entry), "grep -rn TODO") {
			t.Fatalf("a helper's steps leaked into the parent's context: %s", e.Entry)
		}
	}
	usage, _ := f.store.UsageSince(ctx, f.agent.ID, 0)
	helpers := 0
	for _, u := range usage {
		if u.Purpose == "subagent" {
			helpers++
		}
	}
	if helpers != 4 {
		t.Fatalf("expected 4 helper calls (2 helpers × 2 steps), got %d", helpers)
	}
}

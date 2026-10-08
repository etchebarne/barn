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

func TestTaskChecks(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	status := "all good"
	f := setup(t)
	f.rt.Timezone = func(context.Context) *time.Location { return time.UTC }
	f.rt.Sandboxes = &fakeSandbox{exec: func(command string, env map[string]string) sandbox.Result {
		mu.Lock()
		defer mu.Unlock()
		return sandbox.Result{Output: status + "\n"}
	}}
	var woken []string
	f.llm.handler = func(req model.Request) model.Message {
		last := req.Messages[len(req.Messages)-1]
		if last.Role == "user" && strings.Contains(last.Text(), "<task_fired") {
			mu.Lock()
			woken = append(woken, last.Text())
			mu.Unlock()
			return toolCall(toolDone, map[string]string{})
		}
		return model.Text("assistant", "")
	}

	check := "curl -s https://status.example.com"
	cron := "* * * * *"
	name, purpose := "Status watch", "Tell Martin when the status changes."
	task, err := f.rt.SaveTask(ctx, f.agent.ID, "", TaskChange{Name: &name, Purpose: &purpose, Cron: &cron, Check: &check})
	if err != nil {
		t.Fatal(err)
	}
	if task.CheckOutput == nil || *task.CheckOutput != "all good\n" {
		t.Fatalf("the check's output at creation is the baseline: %v", task.CheckOutput)
	}
	fire := func() {
		t.Helper()
		got, _ := f.store.GetTask(ctx, task.ID)
		f.rt.fireCheck(ctx, got, time.Now().UnixMilli())
		f.waitIdle(t)
	}

	// Unchanged: no model call at all.
	fire()
	fire()
	if n := len(woken); n != 0 {
		t.Fatalf("an unchanged check shouldn't wake the agent (woken %d times)", n)
	}
	usage, _ := f.store.UsageSince(ctx, f.agent.ID, 0)
	if len(usage) != 0 {
		t.Fatalf("unchanged checks should cost no tokens, got %d calls", len(usage))
	}

	// Changed: woken once, with before and after.
	mu.Lock()
	status = "degraded: api"
	mu.Unlock()
	fire()
	fire()
	mu.Lock()
	defer mu.Unlock()
	if len(woken) != 1 || !strings.Contains(woken[0], "Before:") || !strings.Contains(woken[0], "all good") ||
		!strings.Contains(woken[0], "degraded: api") {
		t.Fatalf("expected one wake with before and after, got %d: %v", len(woken), woken)
	}

	// Checks aren't for app events.
	signal := TaskChange{Signal: &TaskSignal{AccountID: "a", Type: "slack.message"}, Check: &check}
	if _, err := f.rt.SaveTask(ctx, f.agent.ID, task.ID, signal); err == nil {
		t.Fatal("a signal task with a check should be refused")
	}
}

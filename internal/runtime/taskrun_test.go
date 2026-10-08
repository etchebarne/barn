package runtime

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
)

func TestTaskRunsAreSeparate(t *testing.T) {
	ctx := context.Background()
	var f fixture
	var mu sync.Mutex
	var mainSystem, taskSystem, taskInput, mainInput string
	f = setup(t)
	f.rt.Timezone = func(context.Context) *time.Location { return time.UTC }
	f.llm.handler = func(req model.Request) model.Message {
		mu.Lock()
		defer mu.Unlock()
		last := req.Messages[len(req.Messages)-1]
		switch {
		case last.Role == "user" && strings.Contains(last.Text(), `name="Quiet poll"`):
			taskSystem, taskInput = req.Messages[0].Text(), last.Text()
			return toolCall(toolDone, map[string]string{})
		case last.Role == "user" && strings.Contains(last.Text(), `name="Alert"`):
			return sendCall(f.chatID, "Heads up: the build broke.")
		case last.Role == "user":
			mainSystem, mainInput = req.Messages[0].Text(), last.Text()
		}
		return model.Text("assistant", "")
	}
	// Tasks first: they're listed in the system prompt, so creating one rebuilds it.
	tasks := map[string]string{}
	for _, name := range []string{"Quiet poll", "Alert"} {
		task, err := f.store.CreateTask(ctx, store.Task{AgentID: f.agent.ID, Name: name, Purpose: "Check things.",
			Kind: "cron", Cron: "0 9 * * *", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		tasks[name] = task.ID
	}
	f.userSays(t, "hello there")
	f.waitIdle(t)
	before, _ := f.store.Context(ctx, f.agent.ID)

	fire := func(name string) {
		task := store.Task{ID: tasks[name]}
		if _, err := f.store.InsertEvent(ctx, f.agent.ID, EventTask, map[string]any{"taskId": task.ID, "scheduledFor": time.Now().UnixMilli()}); err != nil {
			t.Fatal(err)
		}
		f.rt.wake(f.agent.ID)
		f.waitIdle(t)
	}

	// A quiet run: nothing in the main conversation, no report.
	fire("Quiet poll")
	after, _ := f.store.Context(ctx, f.agent.ID)
	pending, _ := f.store.PendingEvents(ctx, f.agent.ID)
	if len(after) != len(before) || len(pending) != 0 {
		t.Fatalf("a quiet task run left traces: %d→%d entries, %d pending", len(before), len(after), len(pending))
	}
	mu.Lock()
	if taskSystem == "" || taskSystem != mainSystem {
		t.Errorf("task runs should share the main conversation's system prompt (and its cache)")
	}
	if !strings.Contains(taskInput, "<recent_dm") || !strings.Contains(taskInput, "hello there") || !strings.Contains(taskInput, "separate run") {
		t.Errorf("task run input should carry the DM digest and the note: %s", taskInput)
	}
	mu.Unlock()

	// A run that messages the user: the message is sent, and a report waits for the next turn.
	fire("Alert")
	msgs, _, _ := f.store.ListMessages(ctx, f.chatID, "", 10)
	if msgs[len(msgs)-1].Body != "Heads up: the build broke." {
		t.Fatalf("the task's message should be sent: %+v", msgs[len(msgs)-1])
	}
	if after, _ := f.store.Context(ctx, f.agent.ID); len(after) != len(before) {
		t.Fatalf("the report shouldn't start a turn on its own")
	}
	f.userSays(t, "what broke?")
	f.waitIdle(t)
	entries, _ := f.store.Context(ctx, f.agent.ID)
	var joined strings.Builder
	for _, e := range entries {
		joined.Write(e.Entry)
	}
	if !strings.Contains(joined.String(), "task_report") || !strings.Contains(joined.String(), "Heads up: the build broke.") {
		t.Fatalf("the report should join the main conversation with the next turn")
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(mainInput, "what broke?") {
		t.Fatalf("main turn input: %s", mainInput)
	}
}

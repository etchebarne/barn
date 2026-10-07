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

func TestParseAtAndNextFire(t *testing.T) {
	mvd, _ := time.LoadLocation("America/Montevideo") // UTC-3
	at, err := parseAt("2030-01-02 09:30", mvd)
	if err != nil || at.UTC().Format(time.RFC3339) != "2030-01-02T12:30:00Z" {
		t.Fatalf("local time should be read in the user's zone: %v %v", at, err)
	}
	if _, err := parseAt("tomorrow-ish", mvd); err == nil {
		t.Fatal("expected an error for an unreadable time")
	}

	// Weekdays at 10:01 in Montevideo, asked on a Saturday: next is Monday 13:01 UTC.
	sat := time.Date(2030, 1, 5, 12, 0, 0, 0, time.UTC)
	next, err := nextFire(store.Task{Kind: "cron", Cron: "1 10 * * 1-5"}, sat, mvd)
	if err != nil || time.UnixMilli(*next).UTC().Format(time.RFC3339) != "2030-01-07T13:01:00Z" {
		t.Fatalf("next = %v, %v", next, err)
	}
	if _, err := nextFire(store.Task{Kind: "cron", Cron: "every tuesday"}, sat, mvd); err == nil {
		t.Fatal("expected an invalid-cron error")
	}
	if next, _ := nextFire(store.Task{Kind: "once", At: sat.Add(-time.Hour).UnixMilli()}, sat, mvd); next != nil {
		t.Fatal("a one-off task in the past never fires")
	}
}

// An agent schedules a one-off task; when it fires, the agent acts on it.
func TestTaskFires(t *testing.T) {
	var f fixture
	var mu sync.Mutex
	var fired string
	f = setup(t)
	f.rt.Timezone = func(context.Context) *time.Location { return time.UTC }
	f.llm.handler = func(req model.Request) model.Message {
		last := req.Messages[len(req.Messages)-1]
		switch {
		case last.Role == "user" && strings.Contains(last.Text(), "remind me"):
			at := time.Now().Add(1500 * time.Millisecond).UTC().Format(time.RFC3339)
			return toolCall(toolTaskCreate, map[string]string{"name": "Stretch", "purpose": "Remind Martin to stretch.", "at": at})
		case last.Role == "user" && strings.Contains(last.Text(), "<task_fired"):
			mu.Lock()
			fired = last.Text()
			mu.Unlock()
			if !strings.Contains(req.Messages[0].Text(), "Stretch (once at") {
				t.Errorf("the task should be listed in the system prompt")
			}
			return sendCall(f.chatID, "Time to stretch!")
		}
		return model.Text("assistant", "")
	}
	f.userSays(t, "remind me to stretch in a sec")
	msgs := f.waitForMessages(t, 2)
	if msgs[1].Body != "Time to stretch!" {
		t.Fatalf("unexpected reply: %+v", msgs[1])
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(fired, `name="Stretch"`) || !strings.Contains(fired, "Remind Martin to stretch.") {
		t.Fatalf("task_fired = %q", fired)
	}
	tasks, _ := f.store.Tasks(context.Background(), f.agent.ID)
	if len(tasks) != 1 || tasks[0].Enabled || tasks[0].NextFireAt != nil || tasks[0].LastFiredAt == nil {
		t.Fatalf("a fired one-off task should be done: %+v", tasks)
	}
}

// After downtime, missed runs of a repeating task fire once, not once per missed slot.
func TestMissedRunsCollapse(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	past := time.Now().Add(-3 * time.Hour).UnixMilli()
	task, err := f.store.CreateTask(ctx, store.Task{AgentID: f.agent.ID, Name: "Ping", Purpose: "p", Kind: "cron",
		Cron: "*/5 * * * *", Enabled: true, NextFireAt: &past})
	if err != nil {
		t.Fatal(err)
	}
	// The runtime's own scheduler may claim it first (and record the firing a moment later), and
	// the agent may consume the event before we look: count firings wherever they ended up,
	// waiting for the first, then make sure no second one follows.
	firings := func() int {
		n := 0
		events, _ := f.store.PendingEvents(ctx, f.agent.ID)
		for _, e := range events {
			if e.Kind == EventTask {
				n++
			}
		}
		entries, _ := f.store.Context(ctx, f.agent.ID)
		for _, e := range entries {
			if strings.Contains(string(e.Entry), "task_fired task_id") {
				n++
			}
		}
		return n
	}
	f.rt.fireDueTasks(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for firings() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	f.waitIdle(t)
	f.rt.fireDueTasks(ctx) // nothing is due any more
	time.Sleep(50 * time.Millisecond)
	f.waitIdle(t)
	count := firings()
	if count != 1 {
		t.Fatalf("expected one firing, got %d", count)
	}
	got, _ := f.store.GetTask(ctx, task.ID)
	if got.NextFireAt == nil || *got.NextFireAt <= time.Now().UnixMilli() || *got.NextFireAt > time.Now().Add(6*time.Minute).UnixMilli() {
		t.Fatalf("next run should be the next 5-minute slot, got %v", got.NextFireAt)
	}
}

func TestPauseAndResumeTask(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	task, _ := f.store.CreateTask(ctx, store.Task{AgentID: f.agent.ID, Name: "Daily", Purpose: "p", Kind: "cron", Cron: "0 9 * * *", Enabled: true})
	paused, err := f.rt.SetTaskEnabled(ctx, task.ID, false)
	if err != nil || paused.Enabled {
		t.Fatalf("pause: %+v %v", paused, err)
	}
	if due, _ := f.store.DueTasks(ctx, time.Now().Add(48*time.Hour).UnixMilli()); len(due) != 0 {
		t.Fatal("paused tasks are never due")
	}
	resumed, err := f.rt.SetTaskEnabled(ctx, task.ID, true)
	if err != nil || !resumed.Enabled || resumed.NextFireAt == nil {
		t.Fatalf("resume should compute the next run: %+v %v", resumed, err)
	}
}

func TestAgentMessagesArePushed(t *testing.T) {
	var f fixture
	f = setup(t,
		func(model.Request) model.Message { return sendCall(f.chatID, "**Done**, see `report.md`") },
		func(model.Request) model.Message { return model.Text("assistant", "") },
		func(model.Request) model.Message { return sendCall(f.chatID, "quiet one") },
		func(model.Request) model.Message { return model.Text("assistant", "") },
	)
	ctx := context.Background()
	on := true
	f.store.UpdateAgent(ctx, f.agent.ID, store.AgentUpdate{Notifications: &on})
	var mu sync.Mutex
	var pushed []string
	f.rt.Push = func(_ context.Context, title, body, chatID string) {
		mu.Lock()
		pushed = append(pushed, title+"|"+body+"|"+chatID)
		mu.Unlock()
	}
	f.userSays(t, "do it")
	f.waitForMessages(t, 2)
	f.waitIdle(t)
	off := false
	f.store.UpdateAgent(ctx, f.agent.ID, store.AgentUpdate{Notifications: &off})
	f.userSays(t, "again")
	f.waitForMessages(t, 4)
	f.waitIdle(t)
	time.Sleep(100 * time.Millisecond) // pushes are sent in the background

	mu.Lock()
	defer mu.Unlock()
	if len(pushed) != 1 || pushed[0] != "openbot|**Done**, see `report.md`|"+f.chatID {
		t.Fatalf("expected exactly one push while notifications were on, got %q", pushed)
	}
}

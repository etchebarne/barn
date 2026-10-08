package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/sandbox"
	"github.com/etchebarne/openbot/internal/store"
)

// waitRuns waits until the task has n finished runs and returns them, newest first.
func waitRuns(t *testing.T, f fixture, taskID string, n int) []store.TaskRun {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		runs, err := f.store.TaskRuns(context.Background(), taskID, 100)
		if err != nil {
			t.Fatal(err)
		}
		done := 0
		for _, r := range runs {
			if r.Outcome != "running" {
				done++
			}
		}
		if done >= n {
			return runs
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d runs, have %+v", n, runs)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTaskRunHistory(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	f.llm.handler = func(req model.Request) model.Message {
		last := req.Messages[len(req.Messages)-1]
		switch {
		case last.Role == "user" && strings.Contains(last.Text(), `name="Quiet poll"`):
			return toolCall(toolDone, map[string]string{})
		case last.Role == "user" && strings.Contains(last.Text(), `name="Alert"`):
			return sendCall(f.chatID, "Heads up: the build broke.")
		}
		return model.Text("assistant", "")
	}
	mk := func(name string) store.Task {
		task, err := f.store.CreateTask(ctx, store.Task{AgentID: f.agent.ID, Name: name, Purpose: "Check things.",
			Kind: "cron", Cron: "0 9 * * *", Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}
	quiet, alert := mk("Quiet poll"), mk("Alert")

	// Run now: the run is recorded as started by the user, and a quiet one as quiet.
	if err := f.rt.RunTaskNow(ctx, quiet.ID); err != nil {
		t.Fatal(err)
	}
	runs := waitRuns(t, f, quiet.ID, 1)
	if runs[0].Trigger != "manual" || runs[0].Outcome != "quiet" || runs[0].FinishedAt == nil {
		t.Fatalf("quiet run = %+v", runs[0])
	}

	// A scheduled run that messages the user is recorded with what it did.
	if _, err := f.store.InsertEvent(ctx, f.agent.ID, EventTask, map[string]any{"taskId": alert.ID, "scheduledFor": time.Now().UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	f.rt.wake(f.agent.ID)
	runs = waitRuns(t, f, alert.ID, 1)
	if runs[0].Trigger != "schedule" || runs[0].Outcome != "acted" || !strings.Contains(runs[0].Detail, "Heads up") {
		t.Fatalf("alert run = %+v", runs[0])
	}

	// Signal tasks can't run on their own.
	sig, err := f.store.CreateTask(ctx, store.Task{AgentID: f.agent.ID, Name: "On push", Purpose: "x", Kind: "signal",
		SignalAccountID: "acct", SignalType: "push", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.rt.RunTaskNow(ctx, sig.ID); !errors.Is(err, ErrSignalTask) {
		t.Fatalf("RunTaskNow(signal) = %v", err)
	}

	// Recent runs list both, newest first.
	recent, err := f.store.RecentTaskRuns(ctx, 10, false)
	if err != nil || len(recent) != 2 || recent[0].TaskID != alert.ID {
		t.Fatalf("recent = %+v, %v", recent, err)
	}
}

func TestUnchangedChecksAreRecorded(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	f.rt.Sandboxes = &fakeSandbox{exec: func(string, map[string]string) sandbox.Result {
		return sandbox.Result{Output: "same"}
	}}
	task, err := f.store.CreateTask(ctx, store.Task{AgentID: f.agent.ID, Name: "Watch", Purpose: "x", Kind: "cron",
		Cron: "*/5 * * * *", Check: "cat status", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	same := "same"
	task.CheckOutput = &same
	f.rt.fireCheck(ctx, task, time.Now().UnixMilli(), false)
	runs := waitRuns(t, f, task.ID, 1)
	if runs[0].Outcome != "unchanged" {
		t.Fatalf("run = %+v", runs[0])
	}
	// Left out of the recent list unless asked for.
	if recent, _ := f.store.RecentTaskRuns(ctx, 10, false); len(recent) != 0 {
		t.Fatalf("recent = %+v", recent)
	}
}

func TestUpcoming(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	f.rt.Timezone = func(context.Context) *time.Location { return time.UTC }
	now := time.Now()
	mk := func(task store.Task) store.Task {
		next, err := nextFire(task, now, time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		task.NextFireAt = next
		task.AgentID, task.Purpose, task.Enabled = f.agent.ID, "x", task.Enabled || task.Name != "Paused"
		return task
	}
	tasks := []store.Task{
		mk(store.Task{ID: "daily", Name: "Daily", Kind: "cron", Cron: "0 9 * * *"}),
		mk(store.Task{ID: "often", Name: "Often", Kind: "cron", Cron: "* * * * *"}),
		mk(store.Task{ID: "once", Name: "Once", Kind: "once", At: now.Add(2 * time.Hour).UnixMilli()}),
		mk(store.Task{ID: "later", Name: "Later", Kind: "once", At: now.Add(30 * 24 * time.Hour).UnixMilli()}),
		mk(store.Task{ID: "paused", Name: "Paused", Kind: "cron", Cron: "0 9 * * *"}),
	}
	got := map[string][2]int{}
	for _, u := range f.rt.Upcoming(ctx, tasks, 7) {
		got[u.TaskId] = [2]int{len(u.Times), u.More}
	}
	if d := got["daily"]; d[0] < 7 || d[0] > 8 || d[1] != 0 {
		t.Errorf("daily = %v", d)
	}
	if o := got["often"]; o[0] != maxUpcoming || o[1] < 5000 {
		t.Errorf("often = %v", o)
	}
	if o := got["once"]; o != [2]int{1, 0} {
		t.Errorf("once = %v", o)
	}
	if _, ok := got["later"]; ok {
		t.Error("a run beyond the window shouldn't be listed")
	}
	if _, ok := got["paused"]; ok {
		t.Error("a paused task shouldn't be listed")
	}
}

package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/etchebarne/openbot/internal/api/gen"
	"github.com/etchebarne/openbot/internal/store"
	"github.com/etchebarne/openbot/internal/view"
)

// Tasks keep a history of their runs (store.TaskRun), shown on the Schedule page, which also
// shows when they run next and lets the user run one now.

// ErrSignalTask is returned by RunTaskNow for tasks that run on app events.
var ErrSignalTask = errors.New("this task runs when an app sends an event, so it can't run on its own")

func (m *Manager) publishTaskRun(r store.TaskRun) {
	m.bus.Publish(gen.WsTaskRun{Type: "task.run", Run: view.TaskRun(r)})
}

// finishTaskRun records how a run ended and tells clients.
func (m *Manager) finishTaskRun(ctx context.Context, r store.TaskRun, outcome, detail string) {
	if err := m.store.FinishTaskRun(ctx, r.ID, outcome, detail); err != nil {
		logger(r.AgentID).Error("record task run", "task", r.TaskID, "err", err)
		return
	}
	finished := time.Now().UnixMilli()
	r.Outcome, r.Detail, r.FinishedAt = outcome, detail, &finished
	m.publishTaskRun(r)
}

// recordRun records a run that's already over (a check whose output didn't change).
func (m *Manager) recordRun(ctx context.Context, t store.Task, trigger, outcome, detail string) {
	r, err := m.store.StartTaskRun(ctx, store.TaskRun{TaskID: t.ID, AgentID: t.AgentID, Trigger: trigger, Outcome: outcome, Detail: detail})
	if err != nil {
		logger(t.AgentID).Error("record task run", "task", t.ID, "err", err)
		return
	}
	m.publishTaskRun(r)
}

// RunTaskNow runs a task outside its schedule. A task with a check runs the check and wakes the
// agent whatever it outputs.
func (m *Manager) RunTaskNow(ctx context.Context, taskID string) error {
	t, err := m.store.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if t.Kind == "signal" {
		return ErrSignalTask
	}
	now := time.Now().UnixMilli()
	if t.Check != "" {
		go m.fireCheck(context.WithoutCancel(ctx), t, now, true)
		return nil
	}
	if _, err := m.store.InsertEvent(ctx, t.AgentID, EventTask, map[string]any{
		"taskId": t.ID, "scheduledFor": now, "manual": true,
	}); err != nil {
		return err
	}
	logger(t.AgentID).Info("task run by the user", "task", t.Name)
	m.wake(t.AgentID)
	return nil
}

// maxUpcoming bounds the runs listed per task (a task every few minutes has thousands a week).
const maxUpcoming = 50

// Upcoming lists when each enabled cron or one-off task runs in the next days.
func (m *Manager) Upcoming(ctx context.Context, tasks []store.Task, days int) []gen.UpcomingRuns {
	loc := m.location(ctx)
	now := time.Now()
	end := now.Add(time.Duration(days) * 24 * time.Hour)
	out := []gen.UpcomingRuns{}
	for _, t := range tasks {
		if !t.Enabled || t.Kind == "signal" || t.NextFireAt == nil {
			continue
		}
		u := gen.UpcomingRuns{TaskId: t.ID, Times: []time.Time{}}
		at := time.UnixMilli(*t.NextFireAt)
		for !at.After(end) {
			if len(u.Times) < maxUpcoming {
				u.Times = append(u.Times, at.UTC())
			} else {
				u.More++
			}
			next, err := nextFire(t, at, loc)
			if err != nil || next == nil {
				break
			}
			at = time.UnixMilli(*next)
			if u.More > 10_000 { // a task every minute for a month; enough counting
				break
			}
		}
		if len(u.Times) > 0 {
			out = append(out, u)
		}
	}
	return out
}

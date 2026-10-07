package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/robfig/cron/v3"

	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/store"
)

// EventTask fires an agent's scheduled task. Payload: {"taskId", "scheduledFor"}.
const EventTask = "task"

const (
	toolTaskCreate = "task_create"
	toolTaskUpdate = "task_update"
	toolTaskDelete = "task_delete"

	maxTasksPerAgent = 50
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

var (
	taskCreateTool = function(toolTaskCreate,
		"Schedule something for yourself to do later or regularly. When it's due you get a "+
			"<task_fired> and act on it (e.g. message the user). Give exactly one of cron (repeating, "+
			"5 fields: minute hour day-of-month month day-of-week, in the user's time zone; e.g. "+
			"\"1 10 * * 1-5\" is weekdays at 10:01) or at (once, local date-time \"2026-10-09 15:30\" "+
			"or RFC 3339).",
		`{
			"type": "object",
			"properties": {
				"name": {"type": "string", "description": "Short title, e.g. \"Weekday check-in\"."},
				"purpose": {"type": "string", "description": "What to do when it fires, in enough detail to act on it later."},
				"cron": {"type": "string"},
				"at": {"type": "string"}
			},
			"required": ["name", "purpose"],
			"additionalProperties": false
		}`)

	taskUpdateTool = function(toolTaskUpdate,
		"Change one of your tasks: rename it, change what it does or when, or pause/resume it.",
		`{
			"type": "object",
			"properties": {
				"task_id": {"type": "string"},
				"name": {"type": "string"},
				"purpose": {"type": "string"},
				"cron": {"type": "string"},
				"at": {"type": "string"},
				"enabled": {"type": "boolean"}
			},
			"required": ["task_id"],
			"additionalProperties": false
		}`)

	taskDeleteTool = function(toolTaskDelete,
		"Delete one of your tasks.",
		`{
			"type": "object",
			"properties": {"task_id": {"type": "string"}},
			"required": ["task_id"],
			"additionalProperties": false
		}`)
)

// Timezone returns the user's time zone; set by barnd from the app's settings.
type Timezone func(ctx context.Context) *time.Location

func (m *Manager) location(ctx context.Context) *time.Location {
	if m.Timezone != nil {
		return m.Timezone(ctx)
	}
	return time.Local
}

// parseAt reads a one-off time: RFC 3339, or a local date-time in the user's time zone.
func parseAt(s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("can't read %q as a date-time; use \"2006-01-02 15:04\" or RFC 3339", s)
}

// nextFire computes when a task runs next after `after` (nil: never again).
func nextFire(t store.Task, after time.Time, loc *time.Location) (*int64, error) {
	switch t.Kind {
	case "cron":
		sched, err := cronParser.Parse(t.Cron)
		if err != nil {
			return nil, fmt.Errorf("invalid cron %q: %v", t.Cron, err)
		}
		next := sched.Next(after.In(loc))
		if next.IsZero() {
			return nil, nil
		}
		ms := next.UnixMilli()
		return &ms, nil
	case "once":
		if t.At <= after.UnixMilli() {
			return nil, nil
		}
		at := t.At
		return &at, nil
	}
	return nil, fmt.Errorf("unknown task kind %q", t.Kind)
}

// describeSchedule is a compact human description for prompts.
func describeSchedule(t store.Task, loc *time.Location) string {
	if t.Kind == "cron" {
		return "repeats: cron " + t.Cron
	}
	return "once at " + time.UnixMilli(t.At).In(loc).Format("Mon 2 Jan 2006 15:04")
}

// ---- scheduler ----

// runScheduler fires due tasks. It sleeps until the next one is due (at most a minute, so clock
// changes and newly enabled tasks are picked up) or until tasks change.
func (m *Manager) runScheduler(ctx context.Context) {
	for {
		m.fireDueTasks(ctx)
		wait := time.Minute
		if next, err := m.store.NextTaskFireAt(ctx); err == nil && next > 0 {
			if d := time.Until(time.UnixMilli(next)); d < wait {
				wait = max(d, 0)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		case <-m.tasksChanged:
		}
	}
}

func (m *Manager) pokeScheduler() {
	select {
	case m.tasksChanged <- struct{}{}:
	default:
	}
}

func (m *Manager) fireDueTasks(ctx context.Context) {
	now := time.Now()
	due, err := m.store.DueTasks(ctx, now.UnixMilli())
	if err != nil {
		logger("scheduler").Error("load due tasks", "err", err)
		return
	}
	loc := m.location(ctx)
	for _, t := range due {
		scheduled := *t.NextFireAt
		fired := now.UnixMilli()
		t.LastFiredAt = &fired
		// Runs missed while the server was down collapse into this one.
		next, err := nextFire(t, now, loc)
		if err != nil {
			next = nil
		}
		t.NextFireAt = next
		if next == nil && t.Kind == "once" {
			t.Enabled = false
		}
		won, err := m.store.ClaimTaskFire(ctx, t, scheduled)
		if err != nil {
			logger(t.AgentID).Error("claim task", "task", t.ID, "err", err)
			continue
		}
		if !won {
			continue // fired by someone else already
		}
		if _, err := m.store.InsertEvent(ctx, t.AgentID, EventTask, map[string]any{
			"taskId": t.ID, "scheduledFor": scheduled,
		}); err != nil {
			logger(t.AgentID).Error("fire task", "task", t.ID, "err", err)
			continue
		}
		logger(t.AgentID).Info("task fired", "task", t.Name)
		m.wake(t.AgentID)
	}
}

// ---- tools ----

func (l *loop) runTaskTool(ctx context.Context, agent store.Agent, name string, raw []byte) (string, bool) {
	loc := l.m.location(ctx)
	var a struct {
		TaskID  string  `json:"task_id"`
		Name    *string `json:"name"`
		Purpose *string `json:"purpose"`
		Cron    *string `json:"cron"`
		At      *string `json:"at"`
		Enabled *bool   `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: %v", err), false
	}

	var t store.Task
	switch name {
	case toolTaskCreate:
		existing, err := l.m.store.Tasks(ctx, agent.ID)
		if err != nil {
			return toolError("%v", err), false
		}
		if len(existing) >= maxTasksPerAgent {
			return toolError("you already have %d tasks; delete some first", len(existing)), false
		}
		t = store.Task{AgentID: agent.ID, Enabled: true}
	case toolTaskDelete, toolTaskUpdate:
		var err error
		t, err = l.m.store.GetTask(ctx, a.TaskID)
		if err != nil || t.AgentID != agent.ID {
			return toolError("you have no task %q", a.TaskID), false
		}
		if name == toolTaskDelete {
			if err := l.m.store.DeleteTask(ctx, t.ID); err != nil {
				return toolError("%v", err), false
			}
			l.m.pokeScheduler()
			return toolOK(map[string]string{"deleted": t.Name}), true
		}
	}

	if a.Name != nil {
		t.Name = strings.TrimSpace(*a.Name)
	}
	if a.Purpose != nil {
		t.Purpose = strings.TrimSpace(*a.Purpose)
	}
	if t.Name == "" || utf8.RuneCountInString(t.Name) > 80 {
		return toolError("name must be 1–80 characters"), false
	}
	if t.Purpose == "" {
		return toolError("purpose is required"), false
	}
	if a.Cron != nil && a.At != nil {
		return toolError("give either cron or at, not both"), false
	}
	switch {
	case a.Cron != nil:
		t.Kind, t.Cron, t.At = "cron", strings.TrimSpace(*a.Cron), 0
	case a.At != nil:
		at, err := parseAt(*a.At, loc)
		if err != nil {
			return toolError("%v", err), false
		}
		if !at.After(time.Now()) {
			return toolError("%s is in the past (now it's %s)", at.In(loc).Format(time.RFC3339), time.Now().In(loc).Format(time.RFC3339)), false
		}
		t.Kind, t.At, t.Cron = "once", at.UnixMilli(), ""
		if a.Enabled == nil {
			t.Enabled = true
		}
	case name == toolTaskCreate:
		return toolError("give a schedule: cron (repeating) or at (once)"), false
	}
	if a.Enabled != nil {
		t.Enabled = *a.Enabled
	}
	next, err := nextFire(t, time.Now(), loc)
	if err != nil {
		return toolError("%v", err), false
	}
	t.NextFireAt = next

	if name == toolTaskCreate {
		if t, err = l.m.store.CreateTask(ctx, t); err != nil {
			return toolError("%v", err), false
		}
	} else if err := l.m.store.SaveTask(ctx, t); err != nil {
		return toolError("%v", err), false
	}
	l.m.pokeScheduler()
	out := map[string]any{"task_id": t.ID, "name": t.Name, "schedule": describeSchedule(t, loc), "enabled": t.Enabled}
	if t.NextFireAt != nil && t.Enabled {
		out["next_run"] = time.UnixMilli(*t.NextFireAt).In(loc).Format("Mon 2 Jan 2006 15:04 MST")
	}
	return toolOK(out), true
}

func taskTools() []model.Tool { return []model.Tool{taskCreateTool, taskUpdateTool, taskDeleteTool} }

// writeTasks lists the agent's tasks in its system prompt.
func (l *loop) writeTasks(ctx context.Context, b *strings.Builder, agent store.Agent) error {
	tasks, err := l.m.store.Tasks(ctx, agent.ID)
	if err != nil {
		return err
	}
	loc := l.m.location(ctx)
	b.WriteString("# Your tasks\n")
	b.WriteString("Things you scheduled with task_create. When one is due you get a <task_fired>.\n")
	if len(tasks) == 0 {
		b.WriteString("(none)\n")
	}
	for _, t := range tasks {
		state := "next: never"
		switch {
		case !t.Enabled:
			state = "paused"
		case t.NextFireAt != nil:
			state = "next: " + time.UnixMilli(*t.NextFireAt).In(loc).Format("Mon 2 Jan 15:04")
		}
		fmt.Fprintf(b, "- [%s] %s (%s; %s): %s\n", t.ID, t.Name, describeSchedule(t, loc), state, truncate(t.Purpose, 300))
	}
	b.WriteString("\n")
	return nil
}

func (l *loop) renderTask(ctx context.Context, payload json.RawMessage) (string, error) {
	var p struct {
		TaskID       string `json:"taskId"`
		ScheduledFor int64  `json:"scheduledFor"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", err
	}
	t, err := l.m.store.GetTask(ctx, p.TaskID)
	if err != nil {
		return "", nil // deleted since it fired
	}
	loc := l.m.location(ctx)
	return fmt.Sprintf("<task_fired task_id=%q name=%q scheduled_for=%q>\n%s\n\nDo this now. If there's something "+
		"to tell the user, message them (usually in your DM); if not, end your turn quietly.\n</task_fired>",
		t.ID, t.Name, time.UnixMilli(p.ScheduledFor).In(loc).Format(time.RFC3339), t.Purpose), nil
}

// SetTaskEnabled pauses or resumes a task (resuming recomputes its next run).
func (m *Manager) SetTaskEnabled(ctx context.Context, taskID string, enabled bool) (store.Task, error) {
	t, err := m.store.GetTask(ctx, taskID)
	if err != nil {
		return t, err
	}
	t.Enabled = enabled
	if enabled {
		if t.NextFireAt, err = nextFire(t, time.Now(), m.location(ctx)); err != nil {
			return t, err
		}
	}
	if err := m.store.SaveTask(ctx, t); err != nil {
		return t, err
	}
	m.pokeScheduler()
	return t, nil
}

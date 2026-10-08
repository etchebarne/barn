package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/robfig/cron/v3"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
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
			"<task_fired> and act on it (e.g. message the user). Give exactly one of: cron (repeating, "+
			"5 fields: minute hour day-of-month month day-of-week, in the user's time zone; e.g. "+
			"\"1 10 * * 1-5\" is weekdays at 10:01), at (once, local date-time \"2026-10-09 15:30\" "+
			"or RFC 3339), or on_signal (whenever a connected app sends a matching event). To watch "+
			"something regularly (a page, an API, a channel), give a check: a shell command run on "+
			"the schedule in your computer that wakes you only when its output changes, so polling "+
			"costs nothing until there's news.",
		`{
			"type": "object",
			"properties": {
				"name": {"type": "string", "description": "Short title, e.g. \"Weekday check-in\"."},
				"purpose": {"type": "string", "description": "What to do when it fires, in enough detail to act on it later."},
				"cron": {"type": "string"},
				"at": {"type": "string"},
				"check": {"type": "string", "description": "Optional, for cron/at: a shell command (with your secrets as env vars), e.g. curl -s https://api.example.com/status | jq .state. You're woken only when its output differs from the last run's, and see before and after. Its output now is the baseline."},
				"on_signal": {
					"type": "object",
					"description": "Run when a connected app sends this kind of event.",
					"properties": {
						"account": {"type": "string", "description": "Account name or account_id (see Your connected apps)."},
						"type": {"type": "string", "description": "Signal type, e.g. slack.app_mention or linear.issue."},
						"match": {"type": "object", "additionalProperties": {"type": "string"}, "description": "Field → text it must contain, e.g. {\"channel\": \"C0123\"} or {\"state\": \"Done\"}. Empty matches every event of the type."}
					},
					"required": ["account", "type"],
					"additionalProperties": false
				}
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
				"on_signal": {"type": "object", "description": "Same shape as in task_create."},
				"check": {"type": "string", "description": "Same as in task_create; empty removes it."},
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

// Timezone returns the user's time zone; set by openbotd from the app's settings.
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
	case "signal":
		return nil, nil // runs when events arrive, not on a clock
	}
	return nil, fmt.Errorf("unknown task kind %q", t.Kind)
}

// describeSchedule is a compact human description for prompts.
func describeSchedule(t store.Task, loc *time.Location) string {
	switch t.Kind {
	case "cron":
		return "repeats: cron " + t.Cron
	case "signal":
		return "on signal " + t.SignalType + describeMatch(t.SignalMatch)
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
		if t.Check != "" {
			// Off the scheduler's loop: a check can take up to a minute.
			go m.fireCheck(context.WithoutCancel(ctx), t, scheduled, false)
			continue
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
	var a struct {
		TaskID   string          `json:"task_id"`
		Name     *string         `json:"name"`
		Purpose  *string         `json:"purpose"`
		Cron     *string         `json:"cron"`
		At       *string         `json:"at"`
		OnSignal json.RawMessage `json:"on_signal"`
		Enabled  *bool           `json:"enabled"`
		Check    *string         `json:"check"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return toolError("invalid arguments: %v", err), false
	}
	if name != toolTaskCreate {
		t, err := l.m.store.GetTask(ctx, a.TaskID)
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
	c := TaskChange{Name: a.Name, Purpose: a.Purpose, Cron: a.Cron, At: a.At, Enabled: a.Enabled, Check: a.Check}
	if len(a.OnSignal) > 0 && string(a.OnSignal) != "null" {
		accountID, typ, match, err := l.resolveSignal(ctx, agent, a.OnSignal)
		if err != nil {
			return toolError("%v", err), false
		}
		c.Signal = &TaskSignal{AccountID: accountID, Type: typ, Match: match}
	}
	taskID := ""
	if name != toolTaskCreate {
		taskID = a.TaskID
	}
	t, err := l.m.SaveTask(ctx, agent.ID, taskID, c)
	if err != nil {
		return toolError("%v", err), false
	}
	loc := l.m.location(ctx)
	out := map[string]any{"task_id": t.ID, "name": t.Name, "schedule": describeSchedule(t, loc), "enabled": t.Enabled}
	if t.Check != "" && a.Check != nil && t.CheckOutput != nil {
		out["check_output_now"] = truncate(*t.CheckOutput, 1500)
		out["note"] = "That's the baseline: you're woken when the check's output changes from it."
	}
	if t.NextFireAt != nil && t.Enabled {
		out["next_run"] = time.UnixMilli(*t.NextFireAt).In(loc).Format("Mon 2 Jan 2006 15:04 MST")
	}
	return toolOK(out), true
}

// TaskChange is a new task, or a change to one: fields left nil stay as they are. At most one
// of Cron, At and Signal.
type TaskChange struct {
	Name, Purpose *string
	Cron          *string // repeating, 5 fields in the user's time zone
	At            *string // once: local "2026-10-09 15:30" or RFC 3339
	Signal        *TaskSignal
	Enabled       *bool
	// Check is a shell command that gates the task ("" removes it); see checks.go.
	Check *string
}

// TaskSignal runs a task on a connected app's events.
type TaskSignal struct {
	AccountID, Type string
	Match           map[string]string
}

// SaveTask creates a task for an agent (taskID "") or changes one of its tasks, by the agent or
// the user. Problems with the change are *ModelError.
func (m *Manager) SaveTask(ctx context.Context, agentID, taskID string, c TaskChange) (store.Task, error) {
	loc := m.location(ctx)
	var t store.Task
	if taskID == "" {
		existing, err := m.store.Tasks(ctx, agentID)
		if err != nil {
			return t, err
		}
		if len(existing) >= maxTasksPerAgent {
			return t, &ModelError{fmt.Sprintf("there are already %d tasks; delete some first", len(existing))}
		}
		t = store.Task{AgentID: agentID, Enabled: true}
	} else {
		var err error
		if t, err = m.store.GetTask(ctx, taskID); err != nil {
			return t, err
		}
		if t.AgentID != agentID {
			return t, store.ErrNotFound
		}
	}

	if c.Name != nil {
		t.Name = strings.TrimSpace(*c.Name)
	}
	if c.Purpose != nil {
		t.Purpose = strings.TrimSpace(*c.Purpose)
	}
	if t.Name == "" || utf8.RuneCountInString(t.Name) > 80 {
		return t, &ModelError{"name must be 1–80 characters"}
	}
	if t.Purpose == "" {
		return t, &ModelError{"purpose is required"}
	}
	schedules := 0
	for _, set := range []bool{c.Cron != nil, c.At != nil, c.Signal != nil} {
		if set {
			schedules++
		}
	}
	if schedules > 1 {
		return t, &ModelError{"give only one of cron, at or on_signal"}
	}
	switch {
	case c.Signal != nil:
		t.Kind, t.Cron, t.At = "signal", "", 0
		t.SignalAccountID, t.SignalType, t.SignalMatch = c.Signal.AccountID, c.Signal.Type, c.Signal.Match
	case c.Cron != nil:
		t.Kind, t.Cron, t.At = "cron", strings.TrimSpace(*c.Cron), 0
	case c.At != nil:
		at, err := parseAt(*c.At, loc)
		if err != nil {
			return t, &ModelError{err.Error()}
		}
		if !at.After(time.Now()) {
			return t, &ModelError{fmt.Sprintf("%s is in the past (now it's %s)", at.In(loc).Format(time.RFC3339), time.Now().In(loc).Format(time.RFC3339))}
		}
		t.Kind, t.At, t.Cron = "once", at.UnixMilli(), ""
		if c.Enabled == nil {
			t.Enabled = true
		}
	case taskID == "":
		return t, &ModelError{"give a schedule: cron (repeating), at (once) or on_signal (events)"}
	}
	if c.Enabled != nil {
		t.Enabled = *c.Enabled
	}
	checkChanged := false
	if c.Check != nil {
		check := strings.TrimSpace(*c.Check)
		if len(check) > maxCheckLength {
			return t, &ModelError{fmt.Sprintf("a check can be at most %d characters", maxCheckLength)}
		}
		checkChanged = check != t.Check
		t.Check = check
	}
	if t.Check != "" {
		if t.Kind == "signal" {
			return t, &ModelError{"checks are for scheduled tasks (cron or at); app events already wake you only when something happens"}
		}
		if !m.sandboxesAvailable() {
			return t, &ModelError{"checks run in your computer, and this server has none"}
		}
	}
	if checkChanged {
		t.CheckOutput = nil
		if t.Check != "" {
			// Its current output is the baseline: you're woken when it changes from this.
			out := m.runCheck(ctx, t)
			t.CheckOutput = &out
		}
	}
	next, err := nextFire(t, time.Now(), loc)
	if err != nil {
		return t, &ModelError{err.Error()}
	}
	t.NextFireAt = next

	if taskID == "" {
		if t, err = m.store.CreateTask(ctx, t); err != nil {
			return t, err
		}
	} else if err := m.store.SaveTask(ctx, t); err != nil {
		return t, err
	}
	m.pokeScheduler()
	return t, nil
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
		// No next-run times: they change after every run, and this prompt stays frozen.
		state := "active"
		if !t.Enabled {
			state = "paused"
		}
		check := ""
		if t.Check != "" {
			check = "; wakes you only when its check changes: `" + truncate(t.Check, 200) + "`"
		}
		fmt.Fprintf(b, "- [%s] %s (%s; %s%s): %s\n", t.ID, t.Name, describeSchedule(t, loc), state, check, truncate(t.Purpose, 300))
	}
	b.WriteString("\n")
	return nil
}

func (l *loop) renderTask(ctx context.Context, payload json.RawMessage) (string, error) {
	var p struct {
		TaskID       string `json:"taskId"`
		ScheduledFor int64  `json:"scheduledFor"`
		SignalID     string `json:"signalId"`
		CheckBefore  string `json:"checkBefore"`
		CheckAfter   string `json:"checkAfter"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", err
	}
	t, err := l.m.store.GetTask(ctx, p.TaskID)
	if err != nil {
		return "", nil // deleted since it fired
	}
	loc := l.m.location(ctx)
	event := ""
	if p.SignalID != "" {
		if sig, err := l.m.store.GetSignal(ctx, p.SignalID); err == nil {
			var payload struct {
				Fields map[string]string `json:"fields"`
			}
			_ = json.Unmarshal(sig.Payload, &payload)
			fields, _ := json.MarshalIndent(payload.Fields, "", "  ")
			event = fmt.Sprintf("\n\nThe event (%s):\n%s", sig.Type, truncate(string(fields), 6000))
		}
	}
	if p.CheckAfter != "" {
		event += renderCheckChange(p.CheckBefore, p.CheckAfter)
	}
	return fmt.Sprintf("<task_fired task_id=%q name=%q at=%q>\n%s%s\n\nDo this now. If there's something "+
		"to tell the user, message them (usually in your DM); if not, end your turn quietly.\n</task_fired>",
		t.ID, t.Name, time.UnixMilli(p.ScheduledFor).In(loc).Format(time.RFC3339), t.Purpose, event), nil
}

func describeMatch(match map[string]string) string {
	if len(match) == 0 {
		return ""
	}
	keys := make([]string, 0, len(match))
	for k := range match {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s contains %q", k, match[k])
	}
	return " where " + strings.Join(parts, " and ")
}

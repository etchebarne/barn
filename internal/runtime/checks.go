package runtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/etchebarne/openbot/internal/store"
)

// A task can have a check: a shell command run on schedule in the agent's computer, at no token
// cost. The agent is woken only when its output differs from the previous run's (a poll that
// finds nothing new costs nothing), and then sees what changed.

const (
	checkTimeout   = 60 * time.Second
	maxCheckOutput = 8000
	maxCheckLength = 2000
)

// runCheck runs a task's check and returns its output (with the exit code when it failed), or
// why it couldn't run.
func (m *Manager) runCheck(ctx context.Context, t store.Task) string {
	if !m.sandboxesAvailable() {
		return "the check couldn't run: this server has no sandboxes"
	}
	box, err := m.store.SandboxFor(ctx, t.AgentID)
	if err != nil {
		return "the check couldn't run: " + err.Error()
	}
	env, err := m.secretValues(ctx, t.AgentID)
	if err != nil {
		return "the check couldn't run: " + err.Error()
	}
	res, err := m.Sandboxes.Exec(ctx, box, t.Check, "", nil, checkTimeout, env)
	if err != nil {
		return "the check couldn't run: " + err.Error()
	}
	out := res.Output
	switch {
	case res.TimedOut:
		out += fmt.Sprintf("\n[timed out after %s]", checkTimeout)
	case res.ExitCode != 0:
		out += fmt.Sprintf("\n[exit %d]", res.ExitCode)
	}
	if len(out) > maxCheckOutput {
		out = out[:maxCheckOutput] + "\n[…]"
	}
	return out
}

// fireCheck runs a due task's check and wakes the agent only if its output changed (or always,
// when the user ran it with Run now).
func (m *Manager) fireCheck(ctx context.Context, t store.Task, scheduled int64, manual bool) {
	out := m.runCheck(ctx, t)
	if t.CheckOutput != nil && *t.CheckOutput == out && !manual {
		logger(t.AgentID).Info("task check unchanged", "task", t.Name)
		m.recordRun(ctx, t, "schedule", "unchanged", "")
		return
	}
	if err := m.store.SetTaskCheckOutput(ctx, t.ID, out); err != nil {
		logger(t.AgentID).Error("save check output", "task", t.ID, "err", err)
		return
	}
	before := ""
	if t.CheckOutput != nil {
		before = *t.CheckOutput
	}
	if _, err := m.store.InsertEvent(ctx, t.AgentID, EventTask, map[string]any{
		"taskId": t.ID, "scheduledFor": scheduled, "checkBefore": before, "checkAfter": out, "manual": manual,
	}); err != nil {
		logger(t.AgentID).Error("fire task", "task", t.ID, "err", err)
		return
	}
	logger(t.AgentID).Info("task check changed", "task", t.Name)
	m.wake(t.AgentID)
}

// renderCheckChange shows what a check's output went from and to.
func renderCheckChange(before, after string) string {
	clip := func(s string) string {
		s = strings.TrimSpace(s)
		if len(s) > 3000 {
			s = s[:3000] + "\n[…]"
		}
		if s == "" {
			s = "(no output)"
		}
		return s
	}
	if before == "" {
		return "\n\nYour check ran for the first time. Its output:\n```\n" + clip(after) + "\n```"
	}
	return "\n\nYour check's output changed.\nBefore:\n```\n" + clip(before) + "\n```\nNow:\n```\n" + clip(after) + "\n```"
}

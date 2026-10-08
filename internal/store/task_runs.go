package store

import (
	"context"

	"github.com/etchebarne/openbot/internal/ids"
)

// TaskRun is one run of a task.
type TaskRun struct {
	ID         string
	TaskID     string
	AgentID    string
	Trigger    string // "schedule" | "signal" | "manual"
	Outcome    string // "running" | "quiet" | "acted" | "unchanged" | "failed" | "stopped"
	Detail     string // what the agent did (acted), or what went wrong (failed)
	StartedAt  int64
	FinishedAt *int64
}

// maxRunsPerTask bounds each task's history (a check every few minutes adds up).
const maxRunsPerTask = 100

const taskRunColumns = `id, task_id, agent_id, trigger, outcome, detail, started_at, finished_at`

// StartTaskRun records that a task started running (outcome "running"), or, with a final
// outcome, a run that's already over.
func (s *Store) StartTaskRun(ctx context.Context, r TaskRun) (TaskRun, error) {
	r.ID, r.StartedAt = ids.New(), now()
	if r.Outcome == "" {
		r.Outcome = "running"
	} else {
		r.FinishedAt = &r.StartedAt
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO task_runs (`+taskRunColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.TaskID, r.AgentID, r.Trigger, r.Outcome, r.Detail, r.StartedAt, r.FinishedAt)
	if err != nil {
		return r, err
	}
	// Keep the latest runs only.
	_, err = s.db.ExecContext(ctx, `DELETE FROM task_runs WHERE task_id = ? AND id NOT IN
		(SELECT id FROM task_runs WHERE task_id = ? ORDER BY started_at DESC, id DESC LIMIT ?)`,
		r.TaskID, r.TaskID, maxRunsPerTask)
	return r, err
}

// FinishTaskRun records how a run ended.
func (s *Store) FinishTaskRun(ctx context.Context, id, outcome, detail string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE task_runs SET outcome = ?, detail = ?, finished_at = ? WHERE id = ?`,
		outcome, detail, now(), id)
	return err
}

// InterruptTaskRuns ends runs a restart cut short.
func (s *Store) InterruptTaskRuns(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE task_runs SET outcome = 'failed',
		detail = 'Interrupted by a server restart.', finished_at = ? WHERE outcome = 'running'`, now())
	return err
}

// TaskRuns returns a task's runs, newest first.
func (s *Store) TaskRuns(ctx context.Context, taskID string, limit int) ([]TaskRun, error) {
	return s.queryTaskRuns(ctx, `SELECT `+taskRunColumns+` FROM task_runs WHERE task_id = ?
		ORDER BY started_at DESC, id DESC LIMIT ?`, taskID, limit)
}

// RecentTaskRuns returns the latest runs of all tasks, newest first. Unchanged checks are left
// out unless includeUnchanged: they'd crowd out everything else.
func (s *Store) RecentTaskRuns(ctx context.Context, limit int, includeUnchanged bool) ([]TaskRun, error) {
	q := `SELECT ` + taskRunColumns + ` FROM task_runs`
	if !includeUnchanged {
		q += ` WHERE outcome != 'unchanged'`
	}
	return s.queryTaskRuns(ctx, q+` ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
}

func (s *Store) queryTaskRuns(ctx context.Context, q string, args ...any) ([]TaskRun, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TaskRun
	for rows.Next() {
		var r TaskRun
		if err := rows.Scan(&r.ID, &r.TaskID, &r.AgentID, &r.Trigger, &r.Outcome, &r.Detail, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AllTasks returns every agent's tasks, oldest first.
func (s *Store) AllTasks(ctx context.Context) ([]Task, error) {
	return s.queryTasks(ctx, `SELECT `+taskColumns+` FROM tasks ORDER BY id`)
}

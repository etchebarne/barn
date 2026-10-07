package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/etchebarne/openbot/internal/ids"
)

// Task is something an agent does on a schedule.
type Task struct {
	ID      string
	AgentID string
	Name    string
	Purpose string
	Kind    string // "cron" | "once" | "signal"
	Cron    string // for kind "cron"
	At      int64  // for kind "once" (unix ms)
	// For kind "signal": which account's events, of which type, matching which fields.
	SignalAccountID string
	SignalType      string
	SignalMatch     map[string]string
	Enabled         bool
	NextFireAt      *int64
	LastFiredAt     *int64
	CreatedAt       int64
}

const taskColumns = `id, agent_id, name, purpose, kind, spec, enabled, next_fire_at, last_fired_at, created_at`

func scanTask(row interface{ Scan(...any) error }) (Task, error) {
	var t Task
	var spec string
	err := row.Scan(&t.ID, &t.AgentID, &t.Name, &t.Purpose, &t.Kind, &spec, &t.Enabled,
		&t.NextFireAt, &t.LastFiredAt, &t.CreatedAt)
	if err != nil {
		return t, err
	}
	var s struct {
		Cron      string            `json:"cron"`
		At        int64             `json:"at"`
		AccountID string            `json:"accountId"`
		Type      string            `json:"type"`
		Match     map[string]string `json:"match"`
	}
	if err := unmarshalString(spec, &s); err != nil {
		return t, err
	}
	t.Cron, t.At = s.Cron, s.At
	t.SignalAccountID, t.SignalType, t.SignalMatch = s.AccountID, s.Type, s.Match
	return t, nil
}

func taskSpec(t Task) (string, error) {
	switch t.Kind {
	case "cron":
		return marshalString(map[string]string{"cron": t.Cron})
	case "signal":
		return marshalString(map[string]any{"accountId": t.SignalAccountID, "type": t.SignalType, "match": t.SignalMatch})
	}
	return marshalString(map[string]int64{"at": t.At})
}

func (s *Store) CreateTask(ctx context.Context, t Task) (Task, error) {
	t.ID, t.CreatedAt = ids.New(), now()
	spec, err := taskSpec(t)
	if err != nil {
		return t, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO tasks (`+taskColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.AgentID, t.Name, t.Purpose, t.Kind, spec, t.Enabled, t.NextFireAt, t.LastFiredAt, t.CreatedAt)
	return t, err
}

// SaveTask writes back a task's editable fields and schedule state.
func (s *Store) SaveTask(ctx context.Context, t Task) error {
	spec, err := taskSpec(t)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE tasks SET name = ?, purpose = ?, kind = ?, spec = ?, enabled = ?,
		next_fire_at = ?, last_fired_at = ? WHERE id = ?`,
		t.Name, t.Purpose, t.Kind, spec, t.Enabled, t.NextFireAt, t.LastFiredAt, t.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetTask(ctx context.Context, id string) (Task, error) {
	t, err := scanTask(s.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

func (s *Store) DeleteTask(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Tasks returns an agent's tasks, oldest first.
func (s *Store) Tasks(ctx context.Context, agentID string) ([]Task, error) {
	return s.queryTasks(ctx, `SELECT `+taskColumns+` FROM tasks WHERE agent_id = ? ORDER BY id`, agentID)
}

// DueTasks returns enabled tasks whose next run is at or before t.
func (s *Store) DueTasks(ctx context.Context, t int64) ([]Task, error) {
	return s.queryTasks(ctx, `SELECT `+taskColumns+` FROM tasks
		WHERE enabled = 1 AND next_fire_at IS NOT NULL AND next_fire_at <= ?
		ORDER BY next_fire_at`, t)
}

// NextTaskFireAt returns the earliest upcoming run of any enabled task (0 if none).
func (s *Store) NextTaskFireAt(ctx context.Context) (int64, error) {
	var next sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MIN(next_fire_at) FROM tasks WHERE enabled = 1`).Scan(&next)
	return next.Int64, err
}

func (s *Store) queryTasks(ctx context.Context, q string, args ...any) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ClaimTaskFire records that a due task fired (its new schedule state in t), but only if its
// next run is still prevNext, so two schedulers can never fire the same run twice. It reports
// whether this caller won the claim.
func (s *Store) ClaimTaskFire(ctx context.Context, t Task, prevNext int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE tasks SET enabled = ?, next_fire_at = ?, last_fired_at = ?
		WHERE id = ? AND next_fire_at = ?`, t.Enabled, t.NextFireAt, t.LastFiredAt, t.ID, prevNext)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

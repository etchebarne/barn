package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/etchebarne/barn/internal/ids"
)

type Agent struct {
	ID            string
	Name          string
	Instructions  string
	Model         string
	Language      string
	Notifications bool
	TrustMode     string
	IsAdmin       bool
	CreatedAt     int64
}

const agentColumns = `id, name, instructions, model, language, notifications, trust_mode, is_admin, created_at`

func scanAgent(row interface{ Scan(...any) error }) (Agent, error) {
	var a Agent
	err := row.Scan(&a.ID, &a.Name, &a.Instructions, &a.Model, &a.Language,
		&a.Notifications, &a.TrustMode, &a.IsAdmin, &a.CreatedAt)
	return a, err
}

func (s *Store) ListAgents(ctx context.Context) ([]Agent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+agentColumns+` FROM agents WHERE archived_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAgent(ctx context.Context, id string) (Agent, error) {
	a, err := scanAgent(s.db.QueryRowContext(ctx,
		`SELECT `+agentColumns+` FROM agents WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// CreateAgentWithDM creates an agent and its DM chat. Returns the agent and the DM chat id.
func (s *Store) CreateAgentWithDM(ctx context.Context, a Agent) (Agent, string, error) {
	a.ID = ids.New()
	a.CreatedAt = now()
	chatID := ids.New()
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO agents (`+agentColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, a.Name, a.Instructions, a.Model, a.Language, a.Notifications,
			a.TrustMode, a.IsAdmin, a.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chats (id, kind, name, created_at) VALUES (?, 'dm', ?, ?)`,
			chatID, a.Name, a.CreatedAt); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO chat_members (chat_id, agent_id, position) VALUES (?, ?, 0)`, chatID, a.ID)
		return err
	})
	return a, chatID, err
}

func (s *Store) UpdateAgentModel(ctx context.Context, id, model string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE agents SET model = ? WHERE id = ?`, model, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetAgentAdmin(ctx context.Context, id string, admin bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agents SET is_admin = ? WHERE id = ?`, admin, id)
	return err
}

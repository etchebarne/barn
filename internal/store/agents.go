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
	SandboxID     *string
}

const agentColumns = `id, name, instructions, model, language, notifications, trust_mode, is_admin, created_at, sandbox_id`

func scanAgent(row interface{ Scan(...any) error }) (Agent, error) {
	var a Agent
	err := row.Scan(&a.ID, &a.Name, &a.Instructions, &a.Model, &a.Language,
		&a.Notifications, &a.TrustMode, &a.IsAdmin, &a.CreatedAt, &a.SandboxID)
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
			INSERT INTO agents (`+agentColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, a.Name, a.Instructions, a.Model, a.Language, a.Notifications,
			a.TrustMode, a.IsAdmin, a.CreatedAt, a.SandboxID); err != nil {
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

func (s *Store) SetAgentAdmin(ctx context.Context, id string, admin bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agents SET is_admin = ? WHERE id = ?`, admin, id)
	return err
}

// AgentUpdate changes an agent's settings; nil fields are left alone.
type AgentUpdate struct {
	Name          *string
	Instructions  *string
	Model         *string
	Language      *string
	TrustMode     *string
	Notifications *bool
}

func (s *Store) UpdateAgent(ctx context.Context, id string, u AgentUpdate) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM agents WHERE id = ? AND archived_at IS NULL`, id).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrNotFound
		}
		set := func(column string, v *string) error {
			if v == nil {
				return nil
			}
			_, err := tx.ExecContext(ctx, `UPDATE agents SET `+column+` = ? WHERE id = ?`, *v, id)
			return err
		}
		for _, f := range []struct {
			col string
			v   *string
		}{{"name", u.Name}, {"instructions", u.Instructions}, {"model", u.Model}, {"language", u.Language}, {"trust_mode", u.TrustMode}} {
			if err := set(f.col, f.v); err != nil {
				return err
			}
		}
		if u.Notifications != nil {
			if _, err := tx.ExecContext(ctx, `UPDATE agents SET notifications = ? WHERE id = ?`, *u.Notifications, id); err != nil {
				return err
			}
		}
		// A DM is named after its agent.
		if u.Name != nil {
			if _, err := tx.ExecContext(ctx, `
				UPDATE chats SET name = ? WHERE kind = 'dm'
				AND id IN (SELECT chat_id FROM chat_members WHERE agent_id = ?)`, *u.Name, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// ErrLastAdmin is returned when archiving would leave no admin agent.
var ErrLastAdmin = errors.New("the last admin agent can't be archived")

// ArchiveAgent marks an agent archived. Its DM is hidden from the chat list.
func (s *Store) ArchiveAgent(ctx context.Context, id string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var admin bool
		err := tx.QueryRowContext(ctx,
			`SELECT is_admin FROM agents WHERE id = ? AND archived_at IS NULL`, id).Scan(&admin)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if admin {
			var admins int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM agents WHERE is_admin = 1 AND archived_at IS NULL`).Scan(&admins); err != nil {
				return err
			}
			if admins <= 1 {
				return ErrLastAdmin
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE agents SET archived_at = ? WHERE id = ?`, now(), id)
		return err
	})
}

// SandboxFor returns the agent's sandbox id, creating its own sandbox on first use.
func (s *Store) SandboxFor(ctx context.Context, agentID string) (string, error) {
	var id string
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var current sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT sandbox_id FROM agents WHERE id = ?`, agentID).Scan(&current)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if current.Valid {
			id = current.String
			return nil
		}
		id = ids.New()
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandboxes (id, created_at) VALUES (?, ?)`, id, now()); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE agents SET sandbox_id = ? WHERE id = ?`, id, agentID)
		return err
	})
	return id, err
}

// ShareSandbox moves agentID into withAgentID's sandbox (creating it if needed).
func (s *Store) ShareSandbox(ctx context.Context, agentID, withAgentID string) error {
	target, err := s.SandboxFor(ctx, withAgentID)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE agents SET sandbox_id = ? WHERE id = ? AND archived_at IS NULL`, target, agentID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SandboxMembers returns the active agents using a sandbox.
func (s *Store) SandboxMembers(ctx context.Context, sandboxID string) ([]Agent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+agentColumns+` FROM agents WHERE sandbox_id = ? AND archived_at IS NULL ORDER BY id`, sandboxID)
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

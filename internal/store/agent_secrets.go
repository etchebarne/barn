package store

import "context"

// AgentSecret is a secret the user gave an agent, without its value.
type AgentSecret struct {
	Name        string
	Description string
	CreatedAt   int64
	UpdatedAt   int64
}

// SetAgentSecret saves (or replaces) a secret; sealed is its encrypted value. An empty
// description keeps the current one.
func (s *Store) SetAgentSecret(ctx context.Context, agentID, name, description, sealed string) error {
	t := now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_secrets (agent_id, name, description, value, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (agent_id, name) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at,
			description = CASE WHEN excluded.description = '' THEN agent_secrets.description ELSE excluded.description END`,
		agentID, name, description, sealed, t, t)
	return err
}

// AgentSecrets lists an agent's secrets by name, without values.
func (s *Store) AgentSecrets(ctx context.Context, agentID string) ([]AgentSecret, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, description, created_at, updated_at FROM agent_secrets
		WHERE agent_id = ? ORDER BY name`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentSecret
	for rows.Next() {
		var a AgentSecret
		if err := rows.Scan(&a.Name, &a.Description, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SealedAgentSecrets returns an agent's secrets' encrypted values by name.
func (s *Store) SealedAgentSecrets(ctx context.Context, agentID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, value FROM agent_secrets WHERE agent_id = ?`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, rows.Err()
}

// DeleteAgentSecret removes a secret.
func (s *Store) DeleteAgentSecret(ctx context.Context, agentID, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM agent_secrets WHERE agent_id = ? AND name = ?`, agentID, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

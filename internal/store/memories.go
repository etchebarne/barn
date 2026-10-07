package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/etchebarne/openbot/internal/ids"
)

type Memory struct {
	ID           string
	AgentID      string
	Text         string
	SourceChatID *string
	CreatedAt    int64
}

func (s *Store) AddMemory(ctx context.Context, agentID, text string, sourceChatID *string) (Memory, error) {
	m := Memory{ID: ids.New(), AgentID: agentID, Text: text, SourceChatID: sourceChatID, CreatedAt: now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO memories (id, agent_id, text, source_chat_id, created_at) VALUES (?, ?, ?, ?, ?)`,
		m.ID, m.AgentID, m.Text, m.SourceChatID, m.CreatedAt)
	return m, err
}

func (s *Store) Memories(ctx context.Context, agentID string) ([]Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, text, source_chat_id, created_at FROM memories WHERE agent_id = ? ORDER BY id`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Memory
	for rows.Next() {
		var m Memory
		if err := rows.Scan(&m.ID, &m.AgentID, &m.Text, &m.SourceChatID, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMemory removes one of an agent's memories.
func (s *Store) DeleteMemory(ctx context.Context, agentID, memoryID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM memories WHERE id = ? AND agent_id = ?`, memoryID, agentID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ContextSummary returns the agent's rolling summary of compacted context ("" if none).
func (s *Store) ContextSummary(ctx context.Context, agentID string) (string, error) {
	var content string
	err := s.db.QueryRowContext(ctx, `SELECT content FROM context_summaries WHERE agent_id = ?`, agentID).Scan(&content)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return content, err
}

// Compact replaces the agent's context entries before keepFromID with summary, atomically.
func (s *Store) Compact(ctx context.Context, agentID, keepFromID, summary string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM context_entries WHERE agent_id = ? AND id < ?`, agentID, keepFromID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO context_summaries (agent_id, content, updated_at) VALUES (?, ?, ?)
			ON CONFLICT (agent_id) DO UPDATE SET content = excluded.content, updated_at = excluded.updated_at`,
			agentID, summary, now())
		return err
	})
}

// ClearContextSummary drops the agent's summary of compacted context.
func (s *Store) ClearContextSummary(ctx context.Context, agentID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM context_summaries WHERE agent_id = ?`, agentID)
	return err
}

package store

import (
	"context"

	"github.com/etchebarne/openbot/internal/ids"
)

// UsageRecord is one model call's cost.
type UsageRecord struct {
	AgentID          string
	Model            string
	Purpose          string // "turn" | "task" | "compaction" | "subagent"
	PromptTokens     int
	CachedTokens     int
	CacheWriteTokens int
	CompletionTokens int
	ReasoningTokens  int
	CreatedAt        int64
}

// RecordUsage saves a model call's cost.
func (s *Store) RecordUsage(ctx context.Context, u UsageRecord) error {
	if u.CreatedAt == 0 {
		u.CreatedAt = now()
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO model_usage (id, agent_id, model, purpose, prompt_tokens,
		cached_tokens, cache_write_tokens, completion_tokens, reasoning_tokens, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ids.New(), nullIfEmpty(u.AgentID), u.Model, u.Purpose, u.PromptTokens, u.CachedTokens,
		u.CacheWriteTokens, u.CompletionTokens, u.ReasoningTokens, u.CreatedAt)
	return err
}

// UsageSince returns model calls since a time (unix ms), oldest first; agentID "" means all.
func (s *Store) UsageSince(ctx context.Context, agentID string, since int64) ([]UsageRecord, error) {
	q := `SELECT COALESCE(agent_id, ''), model, purpose, prompt_tokens, cached_tokens, cache_write_tokens,
		completion_tokens, reasoning_tokens, created_at FROM model_usage WHERE created_at >= ?`
	args := []any{since}
	if agentID != "" {
		q += ` AND agent_id = ?`
		args = append(args, agentID)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY created_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UsageRecord
	for rows.Next() {
		var u UsageRecord
		if err := rows.Scan(&u.AgentID, &u.Model, &u.Purpose, &u.PromptTokens, &u.CachedTokens,
			&u.CacheWriteTokens, &u.CompletionTokens, &u.ReasoningTokens, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// LastPromptTokens is the input size of an agent's latest main-conversation call (0 if none).
func (s *Store) LastPromptTokens(ctx context.Context, agentID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT prompt_tokens FROM model_usage
		WHERE agent_id = ? AND purpose = 'turn' ORDER BY created_at DESC, rowid DESC LIMIT 1), 0)`, agentID).Scan(&n)
	return n, err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

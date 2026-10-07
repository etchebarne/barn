package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/etchebarne/barn/internal/ids"
)

// Event is an item in an agent's inbox.
type Event struct {
	ID        string
	AgentID   string
	Kind      string
	Payload   json.RawMessage
	CreatedAt int64
}

// ContextEntry is one model-format message in an agent's working context.
type ContextEntry struct {
	ID    string
	Entry json.RawMessage
}

func (s *Store) InsertEvent(ctx context.Context, agentID, kind string, payload any) (Event, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	e := Event{ID: ids.New(), AgentID: agentID, Kind: kind, Payload: b, CreatedAt: now()}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO events (id, agent_id, kind, payload, created_at) VALUES (?, ?, ?, ?, ?)`,
		e.ID, e.AgentID, e.Kind, string(e.Payload), e.CreatedAt)
	return e, err
}

// PendingEvents returns unconsumed events for an agent, oldest first.
func (s *Store) PendingEvents(ctx context.Context, agentID string) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, kind, payload, created_at FROM events
		WHERE agent_id = ? AND consumed_at IS NULL ORDER BY id`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var payload string
		if err := rows.Scan(&e.ID, &e.AgentID, &e.Kind, &payload, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.Payload = json.RawMessage(payload)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ConsumeEvents marks events consumed and appends the given context entries, atomically.
func (s *Store) ConsumeEvents(ctx context.Context, agentID string, eventIDs []string, entries []json.RawMessage) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		t := now()
		for _, id := range eventIDs {
			if _, err := tx.ExecContext(ctx,
				`UPDATE events SET consumed_at = ? WHERE id = ? AND agent_id = ?`, t, id, agentID); err != nil {
				return err
			}
		}
		return insertContext(ctx, tx, agentID, entries, t)
	})
}

func (s *Store) AppendContext(ctx context.Context, agentID string, entries ...json.RawMessage) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		return insertContext(ctx, tx, agentID, entries, now())
	})
}

func insertContext(ctx context.Context, tx *sql.Tx, agentID string, entries []json.RawMessage, t int64) error {
	for _, e := range entries {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO context_entries (id, agent_id, entry, created_at) VALUES (?, ?, ?, ?)`,
			ids.New(), agentID, string(e), t); err != nil {
			return err
		}
	}
	return nil
}

// Context returns an agent's working context in order.
func (s *Store) Context(ctx context.Context, agentID string) ([]ContextEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, entry FROM context_entries WHERE agent_id = ? ORDER BY id`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ContextEntry
	for rows.Next() {
		var c ContextEntry
		var entry string
		if err := rows.Scan(&c.ID, &entry); err != nil {
			return nil, err
		}
		c.Entry = json.RawMessage(entry)
		out = append(out, c)
	}
	return out, rows.Err()
}

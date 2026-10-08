package store

import (
	"context"
	"strings"
	"unicode"
)

// Snippet markers: the matched words in a search snippet sit between these (control
// characters, so they can't come from a message).
const (
	MatchStart = "\x02"
	MatchEnd   = "\x03"
)

// MessageHit is a message that matches a search, with a snippet around the match.
type MessageHit struct {
	ID            string
	ChatID        string
	AuthorKind    string
	AuthorAgentID *string
	CreatedAt     int64
	Snippet       string
}

// MemoryHit is a memory that matches a search.
type MemoryHit struct {
	ID      string
	AgentID string
	Text    string
}

// ftsQuery turns what the user typed into an FTS5 query: every word must appear, each as a
// prefix ("deplo" finds "deploys"). Words are quoted, so FTS syntax in them is just text.
func ftsQuery(q string) string {
	var terms []string
	for _, w := range strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		terms = append(terms, `"`+w+`"*`)
	}
	return strings.Join(terms, " ")
}

// SearchMessages finds messages (not system notices) containing every word of q, best matches
// first.
func (s *Store) SearchMessages(ctx context.Context, q string, limit int) ([]MessageHit, error) {
	match := ftsQuery(q)
	if match == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.chat_id, m.author_kind, m.author_agent_id, m.created_at,
		       snippet(messages_fts, 0, char(2), char(3), '…', 16)
		FROM messages_fts
		JOIN messages m ON m.rowid = messages_fts.rowid
		WHERE messages_fts MATCH ? AND m.author_kind != 'system'
		ORDER BY bm25(messages_fts), m.created_at DESC
		LIMIT ?`, match, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MessageHit
	for rows.Next() {
		var h MessageHit
		if err := rows.Scan(&h.ID, &h.ChatID, &h.AuthorKind, &h.AuthorAgentID, &h.CreatedAt, &h.Snippet); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// likeWords turns q into LIKE patterns, one per word (LIKE is case-insensitive for ASCII).
func likeWords(q string) []string {
	var out []string
	for _, w := range strings.Fields(q) {
		w = strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(w)
		out = append(out, "%"+w+"%")
	}
	return out
}

// SearchMemories finds memories containing every word of q.
func (s *Store) SearchMemories(ctx context.Context, q string, limit int) ([]MemoryHit, error) {
	words := likeWords(q)
	if len(words) == 0 {
		return nil, nil
	}
	where := strings.Repeat(` AND text LIKE ? ESCAPE '\'`, len(words))[5:]
	args := make([]any, 0, len(words)+1)
	for _, w := range words {
		args = append(args, w)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, agent_id, text FROM memories WHERE `+where+
		` ORDER BY id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemoryHit
	for rows.Next() {
		var h MemoryHit
		if err := rows.Scan(&h.ID, &h.AgentID, &h.Text); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// SearchTasks finds tasks whose name or purpose contains every word of q.
func (s *Store) SearchTasks(ctx context.Context, q string, limit int) ([]Task, error) {
	words := likeWords(q)
	if len(words) == 0 {
		return nil, nil
	}
	where := strings.Repeat(` AND (name || ' ' || purpose) LIKE ? ESCAPE '\'`, len(words))[5:]
	args := make([]any, 0, len(words)+1)
	for _, w := range words {
		args = append(args, w)
	}
	return s.queryTasks(ctx, `SELECT `+taskColumns+` FROM tasks WHERE `+where+` ORDER BY id DESC LIMIT ?`,
		append(args, limit)...)
}

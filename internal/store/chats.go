package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/etchebarne/barn/internal/ids"
)

type Chat struct {
	ID          string
	Kind        string // "dm" | "group"
	Name        string
	CreatedAt   int64
	Members     []ChatMember
	UnreadCount int
	LastMessage *Message
}

type ChatMember struct {
	AgentID  string
	Position int
}

type Message struct {
	ID            string
	ChatID        string
	AuthorKind    string // "user" | "agent" | "system"
	AuthorAgentID *string
	Body          string
	CreatedAt     int64
	// Failure is set on system messages that report a failed agent turn.
	Failure *Failure
}

// Failure describes a failed agent turn.
type Failure struct {
	AgentID   string `json:"agentId"`
	Reason    string `json:"reason"`
	Retryable bool   `json:"retryable"`
}

const readerUser = "user"

// ListChats returns every chat with members, the user's unread count, and the last message,
// most recently active first.
func (s *Store) ListChats(ctx context.Context) ([]Chat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.kind, c.name, c.created_at,
		       (SELECT COUNT(*) FROM messages m
		         WHERE m.chat_id = c.id AND m.author_kind != 'user'
		           AND m.id > COALESCE((SELECT last_message_id FROM reads r
		                                 WHERE r.chat_id = c.id AND r.reader = ?), '')) AS unread
		FROM chats c`, readerUser)
	if err != nil {
		return nil, err
	}
	var chats []Chat
	for rows.Next() {
		var c Chat
		if err := rows.Scan(&c.ID, &c.Kind, &c.Name, &c.CreatedAt, &c.UnreadCount); err != nil {
			rows.Close()
			return nil, err
		}
		chats = append(chats, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range chats {
		if err := s.fillChat(ctx, &chats[i]); err != nil {
			return nil, err
		}
	}
	sortChatsByActivity(chats)
	return chats, nil
}

func (s *Store) GetChat(ctx context.Context, id string) (Chat, error) {
	var c Chat
	err := s.db.QueryRowContext(ctx,
		`SELECT id, kind, name, created_at FROM chats WHERE id = ?`, id).
		Scan(&c.ID, &c.Kind, &c.Name, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	return c, s.fillMembers(ctx, &c)
}

// AgentChats returns the chats an agent belongs to (with members).
func (s *Store) AgentChats(ctx context.Context, agentID string) ([]Chat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.kind, c.name, c.created_at
		FROM chats c JOIN chat_members cm ON cm.chat_id = c.id
		WHERE cm.agent_id = ? ORDER BY c.id`, agentID)
	if err != nil {
		return nil, err
	}
	var chats []Chat
	for rows.Next() {
		var c Chat
		if err := rows.Scan(&c.ID, &c.Kind, &c.Name, &c.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		chats = append(chats, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range chats {
		if err := s.fillMembers(ctx, &chats[i]); err != nil {
			return nil, err
		}
	}
	return chats, nil
}

// DMChatID returns the id of an agent's DM chat.
func (s *Store) DMChatID(ctx context.Context, agentID string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id FROM chats c JOIN chat_members cm ON cm.chat_id = c.id
		WHERE c.kind = 'dm' AND cm.agent_id = ?`, agentID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

func (s *Store) fillChat(ctx context.Context, c *Chat) error {
	if err := s.fillMembers(ctx, c); err != nil {
		return err
	}
	m, err := scanMessage(s.db.QueryRowContext(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE chat_id = ? ORDER BY id DESC LIMIT 1`, c.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	c.LastMessage = &m
	return nil
}

func (s *Store) fillMembers(ctx context.Context, c *Chat) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT agent_id, position FROM chat_members WHERE chat_id = ? ORDER BY position`, c.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	c.Members = c.Members[:0]
	for rows.Next() {
		var m ChatMember
		if err := rows.Scan(&m.AgentID, &m.Position); err != nil {
			return err
		}
		c.Members = append(c.Members, m)
	}
	return rows.Err()
}

func sortChatsByActivity(chats []Chat) {
	activity := func(c Chat) string {
		if c.LastMessage != nil {
			return c.LastMessage.ID
		}
		return c.ID
	}
	slices.SortFunc(chats, func(a, b Chat) int { return strings.Compare(activity(b), activity(a)) })
}

const messageColumns = `id, chat_id, author_kind, author_agent_id, body, created_at, failure`

func scanMessage(row interface{ Scan(...any) error }) (Message, error) {
	var m Message
	var failure sql.NullString
	err := row.Scan(&m.ID, &m.ChatID, &m.AuthorKind, &m.AuthorAgentID, &m.Body, &m.CreatedAt, &failure)
	if err == nil && failure.Valid {
		m.Failure = &Failure{}
		err = json.Unmarshal([]byte(failure.String), m.Failure)
	}
	return m, err
}

func (s *Store) InsertMessage(ctx context.Context, chatID, authorKind string, authorAgentID *string, body string) (Message, error) {
	return s.insertMessage(ctx, Message{ChatID: chatID, AuthorKind: authorKind, AuthorAgentID: authorAgentID, Body: body})
}

// InsertFailure posts a system message reporting a failed agent turn.
func (s *Store) InsertFailure(ctx context.Context, chatID, body string, f Failure) (Message, error) {
	return s.insertMessage(ctx, Message{ChatID: chatID, AuthorKind: "system", Body: body, Failure: &f})
}

func (s *Store) insertMessage(ctx context.Context, m Message) (Message, error) {
	m.ID, m.CreatedAt = ids.New(), now()
	var failure *string
	if m.Failure != nil {
		b, err := json.Marshal(m.Failure)
		if err != nil {
			return m, err
		}
		f := string(b)
		failure = &f
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO messages (`+messageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.ChatID, m.AuthorKind, m.AuthorAgentID, m.Body, m.CreatedAt, failure)
	return m, err
}

// LastMessage returns the most recent message in a chat.
func (s *Store) LastMessage(ctx context.Context, chatID string) (Message, error) {
	m, err := scanMessage(s.db.QueryRowContext(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE chat_id = ? ORDER BY id DESC LIMIT 1`, chatID))
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

func (s *Store) GetMessage(ctx context.Context, id string) (Message, error) {
	m, err := scanMessage(s.db.QueryRowContext(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

// ListMessages returns up to limit messages older than before (or the latest if before is
// empty), in chronological order, and whether older messages exist.
func (s *Store) ListMessages(ctx context.Context, chatID, before string, limit int) ([]Message, bool, error) {
	q := `SELECT ` + messageColumns + ` FROM messages WHERE chat_id = ?`
	args := []any{chatID}
	if before != "" {
		q += ` AND id < ?`
		args = append(args, before)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, hasMore, nil
}

// MarkRead records that the user has read chatID up to lastMessageID. Never moves backwards.
func (s *Store) MarkRead(ctx context.Context, chatID, lastMessageID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO reads (chat_id, reader, last_message_id) VALUES (?, ?, ?)
		ON CONFLICT (chat_id, reader) DO UPDATE SET last_message_id = excluded.last_message_id
		WHERE excluded.last_message_id > reads.last_message_id`, chatID, readerUser, lastMessageID)
	return err
}

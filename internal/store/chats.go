package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/etchebarne/openbot/internal/ids"
)

type Chat struct {
	ID          string
	Kind        string // "dm" | "group"
	Name        string
	CreatedAt   int64
	Members     []ChatMember
	UnreadCount int
	LastMessage *Message
	// Where the user put it in the sidebar (nil: Unassigned / not arranged yet).
	CategoryID *string
	Position   *int
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
	// Prompt is set on agent messages that ask the user something.
	Prompt *Prompt
	// Event is set on system messages that mark something that happened.
	Event *MessageEvent
	// Reactions are filled by ListMessages and GetMessage.
	Reactions []Reaction
	// Mentions are the agent ids @mentioned in the message.
	Mentions []string
	// Attachments are filled by ListMessages, GetMessage and MessagesAfter; on insert, the
	// unsent uploads with these ids are attached.
	Attachments []Attachment
	attachIDs   []string
	// ReplyTo is the earlier message in the same chat this one replies to; Quoted is that
	// message, filled with Attachments (nil if it no longer exists).
	ReplyTo *string
	Quoted  *Message
}

// Reaction is one emoji on a message and who used it. Reactors are "user" or "agent:<id>".
type Reaction struct {
	Emoji    string
	Reactors []string
}

// Prompt is a question with clickable answers.
type Prompt struct {
	Kind       string         `json:"kind"` // "single" | "multi" | "text"
	Question   string         `json:"question"`
	Options    []PromptOption `json:"options"`
	AllowOther bool           `json:"allowOther"`
	Status     string         `json:"status"` // "pending" | "answered" | "dismissed"
	Answer     *PromptAnswer  `json:"answer"`
	// Action is the gated tool call an approval prompt is about. Never sent to clients.
	Action *PendingAction `json:"action,omitempty"`
	// Connection is the app a connect prompt proposes.
	Connection *PendingConnection `json:"connection,omitempty"`
	// Preview describes the action an approval prompt is about, for the card.
	Preview *ActionPreview `json:"preview,omitempty"`
}

// ActionPreview is what an approval card shows: the app, what the action is, its arguments
// and the label for the approve button.
type ActionPreview struct {
	AppType string         `json:"appType,omitempty"` // connector type; empty for openbot's own actions
	AppName string         `json:"appName,omitempty"`
	Title   string         `json:"title"`
	Verb    string         `json:"verb"`
	Note    string         `json:"note,omitempty"`
	Fields  []PreviewField `json:"fields"`
	Body    *PreviewField  `json:"body,omitempty"` // the main content, shown large
}

type PreviewField struct {
	Key   string          `json:"key"`
	Label string          `json:"label"`
	Value json.RawMessage `json:"value"`
}

// PendingConnection is a connection an agent proposed (kind "connect"). It holds no secrets: the
// user types those on the card and they go straight into the connection.
type PendingConnection struct {
	AgentID   string            `json:"agentId"` // who proposed it
	Type      string            `json:"type"`
	Name      string            `json:"name"`
	Config    map[string]string `json:"config"`
	AgentIDs  []string          `json:"agentIds"`
	AccountID string            `json:"accountId,omitempty"` // set once connected
	SignIn    bool              `json:"signIn,omitempty"`    // the server uses OAuth sign-in
}

type PromptOption struct {
	Label       string `json:"label"`
	OpensChatID string `json:"opensChatId,omitempty"`
}

// PendingAction is a gated tool call waiting for the user's approval (kind "approval").
type PendingAction struct {
	AgentID string          `json:"agentId"`
	Tool    string          `json:"tool"`
	Args    json.RawMessage `json:"args"`
	// Rule is what "Always allow" (option 2) saves, for actions that can be always allowed.
	Rule *StandingRule `json:"rule,omitempty"`
}

// StandingRule identifies an action for a standing approval (see ApprovalRule).
type StandingRule struct {
	Action string `json:"action"`
	Label  string `json:"label"`
}

type PromptAnswer struct {
	Selected []int  `json:"selected,omitempty"`
	Text     string `json:"text,omitempty"`
}

// MessageEvent marks something that happened, e.g. {"kind": "agent_created"}.
type MessageEvent struct {
	Kind    string `json:"kind"`
	AgentID string `json:"agentId"`
	ChatID  string `json:"chatId,omitempty"`
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
	return s.chatSummaries(ctx, "", nil)
}

// ChatSummary returns one chat as ListChats would (members, unread count, last message).
func (s *Store) ChatSummary(ctx context.Context, id string) (Chat, error) {
	chats, err := s.chatSummaries(ctx, "AND c.id = ?", []any{id})
	if err != nil {
		return Chat{}, err
	}
	if len(chats) == 0 {
		return Chat{}, ErrNotFound
	}
	return chats[0], nil
}

func (s *Store) chatSummaries(ctx context.Context, filter string, args []any) ([]Chat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.kind, c.name, c.created_at, c.category_id, c.position,
		       (SELECT COUNT(*) FROM messages m
		         WHERE m.chat_id = c.id AND m.author_kind != 'user'
		           AND m.id > COALESCE((SELECT last_message_id FROM reads r
		                                 WHERE r.chat_id = c.id AND r.reader = ?), '')) AS unread
		FROM chats c
		-- DMs of archived agents are hidden.
		WHERE NOT (c.kind = 'dm' AND EXISTS (
			SELECT 1 FROM chat_members cm JOIN agents a ON a.id = cm.agent_id
			WHERE cm.chat_id = c.id AND a.archived_at IS NOT NULL)) `+filter,
		append([]any{readerUser}, args...)...)
	if err != nil {
		return nil, err
	}
	var chats []Chat
	for rows.Next() {
		var c Chat
		if err := rows.Scan(&c.ID, &c.Kind, &c.Name, &c.CreatedAt, &c.CategoryID, &c.Position, &c.UnreadCount); err != nil {
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

const messageColumns = `id, chat_id, author_kind, author_agent_id, body, created_at, failure, prompt, event, mentions, reply_to`

func scanMessage(row interface{ Scan(...any) error }) (Message, error) {
	var m Message
	var failure, prompt, event, mentions sql.NullString
	if err := row.Scan(&m.ID, &m.ChatID, &m.AuthorKind, &m.AuthorAgentID, &m.Body, &m.CreatedAt,
		&failure, &prompt, &event, &mentions, &m.ReplyTo); err != nil {
		return m, err
	}
	if mentions.Valid {
		if err := json.Unmarshal([]byte(mentions.String), &m.Mentions); err != nil {
			return m, err
		}
	}
	if err := unmarshalNullable(failure, &m.Failure); err != nil {
		return m, err
	}
	if err := unmarshalNullable(prompt, &m.Prompt); err != nil {
		return m, err
	}
	return m, unmarshalNullable(event, &m.Event)
}

func unmarshalNullable[T any](s sql.NullString, dst **T) error {
	if !s.Valid {
		return nil
	}
	*dst = new(T)
	return json.Unmarshal([]byte(s.String), *dst)
}

func marshalNullable[T any](v *T) (*string, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	out := string(b)
	return &out, nil
}

func (s *Store) InsertMessage(ctx context.Context, chatID, authorKind string, authorAgentID *string, body string, mentions ...string) (Message, error) {
	return s.insertMessage(ctx, Message{ChatID: chatID, AuthorKind: authorKind, AuthorAgentID: authorAgentID, Body: body, Mentions: mentions})
}

// NewMessage is a user or agent message to post with InsertFull.
type NewMessage struct {
	ChatID        string
	AuthorKind    string
	AuthorAgentID *string
	Body          string
	Mentions      []string
	// AttachmentIDs are unsent uploads from the same chat, attached in order
	// (ErrBadAttachment if any isn't one).
	AttachmentIDs []string
	// ReplyTo is an earlier message in the same chat (ErrBadReply if it isn't one).
	ReplyTo string
}

// ErrBadReply means a reply's message isn't in the same chat.
var ErrBadReply = errors.New("the message replied to isn't in this chat")

// InsertMessageWithAttachments posts a message and attaches unsent uploads from the same chat
// to it, in order (ErrBadAttachment if any isn't one).
func (s *Store) InsertMessageWithAttachments(ctx context.Context, chatID, authorKind string, authorAgentID *string, body string, attachmentIDs []string, mentions ...string) (Message, error) {
	return s.InsertFull(ctx, NewMessage{ChatID: chatID, AuthorKind: authorKind, AuthorAgentID: authorAgentID,
		Body: body, Mentions: mentions, AttachmentIDs: attachmentIDs})
}

// InsertFull posts a message with attachments and what it replies to, and returns it filled.
func (s *Store) InsertFull(ctx context.Context, n NewMessage) (Message, error) {
	m := Message{ChatID: n.ChatID, AuthorKind: n.AuthorKind, AuthorAgentID: n.AuthorAgentID, Body: n.Body,
		Mentions: n.Mentions, attachIDs: n.AttachmentIDs}
	if n.ReplyTo != "" {
		var chatID string
		err := s.db.QueryRowContext(ctx, `SELECT chat_id FROM messages WHERE id = ?`, n.ReplyTo).Scan(&chatID)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && chatID != n.ChatID) {
			return m, ErrBadReply
		}
		if err != nil {
			return m, err
		}
		m.ReplyTo = &n.ReplyTo
	}
	m, err := s.insertMessage(ctx, m)
	if err != nil {
		return m, err
	}
	msgs := []Message{m}
	err = s.fillDetails(ctx, msgs, false)
	return msgs[0], err
}

// InsertPrompt posts an agent message that asks the user something.
func (s *Store) InsertPrompt(ctx context.Context, chatID, agentID string, p Prompt) (Message, error) {
	p.Status, p.Answer = "pending", nil
	return s.insertMessage(ctx, Message{ChatID: chatID, AuthorKind: "agent", AuthorAgentID: &agentID, Body: p.Question, Prompt: &p})
}

// InsertEventMessage posts a system message marking something that happened.
func (s *Store) InsertEventMessage(ctx context.Context, chatID, body string, e MessageEvent) (Message, error) {
	return s.insertMessage(ctx, Message{ChatID: chatID, AuthorKind: "system", Body: body, Event: &e})
}

// ErrPromptClosed is returned when answering or dismissing a prompt that isn't pending.
var ErrPromptClosed = errors.New("prompt is no longer pending")

// UpdatePrompt applies fn to a pending prompt atomically. fn returns the new prompt state.
func (s *Store) UpdatePrompt(ctx context.Context, messageID string, fn func(Prompt) (Prompt, error)) (Message, error) {
	var out Message
	err := s.tx(ctx, func(tx *sql.Tx) error {
		m, err := scanMessage(tx.QueryRowContext(ctx,
			`SELECT `+messageColumns+` FROM messages WHERE id = ?`, messageID))
		if errors.Is(err, sql.ErrNoRows) || (err == nil && m.Prompt == nil) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if m.Prompt.Status != "pending" {
			return ErrPromptClosed
		}
		next, err := fn(*m.Prompt)
		if err != nil {
			return err
		}
		raw, err := marshalNullable(&next)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE messages SET prompt = ? WHERE id = ?`, raw, messageID); err != nil {
			return err
		}
		m.Prompt = &next
		out = m
		return nil
	})
	return out, err
}

// InsertFailure posts a system message reporting a failed agent turn.
func (s *Store) InsertFailure(ctx context.Context, chatID, body string, f Failure) (Message, error) {
	return s.insertMessage(ctx, Message{ChatID: chatID, AuthorKind: "system", Body: body, Failure: &f})
}

func (s *Store) insertMessage(ctx context.Context, m Message) (Message, error) {
	m.ID, m.CreatedAt = ids.New(), now()
	failure, err := marshalNullable(m.Failure)
	if err != nil {
		return m, err
	}
	prompt, err := marshalNullable(m.Prompt)
	if err != nil {
		return m, err
	}
	event, err := marshalNullable(m.Event)
	if err != nil {
		return m, err
	}
	var mentions *string
	if len(m.Mentions) > 0 {
		b, err := json.Marshal(m.Mentions)
		if err != nil {
			return m, err
		}
		v := string(b)
		mentions = &v
	}
	err = s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO messages (`+messageColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			m.ID, m.ChatID, m.AuthorKind, m.AuthorAgentID, m.Body, m.CreatedAt, failure, prompt, event, mentions, m.ReplyTo); err != nil {
			return err
		}
		return attach(ctx, tx, m.ChatID, m.ID, m.attachIDs)
	})
	m.attachIDs = nil
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
	if err != nil {
		return m, err
	}
	msgs := []Message{m}
	err = s.fillDetails(ctx, msgs, true)
	return msgs[0], err
}

// ToggleReaction adds a reaction, or removes it if that reactor already used that emoji.
// It reports whether the reaction now exists.
func (s *Store) ToggleReaction(ctx context.Context, messageID, reactor, emoji string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM reactions WHERE message_id = ? AND reactor = ? AND emoji = ?`, messageID, reactor, emoji)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return false, nil
	}
	return s.AddReaction(ctx, messageID, reactor, emoji)
}

// AddReaction records a reaction. It reports false if that reactor already used that emoji.
func (s *Store) AddReaction(ctx context.Context, messageID, reactor, emoji string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO reactions (message_id, reactor, emoji, created_at) VALUES (?, ?, ?, ?)
		ON CONFLICT DO NOTHING`, messageID, reactor, emoji, now())
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// fillDetails loads what isn't in a message's row: attachments, the quoted message and,
// with reactions, its reactions.
func (s *Store) fillDetails(ctx context.Context, msgs []Message, reactions bool) error {
	if reactions {
		if err := s.fillReactions(ctx, msgs); err != nil {
			return err
		}
	}
	if err := s.fillAttachments(ctx, msgs); err != nil {
		return err
	}
	return s.fillQuoted(ctx, msgs)
}

// fillQuoted loads the messages that msgs reply to, with their attachments.
func (s *Store) fillQuoted(ctx context.Context, msgs []Message) error {
	var args []any
	for _, m := range msgs {
		if m.ReplyTo != nil {
			args = append(args, *m.ReplyTo)
		}
	}
	if len(args) == 0 {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+messageColumns+` FROM messages
		WHERE id IN (?`+strings.Repeat(",?", len(args)-1)+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	var quoted []Message
	for rows.Next() {
		q, err := scanMessage(rows)
		if err != nil {
			return err
		}
		quoted = append(quoted, q)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	if err := s.fillAttachments(ctx, quoted); err != nil {
		return err
	}
	byID := make(map[string]*Message, len(quoted))
	for i := range quoted {
		byID[quoted[i].ID] = &quoted[i]
	}
	for i := range msgs {
		if msgs[i].ReplyTo != nil {
			msgs[i].Quoted = byID[*msgs[i].ReplyTo]
		}
	}
	return nil
}

// fillReactions loads reactions for msgs, grouped by emoji in first-use order.
func (s *Store) fillReactions(ctx context.Context, msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	index := make(map[string]int, len(msgs))
	args := make([]any, 0, len(msgs))
	for i, m := range msgs {
		index[m.ID] = i
		args = append(args, m.ID)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT message_id, reactor, emoji FROM reactions
		WHERE message_id IN (?`+strings.Repeat(",?", len(args)-1)+`)
		ORDER BY created_at, rowid`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var messageID, reactor, emoji string
		if err := rows.Scan(&messageID, &reactor, &emoji); err != nil {
			return err
		}
		m := &msgs[index[messageID]]
		i := slices.IndexFunc(m.Reactions, func(r Reaction) bool { return r.Emoji == emoji })
		if i < 0 {
			m.Reactions = append(m.Reactions, Reaction{Emoji: emoji})
			i = len(m.Reactions) - 1
		}
		m.Reactions[i].Reactors = append(m.Reactions[i].Reactors, reactor)
	}
	return rows.Err()
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
	if err := s.fillDetails(ctx, out, true); err != nil {
		return nil, false, err
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

// MessagesAfter returns a chat's messages newer than afterID (all if empty), oldest first.
func (s *Store) MessagesAfter(ctx context.Context, chatID, afterID string, limit int) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+messageColumns+` FROM messages
		WHERE chat_id = ? AND id > ? ORDER BY id LIMIT ?`, chatID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	return out, s.fillDetails(ctx, out, false)
}

// ReadMarker returns how far a reader ("user" or "agent:<id>") has read a chat ("" if never).
func (s *Store) ReadMarker(ctx context.Context, chatID, reader string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx,
		`SELECT last_message_id FROM reads WHERE chat_id = ? AND reader = ?`, chatID, reader).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// MarkReadBy moves a reader's marker forward (never backwards).
func (s *Store) MarkReadBy(ctx context.Context, chatID, reader, lastMessageID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO reads (chat_id, reader, last_message_id) VALUES (?, ?, ?)
		ON CONFLICT (chat_id, reader) DO UPDATE SET last_message_id = excluded.last_message_id
		WHERE excluded.last_message_id > reads.last_message_id`, chatID, reader, lastMessageID)
	return err
}

// CreateGroup creates a group chat with the agents in order.
func (s *Store) CreateGroup(ctx context.Context, name string, agentIDs []string) (Chat, error) {
	c := Chat{ID: ids.New(), Kind: "group", Name: name, CreatedAt: now()}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chats (id, kind, name, created_at) VALUES (?, 'group', ?, ?)`, c.ID, name, c.CreatedAt); err != nil {
			return err
		}
		for i, id := range agentIDs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO chat_members (chat_id, agent_id, position) VALUES (?, ?, ?)`, c.ID, id, i); err != nil {
				return err
			}
			c.Members = append(c.Members, ChatMember{AgentID: id, Position: i})
		}
		return nil
	})
	return c, err
}

// UpdateGroup renames a group and adds or removes members.
func (s *Store) UpdateGroup(ctx context.Context, chatID string, name *string, add, remove []string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var kind string
		err := tx.QueryRowContext(ctx, `SELECT kind FROM chats WHERE id = ?`, chatID).Scan(&kind)
		if errors.Is(err, sql.ErrNoRows) || kind != "group" {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if name != nil {
			if _, err := tx.ExecContext(ctx, `UPDATE chats SET name = ? WHERE id = ?`, *name, chatID); err != nil {
				return err
			}
		}
		for _, id := range remove {
			if _, err := tx.ExecContext(ctx, `DELETE FROM chat_members WHERE chat_id = ? AND agent_id = ?`, chatID, id); err != nil {
				return err
			}
		}
		for _, id := range add {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO chat_members (chat_id, agent_id, position)
				VALUES (?, ?, (SELECT COALESCE(MAX(position), -1) + 1 FROM chat_members WHERE chat_id = ?))
				ON CONFLICT DO NOTHING`, chatID, id, chatID); err != nil {
				return err
			}
		}
		return nil
	})
}

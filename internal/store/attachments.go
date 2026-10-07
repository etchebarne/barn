package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Attachment is a file attached (or about to be) to a message.
type Attachment struct {
	ID        string
	ChatID    string
	MessageID *string // nil until sent
	Name      string
	Mime      string
	Size      int64
	Width     *int // images
	Height    *int
	CreatedAt int64
}

// ErrBadAttachment means an attachment id isn't an unsent upload in that chat.
var ErrBadAttachment = errors.New("unknown or already sent attachment")

const attachmentColumns = `id, chat_id, message_id, name, mime, size, width, height, created_at`

func scanAttachment(row interface{ Scan(...any) error }) (Attachment, error) {
	var a Attachment
	err := row.Scan(&a.ID, &a.ChatID, &a.MessageID, &a.Name, &a.Mime, &a.Size, &a.Width, &a.Height, &a.CreatedAt)
	return a, err
}

// CreateAttachment records an upload (its id is chosen by the caller, who stored the file).
func (s *Store) CreateAttachment(ctx context.Context, a Attachment) (Attachment, error) {
	a.CreatedAt = now()
	_, err := s.db.ExecContext(ctx, `INSERT INTO attachments (`+attachmentColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.ChatID, a.MessageID, a.Name, a.Mime, a.Size, a.Width, a.Height, a.CreatedAt)
	return a, err
}

func (s *Store) GetAttachment(ctx context.Context, id string) (Attachment, error) {
	a, err := scanAttachment(s.db.QueryRowContext(ctx, `SELECT `+attachmentColumns+` FROM attachments WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// attach links unsent uploads to a new message, in order.
func attach(ctx context.Context, tx *sql.Tx, chatID, messageID string, ids []string) error {
	for i, id := range ids {
		res, err := tx.ExecContext(ctx, `UPDATE attachments SET message_id = ?, position = ?
			WHERE id = ? AND chat_id = ? AND message_id IS NULL`, messageID, i, id, chatID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrBadAttachment
		}
	}
	return nil
}

// fillAttachments loads the attachments of msgs.
func (s *Store) fillAttachments(ctx context.Context, msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	index := make(map[string]int, len(msgs))
	args := make([]any, 0, len(msgs))
	for i, m := range msgs {
		index[m.ID] = i
		args = append(args, m.ID)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+attachmentColumns+` FROM attachments
		WHERE message_id IN (?`+strings.Repeat(",?", len(args)-1)+`) ORDER BY message_id, position`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return err
		}
		m := &msgs[index[*a.MessageID]]
		m.Attachments = append(m.Attachments, a)
	}
	return rows.Err()
}

// StaleUploads returns uploads never sent and older than cutoff (unix ms).
func (s *Store) StaleUploads(ctx context.Context, cutoff int64) ([]Attachment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+attachmentColumns+` FROM attachments
		WHERE message_id IS NULL AND created_at < ?`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attachment
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAttachment(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM attachments WHERE id = ?`, id)
	return err
}

// AttachmentIDs returns every attachment id (to find files on disk without a row).
func (s *Store) AttachmentIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM attachments`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

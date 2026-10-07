package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/etchebarne/openbot/internal/ids"
)

type User struct {
	ID           string
	Username     string
	PasswordHash string
}

// ErrUserExists is returned by CreateFirstUser when an account already exists.
var ErrUserExists = errors.New("an account already exists")

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateFirstUser creates the single account. It fails with ErrUserExists if any user exists.
func (s *Store) CreateFirstUser(ctx context.Context, username, passwordHash string) (User, error) {
	u := User{ID: ids.New(), Username: username, PasswordHash: passwordHash}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrUserExists
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO users (id, username, password_hash, created_at) VALUES (?, ?, ?, ?)`,
			u.ID, u.Username, u.PasswordHash, now())
		return err
	})
	return u, err
}

func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// PrimaryUser returns the single account.
func (s *Store) PrimaryUser(ctx context.Context) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash FROM users ORDER BY created_at LIMIT 1`).
		Scan(&u.ID, &u.Username, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) CreateSession(ctx context.Context, tokenHash, userID string, ttl time.Duration) error {
	t := now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		tokenHash, userID, t, t+ttl.Milliseconds())
	return err
}

// SessionUser returns the user for a valid, unexpired session and extends its expiry.
func (s *Store) SessionUser(ctx context.Context, tokenHash string, ttl time.Duration) (User, error) {
	var u User
	t := now()
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.password_hash
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ?`, tokenHash, t).
		Scan(&u.ID, &u.Username, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	if err != nil {
		return u, err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE sessions SET expires_at = ? WHERE token_hash = ?`, t+ttl.Milliseconds(), tokenHash)
	return u, err
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now())
	return err
}

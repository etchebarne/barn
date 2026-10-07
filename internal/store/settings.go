package store

import (
	"context"
	"database/sql"
	"errors"
)

// Setting keys.
const (
	SettingOpenCodeAPIKey = "provider.opencode_go.api_key" // encrypted
	SettingTimezone       = "user.timezone"                // IANA name, e.g. "America/Montevideo"
	SettingVAPIDPublic    = "push.vapid.public"
	SettingVAPIDPrivate   = "push.vapid.private" // encrypted
)

func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return v, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, now())
	return err
}

func (s *Store) CountAgents(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agents`).Scan(&n)
	return n, err
}

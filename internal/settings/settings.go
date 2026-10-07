// Package settings reads and writes app settings, encrypting secret values at rest.
package settings

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/secrets"
	"github.com/etchebarne/openbot/internal/store"
)

type Settings struct {
	store *store.Store
	box   *secrets.Box
}

func New(s *store.Store, box *secrets.Box) *Settings { return &Settings{store: s, box: box} }

// APIKey returns the decrypted OpenCode Go key, or model.ErrNoKey. It satisfies model.KeyFunc.
func (s *Settings) APIKey(ctx context.Context) (string, error) {
	sealed, err := s.store.GetSetting(ctx, store.SettingOpenCodeAPIKey)
	if errors.Is(err, store.ErrNotFound) {
		return "", model.ErrNoKey
	}
	if err != nil {
		return "", err
	}
	return s.box.Open(sealed)
}

func (s *Settings) SetAPIKey(ctx context.Context, key string) error {
	sealed, err := s.box.Seal(key)
	if err != nil {
		return err
	}
	return s.store.SetSetting(ctx, store.SettingOpenCodeAPIKey, sealed)
}

// Location is the user's time zone (set by the web app), or the server's if unknown.
func (s *Settings) Location(ctx context.Context) *time.Location {
	name, err := s.store.GetSetting(ctx, store.SettingTimezone)
	if err != nil || name == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.Local
	}
	return loc
}

// SetTimezone saves the user's IANA time zone after checking it exists.
func (s *Settings) SetTimezone(ctx context.Context, name string) error {
	if _, err := time.LoadLocation(name); err != nil || name == "" || name == "Local" {
		return fmt.Errorf("unknown time zone %q", name)
	}
	return s.store.SetSetting(ctx, store.SettingTimezone, name)
}

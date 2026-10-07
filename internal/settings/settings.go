// Package settings reads and writes app settings, encrypting secret values at rest.
package settings

import (
	"context"
	"errors"

	"github.com/etchebarne/barn/internal/model"
	"github.com/etchebarne/barn/internal/secrets"
	"github.com/etchebarne/barn/internal/store"
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

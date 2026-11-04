package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SettingKey names one setting. Every key is a constant here, so the whole
// list is in one place and a misspelt key cannot quietly make a second one.
type SettingKey string

// Settings is the handle on the setting table: single remembered values,
// such as the reader's choices. Values are text; a caller that reads one it
// does not recognise uses its own default, since a newer build may have
// written a value an older one does not know.
type Settings struct {
	db *sql.DB
}

// Settings returns the handle on the stored settings.
func (s *Store) Settings() *Settings {
	return &Settings{db: s.db}
}

// Get reads a setting, or returns fallback when it has never been set. A read
// that fails returns fallback too, with the error, so a caller can log it and
// carry on with a usable value.
func (s *Settings) Get(ctx context.Context, key SettingKey, fallback string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM setting WHERE key = ?`, string(key)).Scan(&value)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return fallback, nil
	case err != nil:
		return fallback, fmt.Errorf("reading setting %s: %w", key, err)
	}
	return value, nil
}

// Set stores a setting, replacing any value it had.
func (s *Settings) Set(ctx context.Context, key SettingKey, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO setting (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		string(key), value)
	if err != nil {
		return fmt.Errorf("writing setting %s: %w", key, err)
	}
	return nil
}

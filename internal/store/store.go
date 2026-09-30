// Package store is this application's SQLite database: one file holding the
// speaker cache and, later, playlists and remembered tracks. It knows nothing
// about Sonos or terminals -- callers hand it values and get values back.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// dirName is the directory under the user's config directory, and fileName
// the database in it. Config rather than cache: the file holds the only copy
// of a playlist, and a cache directory may be reclaimed at any time.
const (
	dirName  = "sonos-sandbox"
	fileName = "sonos.db"
)

// driverName is modernc.org/sqlite's registration, which is "sqlite" and not
// the "sqlite3" a CGo driver registers.
const driverName = "sqlite"

// pragmas are applied to every connection through the DSN rather than with
// Exec, because a pooled connection opened later would not see an Exec.
var pragmas = []string{
	"journal_mode(WAL)",   // readers do not block the writer
	"busy_timeout(5000)",  // wait for a held write lock instead of failing
	"synchronous(NORMAL)", // safe against a crash under WAL, not against power loss
	"foreign_keys(ON)",    // off by default in SQLite, for compatibility
}

// Store is an open database, migrated to the schema this build knows.
type Store struct {
	db *sql.DB
}

// DefaultPath is where the database lives when nothing says otherwise.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locating config directory: %w", err)
	}
	return filepath.Join(dir, dirName, fileName), nil
}

// Open opens the database at path, creating the file and its directory if
// they are absent, and brings the schema up to date before returning.
func Open(ctx context.Context, path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}

	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Version is the schema version the file is at, which is the number of
// migrations that have been applied to it.
func (s *Store) Version(ctx context.Context) (int, error) {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("reading schema version: %w", err)
	}
	return version, nil
}

// Cache is the handle the speaker cache works through. It is the only way to
// clear cached data, and it reaches no table but its own.
func (s *Store) Cache() *Cache {
	return &Cache{db: s.db}
}

// dsn builds the connection string. _txlock=immediate takes the write lock
// when a transaction begins rather than when it first writes, so busy_timeout
// can wait for it instead of two writers deadlocking on an upgrade.
func dsn(path string) string {
	uri := url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: url.Values{"_pragma": pragmas, "_txlock": {"immediate"}}.Encode(),
	}
	return uri.String()
}

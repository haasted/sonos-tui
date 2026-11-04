package store

import (
	"context"
	"database/sql"
	"fmt"
)

// migrations are applied in order, and the schema version is how many of them
// a file has had. Before a release the list may be edited rather than added
// to, at the price of deleting existing databases. The SQL sits flush left so
// `sqlite3 sonos.db .schema` reads properly.
var migrations = []string{
	// 1. The whole schema:
	//   - the speaker cache: the speakers last seen, and the context that
	//     judges whether they still apply;
	//   - favourites: what the reader marked, kept as the URI and DIDL a
	//     speaker is given. service, account and item_id are read from the
	//     URI, and say when two marks are the same thing;
	//   - settings: single remembered values, such as the reader's choices.
	//     Not cache: clearing the cache never reaches this table.
	`
CREATE TABLE speaker_cache (
    usn          TEXT PRIMARY KEY,
    ip           TEXT NOT NULL,
    location     TEXT NOT NULL,
    udn          TEXT NOT NULL,
    room_name    TEXT NOT NULL,
    display_name TEXT NOT NULL,
    model_name   TEXT NOT NULL,
    model_number TEXT NOT NULL
) STRICT;

CREATE TABLE speaker_cache_session (
    id       INTEGER PRIMARY KEY CHECK (id = 1),
    saved_at TEXT NOT NULL,
    subnet   TEXT NOT NULL
) STRICT;

CREATE TABLE favourite (
    id       INTEGER PRIMARY KEY,
    kind     TEXT    NOT NULL CHECK (kind IN ('track', 'album', 'playlist', 'station')),
    title    TEXT    NOT NULL,
    artist   TEXT    NOT NULL DEFAULT '',
    album    TEXT    NOT NULL DEFAULT '',
    uri      TEXT    NOT NULL,
    metadata TEXT    NOT NULL,
    service  INTEGER NOT NULL,
    account  INTEGER NOT NULL,
    item_id  TEXT    NOT NULL,
    added_at TEXT    NOT NULL,
    UNIQUE (service, account, item_id)
) STRICT;

CREATE TABLE setting (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) STRICT;
`,
}

// migrate brings the database up to the last of steps. An empty file and a
// part-migrated one arrive at the same schema by the same path, because the
// version says where to start and nothing else does.
func migrate(ctx context.Context, db *sql.DB) error {
	at, err := readVersion(ctx, db)
	if err != nil {
		return err
	}

	if at > len(migrations) {
		return fmt.Errorf("database is at schema %d, this build knows %d", at, len(migrations))
	}

	for ; at < len(migrations); at++ {
		if err := apply(ctx, db, migrations[at], at+1); err != nil {
			return fmt.Errorf("migration %d: %w", at+1, err)
		}
	}
	return nil
}

// apply runs one migration and records it in the same transaction. SQLite's
// DDL is transactional, so a crash part way leaves the file at the version it
// really has rather than at one it only started on.
func apply(ctx context.Context, db *sql.DB, step string, version int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, step); err != nil {
		return err
	}
	// PRAGMA takes no placeholder. version is a loop counter, never input.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return err
	}
	return tx.Commit()
}

// readVersion reads the schema version out of the file header, where SQLite
// keeps four bytes it never touches itself. A new file reads 0, which is the
// same as "no migration has run" and needs no bootstrap table to say so.
func readVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("reading schema version: %w", err)
	}
	return version, nil
}

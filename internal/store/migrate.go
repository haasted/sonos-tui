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
	// 2. TEMPORARY: sample favourites, to try the favourites modal before
	// albums, playlists and stations can be marked. Tracks are marked in the
	// queue. Remove once the others can be marked too.
	`
INSERT INTO favourite (kind, title, artist, album, uri, metadata, service, account, item_id, added_at) VALUES
    ('playlist', 'Awesome singles', '', '',
     'x-rincon-cpcontainer:00060000libraryplaylist%3ap.V7VYpk7FYWLeW?sid=204&flags=0&sn=2',
     '<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"><item id="00060000libraryplaylist%3ap.V7VYpk7FYWLeW" parentID="-1" restricted="true"><dc:title>Awesome singles</dc:title><upnp:class>object.container.playlistContainer</upnp:class><desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">SA_RINCON52231_X_#Svc52231-0-Token</desc></item></DIDL-Lite>',
     204, 2, 'libraryplaylist:p.V7VYpk7FYWLeW', '2026-11-04T12:00:00Z'),
    ('playlist', 'Metallica', '', '',
     'x-rincon-cpcontainer:00060000libraryplaylist%3ap.ZOAXddOUZE1LE?sid=204&flags=0&sn=2',
     '<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"><item id="00060000libraryplaylist%3ap.ZOAXddOUZE1LE" parentID="-1" restricted="true"><dc:title>Metallica</dc:title><upnp:class>object.container.playlistContainer</upnp:class><desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">SA_RINCON52231_X_#Svc52231-0-Token</desc></item></DIDL-Lite>',
     204, 2, 'libraryplaylist:p.ZOAXddOUZE1LE', '2026-11-04T12:01:00Z'),
    ('album', 'Ordensduks i Ingenmandsland', 'Claus Hempler', '',
     'x-rincon-cpcontainer:00040000album%3a1888147421?sid=204&flags=0&sn=2',
     '<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"><item id="00040000album%3a1888147421" parentID="-1" restricted="true"><dc:title>Ordensduks i Ingenmandsland</dc:title><upnp:class>object.container.album.musicAlbum</upnp:class><desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">SA_RINCON52231_X_#Svc52231-0-Token</desc></item></DIDL-Lite>',
     204, 2, 'album:1888147421', '2026-11-04T12:02:00Z'),
    ('station', 'DR P1', '', '',
     'x-sonosapi-hls:urn%3adr%3aradio%3achannel%3a5fa156d1da351264f87b462d?sid=290&flags=8232&sn=3',
     '<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"><item id="10092028urn%3adr%3aradio%3achannel%3a5fa156d1da351264f87b462d" parentID="10fe2064live%3amain" restricted="true"><dc:title>DR P1</dc:title><upnp:class>object.item.audioItem.audioBroadcast.#channel</upnp:class><desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">SA_RINCON74247_</desc></item></DIDL-Lite>',
     290, 3, 'urn:dr:radio:channel:5fa156d1da351264f87b462d', '2026-11-04T12:04:00Z');
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

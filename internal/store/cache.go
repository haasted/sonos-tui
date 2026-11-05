package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// cacheTables are every table holding data a full discovery rebuilds. Clear
// empties these and reaches nothing else, so the list is the whole blast
// radius of throwing the cache away.
var cacheTables = []string{
	"speaker_cache_session",
	"speaker_cache",
}

// timeFormat is how an instant is written to a TEXT column: fixed width and
// always UTC, so the text sorts as the time does. RFC3339Nano drops trailing
// zeros, and ".12Z" sorts after ".120001Z".
const timeFormat = "2006-01-02T15:04:05.000000000Z07:00"

// CachedSpeaker is one speaker as the cache last saw it. Plain strings, since
// this layer holds no Sonos type of its own.
type CachedSpeaker struct {
	USN         string
	IP          string
	Location    string
	UDN         string
	RoomName    string
	DisplayName string
	ModelName   string
	ModelNumber string
}

// Snapshot is a whole cache: the speakers, and the context that judges
// whether they still apply.
type Snapshot struct {
	SavedAt  time.Time
	Subnet   string // the network this was recorded on
	Speakers []CachedSpeaker
}

// Cache is the only handle on the cached speakers. It carries no method that
// takes SQL, so a caller holding one cannot reach a playlist or a remembered
// track: widening the damage means editing this file, not calling it wrongly.
type Cache struct {
	db *sql.DB
}

// Load reads the cache. ok is false when nothing has been written yet, which
// is a first run rather than a fault.
func (c *Cache) Load(ctx context.Context) (snapshot Snapshot, ok bool, err error) {
	var savedAt string
	row := c.db.QueryRowContext(ctx,
		`SELECT saved_at, subnet FROM speaker_cache_session WHERE id = 1`)
	switch err := row.Scan(&savedAt, &snapshot.Subnet); {
	case errors.Is(err, sql.ErrNoRows):
		return Snapshot{}, false, nil
	case err != nil:
		return Snapshot{}, false, fmt.Errorf("reading the cached session: %w", err)
	}

	snapshot.SavedAt, err = parseTime(savedAt)
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("reading when the cache was written: %w", err)
	}

	if snapshot.Speakers, err = c.speakers(ctx); err != nil {
		return Snapshot{}, false, err
	}
	return snapshot, true, nil
}

// Save replaces the cache with this snapshot. A replace rather than an
// update: the speakers on screen at exit are the whole truth, and one
// transaction means a reader never sees half of them.
func (c *Cache) Save(ctx context.Context, snapshot Snapshot) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range cacheTables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("replacing %s: %w", table, err)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO speaker_cache_session VALUES (1, ?, ?)`,
		formatTime(snapshot.SavedAt), snapshot.Subnet); err != nil {
		return fmt.Errorf("writing the cached session: %w", err)
	}

	for _, s := range snapshot.Speakers {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO speaker_cache VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			s.USN, s.IP, s.Location, s.UDN,
			s.RoomName, s.DisplayName, s.ModelName, s.ModelNumber); err != nil {
			return fmt.Errorf("writing cached speaker %s: %w", s.USN, err)
		}
	}
	return tx.Commit()
}

// Clear throws the cache away. Discovery rebuilds it, so this is a cheap
// thing to do and must stay a safe one.
func (c *Cache) Clear(ctx context.Context) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range cacheTables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("clearing %s: %w", table, err)
		}
	}
	return tx.Commit()
}

func (c *Cache) speakers(ctx context.Context) ([]CachedSpeaker, error) {
	rows, err := c.db.QueryContext(ctx,
		`SELECT usn, ip, location, udn, room_name, display_name, model_name, model_number
		 FROM speaker_cache ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("reading cached speakers: %w", err)
	}
	defer rows.Close()

	var speakers []CachedSpeaker
	for rows.Next() {
		var s CachedSpeaker
		if err := rows.Scan(&s.USN, &s.IP, &s.Location, &s.UDN,
			&s.RoomName, &s.DisplayName, &s.ModelName, &s.ModelNumber); err != nil {
			return nil, fmt.Errorf("reading a cached speaker: %w", err)
		}
		speakers = append(speakers, s)
	}
	return speakers, rows.Err()
}

// formatTime writes an instant as timeFormat says.
func formatTime(t time.Time) string { return t.UTC().Format(timeFormat) }

// parseTime reads an instant written by formatTime. RFC3339 rather than
// timeFormat: it takes a fraction of any length, so rows written by hand parse.
func parseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339, s) }

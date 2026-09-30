package store

import (
	"context"
	"database/sql"
	"fmt"
)

// cacheTables are every table holding data a full discovery rebuilds. Clear
// empties these and reaches nothing else, so the list is the whole blast
// radius of throwing the cache away.
var cacheTables = []string{
	"speaker_cache_session",
	"speaker_cache",
}

// Cache is the only handle on the cached speakers. It carries no method that
// takes SQL, so a caller holding one cannot reach a playlist or a remembered
// track: widening the damage means editing this file, not calling it wrongly.
type Cache struct {
	db *sql.DB
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

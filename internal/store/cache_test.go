package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: throwing the cache away, which must actually empty it. Discovery
// rebuilds these rows, so they are the one thing here that is safe to lose.
func TestCache_ClearEmptiesEveryCacheTable(t *testing.T) {
	s := openTemp(t)
	fillCache(t, s)

	require.NoError(t, s.Cache().Clear(t.Context()))

	for _, table := range cacheTables {
		assert.Equal(t, 0, countIn(t, s, table), "%s still holds rows", table)
	}
}

// Case: the guarantee the shared file has to earn. ADR-0003 put the cache in
// the same file as the playlists, so clearing it must reach the cache tables
// and stop there.
func TestCache_ClearLeavesEveryOtherTableAlone(t *testing.T) {
	s := openTemp(t)
	fillCache(t, s)

	// Stands in for the playlist and history tables this file will hold.
	_, err := s.db.ExecContext(t.Context(),
		`CREATE TABLE canary (a TEXT) STRICT; INSERT INTO canary VALUES ('keep me');`)
	require.NoError(t, err)

	require.NoError(t, s.Cache().Clear(t.Context()))

	assert.Equal(t, 1, countIn(t, s, "canary"), "clearing the cache reached a table that is not the cache")
}

// Case: an empty cache. Clearing one is what a first run does if the user
// asks, and it must be uneventful rather than an error.
func TestCache_ClearingAnEmptyCacheIsFine(t *testing.T) {
	s := openTemp(t)

	assert.NoError(t, s.Cache().Clear(t.Context()))
}

func openTemp(t *testing.T) *Store {
	t.Helper()

	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "sonos.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func fillCache(t *testing.T, s *Store) {
	t.Helper()

	_, err := s.db.ExecContext(t.Context(), `
		INSERT INTO speaker_cache VALUES
			('uuid:A','192.168.1.10','http://a','uuid:A','Kitchen','Play:1','S1','S1'),
			('uuid:B','192.168.1.11','http://b','uuid:B','Bedroom','Era 100','S38','S38');
		INSERT INTO speaker_cache_session VALUES (1,'2026-09-29T10:00:00Z','192.168.1','uuid:B');
	`)
	require.NoError(t, err)
}

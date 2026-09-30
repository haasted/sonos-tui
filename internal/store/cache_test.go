package store

import (
	"path/filepath"
	"testing"
	"time"

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

// Case: a whole snapshot through the store and back, every column included.
// A column dropped in the INSERT or the SELECT reads as an empty string, not
// as an error, so each one has to be looked at.
func TestCache_ASnapshotSurvivesTheRoundTrip(t *testing.T) {
	require := require.New(t)
	s := openTemp(t)
	saved := Snapshot{
		SavedAt:  time.Date(2026, 9, 30, 10, 30, 0, 0, time.UTC),
		Subnet:   "192.168.1",
		Selected: "uuid:B",
		Speakers: []CachedSpeaker{{
			USN: "uuid:A", IP: "192.168.1.10", Location: "http://a/d.xml", UDN: "uuid:A",
			RoomName: "Kitchen", DisplayName: "One", ModelName: "Sonos One", ModelNumber: "S13",
		}},
	}

	require.NoError(s.Cache().Save(t.Context(), saved))

	loaded, ok, err := s.Cache().Load(t.Context())
	require.NoError(err)
	require.True(ok)
	require.True(saved.SavedAt.Equal(loaded.SavedAt), "saved at %v, read back %v", saved.SavedAt, loaded.SavedAt)
	loaded.SavedAt = saved.SavedAt // compared above; the rest must match exactly
	require.Equal(saved, loaded)
}

// Case: reading before anything has been written. A first run, not a fault.
func TestCache_AnEmptyCacheReadsAsAbsent(t *testing.T) {
	s := openTemp(t)

	_, ok, err := s.Cache().Load(t.Context())

	require.NoError(t, err, "an empty cache is a normal state, not an error")
	assert.False(t, ok)
}

// Case: the cache after it has been cleared, which must read as absent
// rather than as a snapshot holding nobody.
func TestCache_AClearedCacheReadsAsAbsent(t *testing.T) {
	s := openTemp(t)
	fillCache(t, s)
	require.NoError(t, s.Cache().Clear(t.Context()))

	_, ok, err := s.Cache().Load(t.Context())

	require.NoError(t, err)
	assert.False(t, ok)
}

// Case: a save that fails part way. The session row is written before the
// speakers, so a bad speaker must take the whole snapshot back out rather
// than leave a session pointing at nothing.
func TestCache_AFailedSaveLeavesTheOldCacheStanding(t *testing.T) {
	s := openTemp(t)
	good := Snapshot{SavedAt: time.Now(), Subnet: "192.168.1", Selected: "uuid:A",
		Speakers: []CachedSpeaker{{USN: "uuid:A"}}}
	require.NoError(t, s.Cache().Save(t.Context(), good))

	// Two speakers sharing a USN: the primary key refuses the second.
	clash := Snapshot{SavedAt: time.Now(), Subnet: "10.0.0", Selected: "uuid:Z",
		Speakers: []CachedSpeaker{{USN: "uuid:Z"}, {USN: "uuid:Z"}}}
	require.Error(t, s.Cache().Save(t.Context(), clash))

	loaded, ok, err := s.Cache().Load(t.Context())
	require.NoError(t, err)
	require.True(t, ok, "a failed save wiped the cache it could not replace")
	assert.Equal(t, "uuid:A", loaded.Selected, "a rolled-back save left its session row behind")
}

// Case: the session row is a singleton, enforced by the schema rather than
// by callers remembering. A second row must be refused, not stored -- the
// read leans on there being only one, so nothing else keeps that true.
func TestCache_TheSessionRowCannotBeDuplicated(t *testing.T) {
	s := openTemp(t)
	fillCache(t, s)

	_, err := s.db.ExecContext(t.Context(),
		`INSERT INTO speaker_cache_session VALUES (2, '2026-01-01T00:00:00Z', '10.0.0', 'uuid:Z')`)

	require.Error(t, err, "the schema let a second session row in")
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

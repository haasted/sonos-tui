package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKey stands in for the keys features will add.
const testKey SettingKey = "test.key"

// Case: a setting written and read back.
func TestSettings_ASettingSurvivesTheRoundTrip(t *testing.T) {
	s := openTemp(t)

	require.NoError(t, s.Settings().Set(t.Context(), testKey, "albums"))
	value, err := s.Settings().Get(t.Context(), testKey, "fallback")

	require.NoError(t, err)
	assert.Equal(t, "albums", value)
}

// Case: a setting never written, which is a first run rather than a fault.
// The caller's fallback comes back.
func TestSettings_AnUnsetKeyGivesTheFallback(t *testing.T) {
	value, err := openTemp(t).Settings().Get(t.Context(), testKey, "fallback")

	require.NoError(t, err)
	assert.Equal(t, "fallback", value)
}

// Case: a read that fails. The fallback still comes back, with the error, so
// the caller has a value to carry on with.
func TestSettings_AFailedReadGivesTheFallbackAndTheError(t *testing.T) {
	s := openTemp(t)
	require.NoError(t, s.Close())

	value, err := s.Settings().Get(t.Context(), testKey, "fallback")

	assert.Error(t, err)
	assert.Equal(t, "fallback", value)
}

// Case: a setting written twice. The second value replaces the first, and
// there is still one row for the key.
func TestSettings_SetReplacesTheValue(t *testing.T) {
	s := openTemp(t)

	require.NoError(t, s.Settings().Set(t.Context(), testKey, "albums"))
	require.NoError(t, s.Settings().Set(t.Context(), testKey, "stations"))

	value, err := s.Settings().Get(t.Context(), testKey, "fallback")
	require.NoError(t, err)
	assert.Equal(t, "stations", value)
	assert.Equal(t, 1, countIn(t, s, "setting"))
}

// Case: a setting deleted. A later read gives the fallback, and deleting a
// setting that is not there is no error.
func TestSettings_DeleteRemovesTheValue(t *testing.T) {
	s := openTemp(t)
	require.NoError(t, s.Settings().Set(t.Context(), testKey, "albums"))

	require.NoError(t, s.Settings().Delete(t.Context(), testKey))
	require.NoError(t, s.Settings().Delete(t.Context(), testKey))

	value, err := s.Settings().Get(t.Context(), testKey, "fallback")
	require.NoError(t, err)
	assert.Equal(t, "fallback", value)
	assert.Zero(t, countIn(t, s, "setting"))
}

// Case: a secret deleted while the database is open. No copy of it stays in
// the file's free space or in the write-ahead log.
func TestSettings_DeleteLeavesNoCopyOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sonos.db")
	s, err := Open(t.Context(), path)
	require.NoError(t, err)
	defer s.Close()
	secret := []byte("made-up-secret-7f3a9c")

	require.NoError(t, s.Settings().Set(t.Context(), SettingCloudSession, string(secret)))
	require.NoError(t, s.Settings().Delete(t.Context(), SettingCloudSession))

	for _, name := range []string{path, path + "-wal"} {
		data, err := os.ReadFile(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		require.NoError(t, err)
		assert.False(t, bytes.Contains(data, secret), "%s still holds the deleted value", name)
	}
}

// Case: the cache cleared with settings stored. Settings are not cache and
// must survive.
func TestSettings_ClearingTheCacheKeepsThem(t *testing.T) {
	s := openTemp(t)
	fillCache(t, s)
	require.NoError(t, s.Settings().Set(t.Context(), testKey, "albums"))

	require.NoError(t, s.Cache().Clear(t.Context()))

	assert.Equal(t, 1, countIn(t, s, "setting"))
}

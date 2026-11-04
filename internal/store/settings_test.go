package store

import (
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

// Case: the cache cleared with settings stored. Settings are not cache and
// must survive.
func TestSettings_ClearingTheCacheKeepsThem(t *testing.T) {
	s := openTemp(t)
	fillCache(t, s)
	require.NoError(t, s.Settings().Set(t.Context(), testKey, "albums"))

	require.NoError(t, s.Cache().Clear(t.Context()))

	assert.Equal(t, 1, countIn(t, s, "setting"))
}

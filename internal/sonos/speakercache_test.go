package sonos_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonos"
	"github.com/haasted/sonos-tui/internal/store"
)

// Case: a normal save then load, which must survive the round trip with the
// descriptions intact so tabs can show room names without a fetch.
func TestCacheRoundTrip(t *testing.T) {
	var (
		require = require.New(t)
		assert  = assert.New(t)
	)

	cache := tempCache(t)
	saved := []sonos.Speaker{
		sonos.NewSpeaker("192.168.1.10", "http://192.168.1.10:1400/xml/d.xml", "uuid:A",
			sonos.DeviceDescription{RoomName: "Kitchen", UDN: "uuid:A"}),
		sonos.NewSpeaker("192.168.1.11", "http://192.168.1.11:1400/xml/d.xml", "uuid:B",
			sonos.DeviceDescription{RoomName: "Bedroom", UDN: "uuid:B"}),
	}
	require.NoError(sonos.SaveCache(t.Context(), cache, saved))

	loaded, ok := sonos.LoadCache(t.Context(), cache)
	require.True(ok)

	require.Len(loaded.Speakers, 2)

	desc, ok := loaded.Speakers[0].Description()
	require.True(ok, "a rebuilt speaker should already carry its description")
	assert.Equal("Kitchen", desc.RoomName)
	assert.Equal("Kitchen", loaded.Speakers[0].RoomName())
	assert.Equal("192.168.1.10", loaded.Speakers[0].IP)
}

// Case: the order the speakers were saved in, which is the tab order the
// next run opens with.
func TestCacheKeepsTheOrderItWasGiven(t *testing.T) {
	cache := tempCache(t)
	rooms := []string{"Kitchen", "Bedroom", "Study", "Hall"}

	var saved []sonos.Speaker
	for i, room := range rooms {
		usn := "uuid:" + room
		saved = append(saved, sonos.NewSpeaker(
			"192.168.1.1"+string(rune('0'+i)), "http://x/d.xml", usn,
			sonos.DeviceDescription{RoomName: room, UDN: usn}))
	}
	require.NoError(t, sonos.SaveCache(t.Context(), cache, saved))

	loaded, ok := sonos.LoadCache(t.Context(), cache)
	require.True(t, ok)
	require.Len(t, loaded.Speakers, len(rooms))

	for at, room := range rooms {
		assert.Equal(t, room, loaded.Speakers[at].RoomName(), "speaker %d is out of order", at)
	}
}

// Case: a store nothing has been written to, the first-ever run.
func TestLoadCacheReportsMissingRatherThanFailing(t *testing.T) {
	_, ok := sonos.LoadCache(t.Context(), tempCache(t))

	assert.False(t, ok, "an empty cache is a normal state, not an error")
}

// Case: saving twice. The second save is the whole truth, so a speaker that
// has gone must not linger from the first.
func TestSaveCacheReplacesWhatWasThere(t *testing.T) {
	cache := tempCache(t)
	first := []sonos.Speaker{
		sonos.NewSpeaker("192.168.1.10", "http://x/d.xml", "uuid:A", sonos.DeviceDescription{UDN: "uuid:A"}),
		sonos.NewSpeaker("192.168.1.11", "http://y/d.xml", "uuid:B", sonos.DeviceDescription{UDN: "uuid:B"}),
	}
	require.NoError(t, sonos.SaveCache(t.Context(), cache, first))

	require.NoError(t, sonos.SaveCache(t.Context(), cache, first[:1]))

	loaded, ok := sonos.LoadCache(t.Context(), cache)
	require.True(t, ok)
	assert.Len(t, loaded.Speakers, 1, "a speaker gone at exit must not come back next run")
}

// Case: a cache written on another network, whose IPs mean nothing here.
func TestCacheIsUnusableOnADifferentSubnet(t *testing.T) {
	cache := tempCache(t)
	require.NoError(t, sonos.SaveCache(t.Context(), cache, []sonos.Speaker{
		sonos.NewSpeaker("192.168.1.10", "http://x/d.xml", "uuid:A", sonos.DeviceDescription{}),
	}))

	loaded, ok := sonos.LoadCache(t.Context(), cache)
	require.True(t, ok)

	assert.False(t, loaded.UsableOn("10.0.0.0/24"), "a different subnet must reject the cache")
	assert.True(t, loaded.UsableOn(loaded.Subnet), "the recording subnet must accept it")
	assert.False(t, loaded.UsableOn(""), "an unknown subnet must not accidentally match")
}

// tempCache is a cache handle on a throwaway database, so tests never read
// or clobber the developer's real one.
func tempCache(t *testing.T) *store.Cache {
	t.Helper()

	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "sonos.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	return db.Cache()
}

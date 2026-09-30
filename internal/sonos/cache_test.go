package sonos_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a normal save then load, which must survive the round trip with the
// descriptions intact so tabs can show room names without a fetch.
func TestCacheRoundTrip(t *testing.T) {
	var (
		require = require.New(t)
		assert  = assert.New(t)
	)

	useTempCacheDir(t)

	saved := []sonos.Speaker{
		sonos.NewSpeaker("192.168.1.10", "http://192.168.1.10:1400/xml/d.xml", "uuid:A",
			sonos.DeviceDescription{RoomName: "Kitchen", UDN: "uuid:A"}),
		sonos.NewSpeaker("192.168.1.11", "http://192.168.1.11:1400/xml/d.xml", "uuid:B",
			sonos.DeviceDescription{RoomName: "Bedroom", UDN: "uuid:B"}),
	}
	require.NoError(sonos.SaveCache(saved, "uuid:B"))

	cache, ok := sonos.LoadCache()
	require.True(ok)

	assert.Equal("uuid:B", cache.Selected, "the selected tab must survive, to be re-selected next run")
	require.Len(cache.Speakers, 2)

	rebuilt := cache.SpeakerList()
	require.Len(rebuilt, 2)
	desc, ok := rebuilt[0].Description()
	require.True(ok, "a rebuilt speaker should already carry its description")
	assert.Equal("Kitchen", desc.RoomName)
	assert.Equal("Kitchen", rebuilt[0].RoomName())
}

// Case: no cache file at all, the first-ever run.
func TestLoadCacheReportsMissingRatherThanFailing(t *testing.T) {
	useTempCacheDir(t)

	_, ok := sonos.LoadCache()
	assert.False(t, ok, "a missing cache is a normal state, not an error")
}

// Case: a cache written on another network, whose IPs mean nothing here.
func TestCacheIsUnusableOnADifferentSubnet(t *testing.T) {
	useTempCacheDir(t)
	require.NoError(t, sonos.SaveCache([]sonos.Speaker{
		sonos.NewSpeaker("192.168.1.10", "http://x/d.xml", "uuid:A", sonos.DeviceDescription{}),
	}, ""))

	cache, ok := sonos.LoadCache()
	require.True(t, ok)

	assert.False(t, cache.UsableOn("10.0.0.0/24"), "a different subnet must reject the cache")
	assert.True(t, cache.UsableOn(cache.Subnet), "the recording subnet must accept it")
	assert.False(t, cache.UsableOn(""), "an unknown subnet must not accidentally match")
}

// Case: a corrupt file, which must be ignored rather than crashing startup.
func TestLoadCacheIgnoresCorruptContent(t *testing.T) {
	dir := useTempCacheDir(t)
	path, err := sonos.CachePath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o644))
	_ = dir

	_, ok := sonos.LoadCache()
	assert.False(t, ok)
}

// useTempCacheDir points os.UserCacheDir at a throwaway directory so tests
// never read or clobber the developer's real cache.
func useTempCacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir)
	return dir
}

// Case: a cache written by a different layout version. The guard exists so a
// format change is survivable, and this is the only thing that exercises it —
// ported from cmd/listening before that command was deleted.
func TestLoadCacheRejectsAnotherVersion(t *testing.T) {
	useTempCacheDir(t)
	require.NoError(t, sonos.SaveCache([]sonos.Speaker{
		sonos.NewSpeaker("192.168.1.10", "http://x/d.xml", "uuid:A", sonos.DeviceDescription{}),
	}, ""))

	path, err := sonos.CachePath()
	require.NoError(t, err)

	var stored map[string]any
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &stored))

	stored["Version"] = stored["Version"].(float64) + 1
	bumped, err := json.Marshal(stored)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, bumped, 0o644))

	_, ok := sonos.LoadCache()
	assert.False(t, ok, "a cache from another version must be ignored, not half-read")
}

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// Case: a track found by search. Its URI is the one Soveværelse wrote when the
// Sonos app queued the same track: sid from the service type, sn from the
// account the content API gave.
func TestResultPlayable_ATrackIsTheSpeakersOwn(t *testing.T) {
	playable, err := resultPlayable(sonoscloud.Result{
		ID:      appleMusicID("song:1774570881"),
		Type:    sonoscloud.TypeTrack,
		Name:    "/=/",
		Artists: []sonoscloud.Ref{{Name: "Andrew Bird"}},
		Album:   &sonoscloud.Ref{Name: "The Mysterious Production of Eggs"},
	})
	require.NoError(t, err)

	assert.Equal(t, "x-sonos-http:song%3a1774570881.mp4?sid=204&flags=8232&sn=2", playable.URI)
	assert.Contains(t, playable.Metadata, `<item id="10032020song%3a1774570881"`)
	assert.Contains(t, playable.Metadata, "<dc:creator>Andrew Bird</dc:creator>")
	assert.Contains(t, playable.Metadata, "<upnp:album>The Mysterious Production of Eggs</upnp:album>")
}

// Case: an album found by search. It plays as a service container, as an
// album favourite does.
func TestResultPlayable_AnAlbumIsAContainer(t *testing.T) {
	playable, err := resultPlayable(sonoscloud.Result{
		ID:   appleMusicID("album:1774570638"),
		Type: sonoscloud.TypeAlbum,
		Name: "The Mysterious Production of Eggs",
	})
	require.NoError(t, err)

	assert.Equal(t, "x-rincon-cpcontainer:00040000album%3a1774570638?sid=204&flags=0&sn=2", playable.URI)
	assert.Contains(t, playable.Metadata, "object.container.album.musicAlbum")
	assert.Equal(t, "The Mysterious Production of Eggs", playable.Title)
}

// Case: results that cannot be played: an artist, a station, a service type
// not of the sid × 256 + 7 form, and no account.
func TestResultPlayable_RefusesWhatItCannotPlay(t *testing.T) {
	for name, r := range map[string]sonoscloud.Result{
		"artist":     {ID: appleMusicID("artist:2046"), Type: sonoscloud.TypeArtist},
		"station":    {ID: appleMusicID("radio:ra.2046"), Type: sonoscloud.TypeProgram},
		"service":    {ID: sonoscloud.ResultID{ObjectID: "song:1", ServiceID: "204", AccountID: "2"}, Type: sonoscloud.TypeTrack},
		"no account": {ID: sonoscloud.ResultID{ObjectID: "song:1", ServiceID: "52231"}, Type: sonoscloud.TypeTrack},
	} {
		_, err := resultPlayable(r)
		assert.Error(t, err, name)
	}
}

// appleMusicID is an Apple Music id as the content API gives it, on the
// household's account 2.
func appleMusicID(objectID string) sonoscloud.ResultID {
	return sonoscloud.ResultID{ObjectID: objectID, ServiceID: "52231", AccountID: "2"}
}

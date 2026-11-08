package sonos_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: a queue loaded from a playlist, captured from a real speaker. The
// queue is the speaker's own, but it says what was loaded into it.
func TestQueueSource_APlaylistNamesTheQueue(t *testing.T) {
	event, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_queue_playlist.xml"))
	require.NoError(t, err)

	assert.True(t, event.HasQueueSource)
	assert.Equal(t, "Awesome singles", event.QueueSource.Title)
}

// Case: a queue loaded from an album. A container too, so it names the queue.
func TestQueueSource_AnAlbumNamesTheQueue(t *testing.T) {
	event, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_queue_album.xml"))
	require.NoError(t, err)

	assert.True(t, event.HasQueueSource)
	assert.Equal(t, "Ordensduks i Ingenmandsland", event.QueueSource.Title)
}

// Case: the source as something to play again. The event carries the
// container's URI and its DIDL with the service's desc, which is all a
// favourite needs, and the class says whether it is a playlist or an album.
func TestQueueSource_KeepsWhatPlayingItAgainNeeds(t *testing.T) {
	playlist, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_queue_playlist.xml"))
	require.NoError(t, err)
	album, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_queue_album.xml"))
	require.NoError(t, err)

	assert.Equal(t, "x-rincon-cpcontainer:00060000libraryplaylist%3ap.V7VYpk7FYWLeW?sid=204&flags=0&sn=2", playlist.QueueSource.URI)
	assert.Contains(t, playlist.QueueSource.Metadata, "SA_RINCON52231_X_#Svc52231-0-Token")
	kind, ok := playlist.QueueSource.Kind()
	assert.True(t, ok)
	assert.Equal(t, sonos.ContainerPlaylist, kind)

	kind, ok = album.QueueSource.Kind()
	assert.True(t, ok)
	assert.Equal(t, sonos.ContainerAlbum, kind)
	assert.Contains(t, album.QueueSource.URI, "00040000album%3a")
}

// Case: a station as something to play again. Its source keeps the DIDL the
// speaker sent, since a station cannot be rebuilt from its id; a queue is a
// queue, and a station is not.
func TestSourceInfo_KeepsAStationsDIDL(t *testing.T) {
	station, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_broadcast_radio.xml"))
	require.NoError(t, err)
	queue, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_queue_playlist.xml"))
	require.NoError(t, err)

	assert.Contains(t, station.Source.Metadata, "<dc:title>DR P1</dc:title>")
	assert.False(t, station.Source.IsQueue())
	assert.True(t, queue.Source.IsQueue())
}

// Case: a container that is neither a playlist nor an album has no kind.
func TestQueueSource_AnotherContainerHasNoKind(t *testing.T) {
	_, ok := sonos.QueueSource{Class: "object.container.playlistContainer.sameArtist"}.Kind()
	assert.False(t, ok)
}

// Case: live radio. The speaker reports the station as what was last
// started, and a station is not what the queue holds.
func TestQueueSource_AStationDoesNotNameTheQueue(t *testing.T) {
	event, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_broadcast_radio.xml"))
	require.NoError(t, err)

	assert.False(t, event.HasQueueSource, "a station was taken for the queue's name: %q", event.QueueSource.Title)
}

// Case: a station started after a playlist was loaded. The queue still holds
// the playlist's tracks, so it keeps the playlist's name.
func TestQueueSource_SurvivesAStationStartedAfterIt(t *testing.T) {
	device := replayDevice(t)

	applyEvent(t, device, fixture(t, "avtransport_queue_playlist.xml"))
	applyEvent(t, device, fixture(t, "avtransport_broadcast_radio.xml"))

	assert.Equal(t, "Awesome singles", device.State().QueueSource.Title)
}

// Case: a second playlist loaded over the first. The newer name wins.
func TestQueueSource_AnotherPlaylistReplacesIt(t *testing.T) {
	device := replayDevice(t)

	applyEvent(t, device, fixture(t, "avtransport_queue_playlist.xml"))
	applyEvent(t, device, fixture(t, "avtransport_queue_album.xml"))

	assert.Equal(t, "Ordensduks i Ingenmandsland", device.State().QueueSource.Title)
}

// Case: a real album, captured from a speaker started by the Sonos app. The
// app names no artist for it, so one must come from somewhere else.
func TestQueueSource_TheSonosAppNamesNoArtistForAnAlbum(t *testing.T) {
	event, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_queue_album.xml"))
	require.NoError(t, err)

	assert.Equal(t, "Ordensduks i Ingenmandsland", event.QueueSource.Title)
	assert.Empty(t, event.QueueSource.Creator)
}

// Case: the artist an album's tracks share. All alike or more than half
// alike picks that artist; no such artist is Various Artists.
func TestAlbumArtist_IsTheArtistMostTracksShare(t *testing.T) {
	cases := map[string]struct {
		artists []string
		want    string
	}{
		"all the same":   {[]string{"Mumford & Sons", "Mumford & Sons", "Mumford & Sons"}, "Mumford & Sons"},
		"more than half": {[]string{"Mumford & Sons", "Mumford & Sons", "Guest"}, "Mumford & Sons"},
		"exactly half":   {[]string{"A", "A", "B", "C"}, sonos.VariousArtists},
		"all different":  {[]string{"A", "B", "C"}, sonos.VariousArtists},
		"blanks ignored": {[]string{"A", "", ""}, "A"},
		"no artist":      {[]string{"", ""}, ""},
		"no tracks":      {nil, ""},
	}
	for name, c := range cases {
		assert.Equal(t, c.want, sonos.AlbumArtist("Sigh No More", albumTracks("Sigh No More", c.artists...)), name)
	}
}

// Case: tracks added to the queue after the album. Only the album's own
// tracks count; with none of them in the queue, every track counts.
func TestAlbumArtist_CountsOnlyTheAlbumsTracks(t *testing.T) {
	tracks := append(albumTracks("Sigh No More", "Mumford & Sons"), albumTracks("Other", "X", "X", "X")...)
	assert.Equal(t, "Mumford & Sons", sonos.AlbumArtist("Sigh No More", tracks))

	assert.Equal(t, "X", sonos.AlbumArtist("Not queued", tracks))
}

// replayDevice handles real captures, which carry no position: the handler
// asks the speaker for one, and this speaker refuses, as an unreachable one
// would. The handler logs that and carries on.
func replayDevice(t *testing.T) *sonos.Device {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no position here", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	return sonos.NewDeviceWithClient(sonos.Speaker{IP: "192.168.1.10"},
		&sonos.SonosClient{BaseURL: server.URL, HTTPClient: server.Client()})
}

// albumTracks is one track from album for each artist given.
func albumTracks(album string, artists ...string) []sonos.TrackMetadata {
	tracks := make([]sonos.TrackMetadata, len(artists))
	for i, artist := range artists {
		tracks[i] = sonos.TrackMetadata{Title: "Track", Artist: artist, Album: album}
	}
	return tracks
}

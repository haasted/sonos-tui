package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/store"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: a on the player while a station plays, from a real capture. The
// station is marked at once, with no chooser, as the speaker sent it.
func TestMarkPlayer_AStationIsMarkedAtOnce(t *testing.T) {
	m, db := playerPlaying(t, "avtransport_station_with_track.xml")

	model, cmd := m.Update(keyMsgFor("a"))

	assert.False(t, model.(appModel).marking.IsOpen(), "a station opened the chooser")
	require.NotNil(t, cmd)
	assert.Equal(t, favouriteMarked, cmd().(panes.NoticeMsg).Text)
	favourites, err := db.Favourites().List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, store.FavouriteStation, favourites[0].Kind)
	assert.Equal(t, "Akylas", favourites[0].Title)
	assert.Equal(t, "radio:ra.1594201819", favourites[0].ItemID)
}

// Case: a on a live broadcast that is a favourite already. It is found by
// its service and id, though the speaker's DIDL for it differs, and moved up.
func TestMarkPlayer_AKnownBroadcastIsAlreadyAFavourite(t *testing.T) {
	m, _ := playerPlaying(t, "avtransport_broadcast_radio.xml")

	_, cmd := m.Update(keyMsgFor("a"))

	require.NotNil(t, cmd)
	assert.Equal(t, favouriteMarkedNot, cmd().(panes.NoticeMsg).Text)
}

// Case: a on the player while the queue plays a playlist. The queue's
// chooser opens, with the playing track and the playlist.
func TestMarkPlayer_AQueueTrackOpensTheChooser(t *testing.T) {
	m, _ := playerPlaying(t, "avtransport_queue_playlist.xml")

	model, _ := m.Update(keyMsgFor("a"))
	m = model.(appModel)

	require.True(t, m.marking.IsOpen(), "the chooser did not open")
	assert.Equal(t, []string{"Track: Don't Dream It's Over", "Playlist: Awesome singles"},
		rowLabels(m.marking.Content.(markContent).rows))
}

// Case: a on line-in, which no favourite can hold. A notice says so.
func TestMarkPlayer_LineInCannotBeMarked(t *testing.T) {
	m, device, _ := withQueueServer(t, 1)
	playSource(t, device, "x-rincon-stream:RINCON_TEST01400")

	_, cmd := m.Update(keyMsgFor("a"))

	require.NotNil(t, cmd)
	assert.Equal(t, favouriteNothingToMark, cmd().(panes.NoticeMsg).Text)
}

// playerPlaying is the player on a speaker that has sent the captured
// AVTransport event in testdata, with a database of its own.
func playerPlaying(t *testing.T, capture string) (appModel, *store.Store) {
	t.Helper()
	m, device, _ := withQueueServer(t, 40)
	body, err := os.ReadFile("../../internal/sonos/testdata/" + capture)
	require.NoError(t, err)
	require.NoError(t, device.HandleAVTransportEvent(context.Background(), body))
	<-device.Events()

	m.db = openTestStore(t)
	return resize(m), m.db
}

// playSource has a speaker report that it plays uri.
func playSource(t *testing.T, device *sonos.Device, uri string) {
	t.Helper()
	body := `<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0"><e:property><LastChange>` +
		`&lt;Event xmlns="urn:schemas-upnp-org:metadata-1-0/AVT/"&gt;&lt;InstanceID val="0"&gt;` +
		`&lt;TransportState val="PLAYING"/&gt;&lt;AVTransportURI val="` + uri + `"/&gt;` +
		`&lt;/InstanceID&gt;&lt;/Event&gt;</LastChange></e:property></e:propertyset>`
	require.NoError(t, device.HandleAVTransportEvent(context.Background(), []byte(body)))
	<-device.Events()
}

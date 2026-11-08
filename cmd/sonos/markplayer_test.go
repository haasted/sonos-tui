package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

// Case: a pressed twice on a live broadcast, from a real capture. The second
// press finds it by its service and id, and says it is a favourite already.
func TestMarkPlayer_ABroadcastMarkedTwiceIsAlreadyAFavourite(t *testing.T) {
	m, _ := playerPlaying(t, "avtransport_broadcast_radio.xml")

	_, cmd := m.Update(keyMsgFor("a"))
	require.NotNil(t, cmd)
	assert.Equal(t, favouriteMarked, cmd().(panes.NoticeMsg).Text)

	_, cmd = m.Update(keyMsgFor("a"))
	require.NotNil(t, cmd)
	assert.Equal(t, favouriteMarkedNot, cmd().(panes.NoticeMsg).Text)
}

// Case: a on the player while the queue plays a playlist. The queue's
// chooser opens, with the playing track and the playlist.
func TestMarkPlayer_AQueueTrackOpensTheChooser(t *testing.T) {
	m, _ := playerPlaying(t, "avtransport_queue_playlist.xml")

	model, cmd := m.Update(keyMsgFor("a"))
	require.NotNil(t, cmd)
	m = deliver(model.(appModel), cmd())

	require.True(t, m.marking.IsOpen(), "the chooser did not open")
	assert.Equal(t, []string{"Track: Don't Dream It's Over", "Playlist: Awesome singles"},
		rowLabels(m.marking.Content.(markContent).rows))
}

// Case: a on the player while the queue plays an album, from a real capture
// that names no artist for it. The queue is read, and the album gets the
// artist most of its tracks share.
func TestMarkPlayer_AnAlbumGetsTheArtistOfItsTracks(t *testing.T) {
	device := albumQueueDevice(t, "Ordensduks i Ingenmandsland", "Lars H.U.G.", "Lars H.U.G.", "Guest")
	m := playerOn(t, device, "avtransport_queue_album.xml")

	model, cmd := m.Update(keyMsgFor("a"))
	require.NotNil(t, cmd)
	msg := cmd().(favouriteChooseMsg)
	m = choose(t, model.(appModel), msg)

	assert.Equal(t, "Lars H.U.G.", msg.source.Creator)
	assert.Contains(t, rowLabels(m.marking.Content.(markContent).rows), "Album: Ordensduks i Ingenmandsland")
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
	m = playerOn(t, device, capture)
	return m, m.db
}

// playerOn is the player on device, after it has sent the captured
// AVTransport event in testdata, with a database of its own.
func playerOn(t *testing.T, device *sonos.Device, capture string) appModel {
	t.Helper()
	m := sizedModel(t)
	m.tabs.Active = 0
	m.tabs.Items[0].Content = newSpeakerModel(context.Background(), device)
	body, err := os.ReadFile("../../internal/sonos/testdata/" + capture)
	require.NoError(t, err)
	require.NoError(t, device.HandleAVTransportEvent(context.Background(), body))
	<-device.Events()

	m.db = openTestStore(t)
	return resize(m)
}

// albumQueueDevice is a speaker whose queue holds one track from album for
// each artist given.
func albumQueueDevice(t *testing.T, album string, artists ...string) *sonos.Device {
	t.Helper()
	var didl strings.Builder
	didl.WriteString(`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">`)
	for i, artist := range artists {
		fmt.Fprintf(&didl, `<item id="Q:0/%d"><dc:title>Track %d</dc:title><dc:creator>%s</dc:creator>`+
			`<upnp:album>%s</upnp:album></item>`, i+1, i+1, artist, album)
	}
	didl.WriteString(`</DIDL-Lite>`)
	escaped := strings.NewReplacer("<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(didl.String())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
			`<s:Body><u:BrowseResponse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">`+
			`<Result>%s</Result><NumberReturned>%d</NumberReturned><TotalMatches>%d</TotalMatches>`+
			`<UpdateID>3</UpdateID></u:BrowseResponse></s:Body></s:Envelope>`, escaped, len(artists), len(artists))
	}))
	t.Cleanup(server.Close)
	return sonos.NewDeviceWithClient(sonos.Speaker{IP: "192.168.1.10"},
		&sonos.SonosClient{BaseURL: server.URL, HTTPClient: server.Client()})
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

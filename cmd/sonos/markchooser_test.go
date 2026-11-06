package main

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/store"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: a with nothing picked, in a queue started from a playlist. It asks
// which to mark, carrying the track under the cursor and the playlist.
func TestMarkChooser_AAsksWhenThePlaylistCanBeMarked(t *testing.T) {
	q := navQueue(20, 4)
	q.source = playlistSource()

	_, cmd := q.Update(keyMsgFor("a"))

	require.NotNil(t, cmd)
	assert.Equal(t, favouriteChooseMsg{track: q.items[4].TrackMetadata, source: playlistSource()}, cmd())
}

// Case: a with tracks picked. Picking tracks is choosing to work with
// tracks, so no chooser: the picked tracks are marked as before.
func TestMarkChooser_APickedSelectionNeedsNoChooser(t *testing.T) {
	q := moveQueue(navQueue(20, 4), "s")
	q.source = playlistSource()

	_, cmd := q.Update(keyMsgFor("a"))

	require.NotNil(t, cmd)
	_, marks := cmd().(favouriteMarkMsg)
	assert.True(t, marks, "a selection opened the chooser")
}

// Case: a source that cannot be marked: none, or one not from a music
// service. With only the track to offer there is no chooser.
func TestMarkChooser_NoMarkableSourceNeedsNoChooser(t *testing.T) {
	notService := playlistSource()
	notService.URI = "file:///jffs/settings/savedqueues.rsq#3"

	for name, source := range map[string]sonos.QueueSource{"none": {}, "not a service": notService} {
		q := navQueue(20, 4)
		q.source = source

		_, cmd := q.Update(keyMsgFor("a"))

		require.NotNil(t, cmd, name)
		_, marks := cmd().(favouriteMarkMsg)
		assert.True(t, marks, "%s: the chooser opened with nothing but the track to offer", name)
	}
}

// Case: the rows offered. The track first, then the playlist, each named;
// an album is called one and keeps its artist.
func TestMarkChooser_OffersTheTrackThenThePlaylistOrAlbum(t *testing.T) {
	rows := markRowsFor(serviceTrack(), playlistSource(), time.Now())
	assert.Equal(t, []string{"Track: Seek & Destroy", "Playlist: Kill 'Em All playlist"}, rowLabels(rows))
	assert.Equal(t, store.FavouritePlaylist, rows[1].fav.Kind)

	album := playlistSource()
	album.Class, album.Title, album.Creator = "object.container.album.musicAlbum", "Kill 'Em All", "Metallica"
	album.URI = "x-rincon-cpcontainer:00040000album%3a1440873900?sid=204&flags=0&sn=2"
	rows = markRowsFor(serviceTrack(), album, time.Now())
	assert.Equal(t, []string{"Track: Seek & Destroy", "Album: Kill 'Em All"}, rowLabels(rows))
	assert.Equal(t, store.FavouriteAlbum, rows[1].fav.Kind)
	assert.Equal(t, "Metallica", rows[1].fav.Artist)
}

// Case: the playlist's row as stored. Its service, account and id come from
// the URI, as for every favourite, and the artwork is left out of the DIDL.
func TestMarkChooser_ThePlaylistRowIsReadyToStore(t *testing.T) {
	fav, ok := sourceFavourite(playlistSource(), time.Now())

	require.True(t, ok)
	assert.Equal(t, sonos.ServiceRef{Service: 204, Account: 2, ItemID: "libraryplaylist:p.TESTLIST"},
		sonos.ServiceRef{Service: fav.Service, Account: fav.Account, ItemID: fav.ItemID})
	assert.True(t, strings.HasPrefix(fav.Metadata, "<DIDL-Lite"))
	assert.Contains(t, fav.Metadata, "SA_RINCON52231_X_#Svc52231-0-Token")
	assert.NotContains(t, fav.Metadata, "albumArtURI")
}

// Case: the chooser in the app. It opens over the queue on the track row;
// down and enter mark the playlist, close the chooser, and leave the queue
// open.
func TestMarkChooser_ChoosingThePlaylistStoresIt(t *testing.T) {
	db := openTestStore(t)
	m := pressKey(sizedModel(t), "enter")
	m.db = db
	m = deliver(m, favouriteChooseMsg{track: serviceTrack(), source: playlistSource()})
	require.True(t, m.marking.IsOpen(), "the chooser did not open")
	assert.Equal(t, 0, m.marking.Content.(markContent).cursor, "the chooser did not start on the track")

	m = pressKey(m, "down")
	model, cmd := m.Update(keyMsgFor("enter"))
	m = model.(appModel)
	require.NotNil(t, cmd)
	model, cmd = m.Update(cmd())
	m = model.(appModel)

	assert.False(t, m.marking.IsOpen(), "the chooser stayed open")
	assert.True(t, m.queue.IsOpen(), "choosing closed the queue")
	require.NotNil(t, cmd)
	assert.Equal(t, favouriteMarked, cmd().(panes.NoticeMsg).Text)
	favourites, err := db.Favourites().List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "Kill 'Em All playlist", favourites[0].Title)
	assert.Equal(t, store.FavouritePlaylist, favourites[0].Kind)
}

// Case: esc in the chooser. It closes with nothing marked, and the keys
// reach the queue again, not before.
func TestMarkChooser_EscClosesWithNothingMarked(t *testing.T) {
	m := pressKey(sizedModel(t), "enter")
	m = deliver(m, favouriteChooseMsg{track: serviceTrack(), source: playlistSource()})

	model, cmd := m.Update(keyMsgFor("esc"))
	m = model.(appModel)

	assert.False(t, m.marking.IsOpen())
	assert.True(t, m.queue.IsOpen(), "esc closed the queue too")
	assert.Nil(t, cmd, "esc marked something")
}

// Case: only one thing can be marked, such as a track from a shared folder
// in a playlist queue. That one is marked at once, with no chooser.
func TestMarkChooser_OneChoiceIsMarkedAtOnce(t *testing.T) {
	m := pressKey(sizedModel(t), "enter")
	m.db = openTestStore(t)
	shared := sonos.TrackMetadata{URI: "x-file-cifs://nas/music/a.flac", Title: "A song"}

	model, cmd := m.Update(favouriteChooseMsg{track: shared, source: playlistSource()})

	assert.False(t, model.(appModel).marking.IsOpen(), "a chooser opened with one choice")
	require.NotNil(t, cmd)
	assert.Equal(t, favouriteMarked, cmd().(panes.NoticeMsg).Text)
}

// playlistSource is an Apple Music playlist a queue was started from, as the
// speaker reports it, artwork and all.
func playlistSource() sonos.QueueSource {
	return sonos.QueueSource{
		Title: "Kill 'Em All playlist",
		Class: "object.container.playlistContainer",
		URI:   "x-rincon-cpcontainer:00060000libraryplaylist%3ap.TESTLIST?sid=204&flags=0&sn=2",
		Metadata: `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" ` +
			`xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">` +
			`<item id="00060000libraryplaylist%3ap.TESTLIST" parentID="-1" restricted="true"><dc:title>Kill 'Em All playlist</dc:title>` +
			`<upnp:class>object.container.playlistContainer</upnp:class>` +
			`<desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">SA_RINCON52231_X_#Svc52231-0-Token</desc>` +
			`<upnp:albumArtURI>https://example.com/art.png</upnp:albumArtURI></item></DIDL-Lite>`,
	}
}

func rowLabels(rows []markRow) []string {
	var labels []string
	for _, row := range rows {
		labels = append(labels, row.label)
	}
	return labels
}

package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonos"
	"github.com/haasted/sonos-tui/internal/store"
	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// Case: a in the queue. It must ask for the track under the cursor, and for
// nothing on an empty list.
func TestFavouriteMark_AAsksForTheTrackUnderTheCursor(t *testing.T) {
	q := navQueue(20, 6)

	_, cmd := q.Update(keyMsgFor("a"))
	require.NotNil(t, cmd)
	assert.Equal(t, favouriteMarkMsg{tracks: []sonos.TrackMetadata{q.items[6].TrackMetadata}}, cmd())

	_, cmd = navQueue(0, 0).Update(keyMsgFor("a"))
	assert.Nil(t, cmd, "an empty queue asked to mark something")
}

// Case: a with tracks picked. It asks for the picked tracks in the order they
// were picked, as n does, ignores the cursor, and clears the selection.
func TestFavouriteMark_AMarksThePickedTracksInPressOrder(t *testing.T) {
	q := navQueue(20, 6)
	q = moveQueue(q, "s")
	q.cursor = 2
	q = moveQueue(q, "s")
	q.cursor = 10

	model, cmd := q.Update(keyMsgFor("a"))

	require.NotNil(t, cmd)
	assert.Equal(t, favouriteMarkMsg{tracks: []sonos.TrackMetadata{q.items[6].TrackMetadata, q.items[2].TrackMetadata}}, cmd())
	assert.Zero(t, model.(queueContent).selectionCount(), "the selection was not cleared")
}

// Case: several tracks marked at once: two new, one already a favourite, and
// one from a shared folder. The notice says they were added and counts the
// one skipped; the list shows them in the order given, the old one moved up.
func TestFavouriteMark_SeveralTracksKeepTheirOrder(t *testing.T) {
	db := openTestStore(t)
	m := sizedModel(t)
	m.db = db
	markNotice(t, m, serviceTrack())
	first, second := serviceTrackWithID("1", "First"), serviceTrackWithID("2", "Second")
	shared := sonos.TrackMetadata{URI: "x-file-cifs://nas/music/a.flac", Title: "A song"}

	notice := markNotice(t, m, first, shared, serviceTrack(), second)

	assert.Equal(t, fmt.Sprintf(favouriteMarkedSkipped, 1), notice.Text)
	favourites, err := db.Favourites().List(t.Context())
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(favourites), 3)
	assert.Equal(t, []string{"First", "Seek & Destroy", "Second"},
		[]string{favourites[0].Title, favourites[1].Title, favourites[2].Title})
}

// Case: several tracks, all from music services. The notice is the plain one.
func TestFavouriteMark_SeveralTracksSayOnlyAdded(t *testing.T) {
	m := sizedModel(t)
	m.db = openTestStore(t)

	notice := markNotice(t, m, serviceTrackWithID("1", "First"), serviceTrackWithID("2", "Second"))

	assert.Equal(t, favouriteMarkedSeveral, notice.Text)
}

// Case: a track from a music service, marked twice. The first time stores a
// track row that can be played again; the second adds no row and says so.
func TestFavouriteMark_StoresATrackOnceAndSaysSo(t *testing.T) {
	db := openTestStore(t)
	before := favouriteCount(t, db)
	m := sizedModel(t)
	m.db = db

	notice := markNotice(t, m, serviceTrack())
	assert.Equal(t, favouriteMarked, notice.Text)
	assert.True(t, notice.Confirmation)

	favourites, err := db.Favourites().List(t.Context())
	require.NoError(t, err)
	require.Len(t, favourites, before+1)
	fav := favourites[0]
	assert.Equal(t, store.FavouriteTrack, fav.Kind)
	assert.Equal(t, "Metallica", fav.Artist)
	assert.Equal(t, "Kill 'Em All", fav.Album)
	assert.Equal(t, sonos.ServiceRef{Service: 204, Account: 2, ItemID: "song:1440873907.mp4"},
		sonos.ServiceRef{Service: fav.Service, Account: fav.Account, ItemID: fav.ItemID})
	assert.Contains(t, fav.Metadata, "SA_RINCON52231_", "the DIDL lacks the service's desc")

	notice = markNotice(t, m, serviceTrack())
	assert.Equal(t, favouriteMarkedNot, notice.Text)
	assert.Equal(t, before+1, favouriteCount(t, db), "a second mark added a row")
}

// Case: a track not from a music service, such as one from a shared folder.
// It has no service id to store it by, so the reader is told and nothing is
// stored.
func TestFavouriteMark_ATrackWithNoServiceCannotBeMarked(t *testing.T) {
	db := openTestStore(t)
	before := favouriteCount(t, db)
	m := sizedModel(t)
	m.db = db

	notice := markNotice(t, m, sonos.TrackMetadata{URI: "x-file-cifs://nas/music/a.flac", Title: "A song"})

	assert.Equal(t, favouriteNoService, notice.Text)
	assert.Equal(t, before, favouriteCount(t, db))
}

// markNotice sends a mark for tracks through the app, and returns the notice
// the work in the background answers with.
func markNotice(t *testing.T, m appModel, tracks ...sonos.TrackMetadata) panes.NoticeMsg {
	t.Helper()
	_, cmd := m.Update(favouriteMarkMsg{tracks: tracks})
	require.NotNil(t, cmd)
	notice, ok := cmd().(panes.NoticeMsg)
	require.True(t, ok, "marking did not answer with a notice")
	return notice
}

// serviceTrack is a queue row from Apple Music, which none of the sample
// favourites holds.
func serviceTrack() sonos.TrackMetadata {
	return sonos.TrackMetadata{
		URI:    "x-sonos-http:song%3a1440873907.mp4?sid=204&flags=8224&sn=2",
		Title:  "Seek & Destroy",
		Artist: "Metallica",
		Album:  "Kill 'Em All",
	}
}

// serviceTrackWithID is an Apple Music queue row of its own, by id.
func serviceTrackWithID(id, title string) sonos.TrackMetadata {
	return sonos.TrackMetadata{URI: "x-sonos-http:song%3a" + id + ".mp4?sid=204&flags=8224&sn=2", Title: title}
}

func favouriteCount(t *testing.T, db *store.Store) int {
	t.Helper()
	favourites, err := db.Favourites().List(t.Context())
	require.NoError(t, err)
	return len(favourites)
}

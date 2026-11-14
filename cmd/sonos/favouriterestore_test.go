package main

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/store"
	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// Case: X after x on a row in the middle. The favourite is back in its old
// place, the cursor is on it, and it is asked to be stored again as it was.
func TestFavouriteRestore_PutsTheFavouriteBackInItsPlace(t *testing.T) {
	f := restoreFavourites(store.FavouritePlaylist)
	f = pressOn(f, "down")
	f, _ = removeUnderCursor(t, f, "x")
	require.Equal(t, []string{"Newest", "Oldest"}, favouriteTitles(f))

	f, msgs := restoreLast(t, f)

	assert.Equal(t, []string{"Newest", "Middle", "Oldest"}, favouriteTitles(f))
	assert.Equal(t, 1, f.cursor, "the cursor is not on the favourite put back")
	assert.Contains(t, msgs, tea.Msg(favouriteRestoreMsg{favourite: restoreSamples()[2]}))
}

// Case: X pressed. The favourite fades in, dimmed as a removed one fades out,
// until the last press's tick ends it; an earlier tick does not.
func TestFavouriteRestore_TheRowFadesIn(t *testing.T) {
	f, _ := removeUnderCursor(t, restoreFavourites(store.FavouritePlaylist), "x")
	f, _ = removeUnderCursor(t, f, "x")

	model, _ := f.Update(keyMsgFor("X"))
	f = model.(favouritesContent)
	first := f.comingGen
	model, _ = f.Update(keyMsgFor("X"))
	f = model.(favouritesContent)
	assert.Equal(t, f.items[f.cursor].ID, f.coming, "the row put back is not fading in")

	f = f.arrived(first)
	assert.NotZero(t, f.coming, "an earlier tick ended the fade")
	f = f.arrived(f.comingGen)
	assert.Zero(t, f.coming)
}

// Case: two favourites removed. X puts back the last removed first, then
// the one before it.
func TestFavouriteRestore_TheLastRemovedComesBackFirst(t *testing.T) {
	f, _ := removeUnderCursor(t, restoreFavourites(store.FavouritePlaylist), "x")
	f, _ = removeUnderCursor(t, f, "x")
	require.Equal(t, []string{"Oldest"}, favouriteTitles(f))

	f, _ = restoreLast(t, f)
	assert.Equal(t, []string{"Middle", "Oldest"}, favouriteTitles(f))
	f, _ = restoreLast(t, f)
	assert.Equal(t, []string{"Newest", "Middle", "Oldest"}, favouriteTitles(f))
	assert.Equal(t, 0, f.cursor)
}

// Case: a playlist removed, then → to another type. X goes back to the
// playlists with the playlist in its place, and the type is remembered.
func TestFavouriteRestore_GoesBackToTheTypeItWasRemovedFrom(t *testing.T) {
	f, _ := removeUnderCursor(t, restoreFavourites(store.FavouritePlaylist), "x")
	f = pressOn(f, "left")
	require.Equal(t, store.FavouriteTrack, f.kind)

	f, msgs := restoreLast(t, f)

	assert.Equal(t, store.FavouritePlaylist, f.kind)
	assert.Equal(t, []string{"Newest", "Middle", "Oldest"}, favouriteTitles(f))
	assert.Contains(t, msgs, tea.Msg(favouriteKindChosenMsg{kind: store.FavouritePlaylist}))
}

// Case: the only favourite of a type removed, so the type left the list. X
// brings the type back with it.
func TestFavouriteRestore_BringsBackATypeThatWentEmpty(t *testing.T) {
	f, _ := removeUnderCursor(t, restoreFavourites(store.FavouriteTrack), "x")
	require.Equal(t, store.FavouritePlaylist, f.kind)

	f, _ = restoreLast(t, f)

	assert.Equal(t, store.FavouriteTrack, f.kind)
	assert.Equal(t, []string{"A track"}, favouriteTitles(f))
}

// Case: X while the removed row still fades out. The row is back at once,
// and no longer dimmed as one going.
func TestFavouriteRestore_DuringTheRemovalFade(t *testing.T) {
	model, _ := restoreFavourites(store.FavouritePlaylist).Update(keyMsgFor("x"))

	f, _ := restoreLast(t, model.(favouritesContent))

	assert.Equal(t, []string{"Newest", "Middle", "Oldest"}, favouriteTitles(f))
	assert.False(t, f.isGoing(f.items[0]))
	assert.Equal(t, 0, f.cursor)
}

// Case: a filter that does not match the favourite put back. The filter is
// closed, so the favourite is seen.
func TestFavouriteRestore_AFilterThatHidesItIsClosed(t *testing.T) {
	f, _ := removeUnderCursor(t, restoreFavourites(store.FavouritePlaylist), "x")
	f = pressOn(f, "/", "o", "l", "d", "enter")
	require.Equal(t, []string{"Oldest"}, favouriteTitles(f))

	f, _ = restoreLast(t, f)

	assert.Empty(t, f.filter)
	assert.Equal(t, []string{"Newest", "Middle", "Oldest"}, favouriteTitles(f))
	assert.Equal(t, 0, f.cursor)
}

// Case: X with nothing removed since the modal opened. Nothing happens.
func TestFavouriteRestore_NothingRemovedDoesNothing(t *testing.T) {
	_, cmd := restoreFavourites(store.FavouritePlaylist).Update(keyMsgFor("X"))
	assert.Nil(t, cmd)
}

// Case: the restore through a real database. The row is back with its id
// and added time, and the notice says "restored".
func TestFavouriteRestore_StoresTheRowAgainAndSaysSo(t *testing.T) {
	db := openTestStore(t)
	_, err := db.Favourites().Add(t.Context(), stationFavouriteRow())
	require.NoError(t, err)
	stored, err := db.Favourites().List(t.Context())
	require.NoError(t, err)
	require.NoError(t, db.Favourites().Remove(t.Context(), stored[0].ID))
	m := sizedModel(t)
	m.db = db

	_, cmd := m.Update(favouriteRestoreMsg{favourite: stored[0]})
	require.NotNil(t, cmd)
	notice, ok := cmd().(panes.NoticeMsg)
	require.True(t, ok, "restoring did not answer with a notice")

	assert.Equal(t, favouriteRestored, notice.Text)
	assert.True(t, notice.Confirmation)
	back, err := db.Favourites().List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, stored, back)
}

// restoreFavourites is the modal on kind, with a track and three playlists
// added at different times, newest first.
func restoreFavourites(kind store.FavouriteKind) favouritesContent {
	return favouritesContent{listView: listView[store.Favourite]{rows: 12, width: 60}}.loaded(
		favouritesLoadedMsg{favourites: restoreSamples(), kind: kind})
}

func restoreSamples() []store.Favourite {
	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	return []store.Favourite{
		{ID: 4, Kind: store.FavouriteTrack, Title: "A track", AddedAt: at},
		{ID: 3, Kind: store.FavouritePlaylist, Title: "Newest", AddedAt: at.Add(-time.Hour)},
		{ID: 2, Kind: store.FavouritePlaylist, Title: "Middle", AddedAt: at.Add(-2 * time.Hour)},
		{ID: 1, Kind: store.FavouritePlaylist, Title: "Oldest", AddedAt: at.Add(-3 * time.Hour)},
	}
}

// restoreLast presses X and returns the modal and every message asked for.
// The fade-in tick is not run.
func restoreLast(t *testing.T, f favouritesContent) (favouritesContent, []tea.Msg) {
	t.Helper()
	model, cmd := f.Update(keyMsgFor("X"))
	require.NotNil(t, cmd, "X asked for nothing")
	var msgs []tea.Msg
	for _, msg := range flatten(cmd) {
		if _, tick := msg.(favouriteArrivedMsg); !tick {
			msgs = append(msgs, msg)
		}
	}
	return model.(favouritesContent), msgs
}

// pressOn presses each key on the modal in turn.
func pressOn(f favouritesContent, keys ...string) favouritesContent {
	for _, key := range keys {
		model, _ := f.Update(keyMsgFor(key))
		f = model.(favouritesContent)
	}
	return f
}

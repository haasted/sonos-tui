package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/store"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: the modal opening on the media type shown last. It must show that
// type's favourites, and only those.
func TestFavourites_OpensOnTheTypeShownLast(t *testing.T) {
	f := loadedFavourites(store.FavouritePlaylist)

	assert.Equal(t, store.FavouritePlaylist, f.kind)
	assert.Equal(t, []string{"Awesome singles", "Metallica"}, favouriteTitles(f))
}

// Case: the type shown last has no favourites now. The modal must open on
// the first type that has some.
func TestFavourites_AnEmptyRememberedTypeFallsBackToTheFirst(t *testing.T) {
	f := loadedFavourites(store.FavouriteStation)

	assert.Equal(t, store.FavouriteTrack, f.kind)
}

// Case: → and ← walking the types. Albums has no favourites here and must
// be skipped; each switch must ask for the new type to be remembered.
func TestFavourites_ArrowsSkipEmptyTypes(t *testing.T) {
	f := loadedFavourites(store.FavouriteTrack)

	model, cmd := f.Update(keyMsgFor("right"))
	f = model.(favouritesContent)
	assert.Equal(t, store.FavouritePlaylist, f.kind, "the empty album type was not skipped")
	require.NotNil(t, cmd)
	assert.Equal(t, favouriteKindChosenMsg{kind: store.FavouritePlaylist}, cmd())

	model, _ = f.Update(keyMsgFor("left"))
	assert.Equal(t, store.FavouriteTrack, model.(favouritesContent).kind)
}

// Case: → on the last type. There is nowhere to go, so the edge lights, as
// at the end of a list, and nothing is remembered.
func TestFavourites_TheLastTypeBumpsTheEdge(t *testing.T) {
	f := loadedFavourites(store.FavouritePlaylist)

	model, cmd := f.Update(keyMsgFor("right"))

	assert.Equal(t, store.FavouritePlaylist, model.(favouritesContent).kind)
	assert.True(t, model.(favouritesContent).bumping())
	assert.Nil(t, cmd)

	light := model.(favouritesContent).borderLight()
	require.NotNil(t, light)
	assert.NotNil(t, light(panes.ModalRight, 0).Color, "the right side must answer a press against it")
	assert.Nil(t, light(panes.ModalTop, 0).Color, "the top lit for a sideways press")
}

// Case: a notice arriving while the favourites are open, such as the one n
// sends. It must show in the modal's border, where the reader is looking,
// since the tab row it would go to is under the modal.
func TestFavourites_ANoticeShowsInTheModal(t *testing.T) {
	m := pressKey(sizedModel(t), "f")

	m = deliver(m, panes.NoticeMsg{Level: panes.NoticeInfo, Text: "Metallica plays next", Confirmation: true})

	assert.True(t, m.favourites.NoticeShowing(), "the notice did not reach the modal")
}

// Case: the heading. It names the type, marks with ‹ and › only the sides ←
// and → can still go to, and counts what is shown.
func TestFavourites_TheHeadingNamesTheType(t *testing.T) {
	assert.Equal(t, "Favourites ·    Tracks   › · 1", loadedFavourites(store.FavouriteTrack).heading(),
		"the first type offered a way back")
	assert.Equal(t, "Favourites · ‹ Playlists   · 2", loadedFavourites(store.FavouritePlaylist).heading(),
		"the last type offered a way on")

	single := favouritesContent{listView: listView[store.Favourite]{rows: 12, width: 60}}.loaded(
		favouritesLoadedMsg{favourites: sampleFavourites()[1:3], kind: store.FavouritePlaylist})
	assert.Equal(t, "Favourites ·   Playlists   · 2", single.heading(), "‹ › promised a switch with one type")
}

// Case: the arrows either side of the type. They must stand apart from the
// name in a colour of their own, while the name keeps the heading's colour.
func TestFavourites_TheArrowsHaveTheirOwnColour(t *testing.T) {
	parts := loadedFavourites(store.FavouritePlaylist).headingParts()

	var arrow, name headingPart
	for _, part := range parts {
		switch strings.TrimSpace(part.text) {
		case "‹":
			arrow = part
		case "Playlists":
			name = part
		}
	}
	assert.Equal(t, themedArrowLit(), arrow.style.GetForeground(), "‹ is not in the arrow colour")
	assert.Equal(t, favouritesBorderColor(), name.style.GetForeground(), "the name lost the heading colour")
}

// Case: switching between types whose names differ in length, with arrows
// coming and going. The label must keep one width, so the count after it
// never moves.
func TestFavourites_TheHeadingKeepsOneWidthAcrossTypes(t *testing.T) {
	all := []store.Favourite{
		{Kind: store.FavouriteTrack, Title: "a"}, {Kind: store.FavouriteAlbum, Title: "b"},
		{Kind: store.FavouritePlaylist, Title: "c"}, {Kind: store.FavouriteStation, Title: "d"},
	}
	widths := map[int]bool{}
	for _, kind := range favouriteKinds {
		f := favouritesContent{listView: listView[store.Favourite]{rows: 12, width: 60}}.loaded(
			favouritesLoadedMsg{favourites: all, kind: kind})
		widths[lipgloss.Width(f.heading())] = true
	}
	assert.Len(t, widths, 1, "the heading changed width between types")
}

// Case: no favourites at all. The body says so, and the heading names no type.
func TestFavourites_NoneAtAllSaysSo(t *testing.T) {
	f := favouritesContent{listView: listView[store.Favourite]{rows: 12, width: 60}}.loaded(favouritesLoadedMsg{})

	assert.Contains(t, plain(f.View().Content), "No favourites yet.")
	assert.Equal(t, "Favourites", f.heading())
	assert.False(t, f.hintReady())
}

// Case: the three keys that act. Each must ask for the favourite under the
// cursor, with its own action.
func TestFavourites_KeysAskForTheFavouriteUnderTheCursor(t *testing.T) {
	f := loadedFavourites(store.FavouritePlaylist)
	model, _ := f.Update(keyMsgFor("down"))
	f = model.(favouritesContent)

	for key, want := range map[string]favouriteAction{"enter": favouritePlay, "n": favouritePlayNext, "N": favouriteAddToQueue} {
		_, cmd := f.Update(keyMsgFor(key))
		require.NotNil(t, cmd, key)
		msg := cmd().(favouriteActionMsg)
		assert.Equal(t, "Metallica", msg.favourite.Title, key)
		assert.Equal(t, want, msg.action, key)
	}
}

// Case: f on the player. It must open the favourites modal, and esc must
// close it again.
func TestFavourites_FOpensAndEscCloses(t *testing.T) {
	m := sizedModel(t)

	m = pressKey(m, "f")
	require.True(t, m.favourites.IsOpen(), "f did not open the favourites")

	m = pressKey(m, "esc")
	assert.False(t, m.favourites.IsOpen(), "esc did not close the favourites")
}

// Case: n on a station. Stations cannot be queued, so the reader is told and
// nothing is sent; the modal stays open.
func TestFavourites_AStationCannotBeQueued(t *testing.T) {
	m := pressKey(sizedModel(t), "f")
	station := store.Favourite{Kind: store.FavouriteStation, Title: "DR P1"}

	model, cmd := m.Update(favouriteActionMsg{favourite: station, action: favouritePlayNext})

	require.NotNil(t, cmd)
	notice, ok := cmd().(panes.NoticeMsg)
	require.True(t, ok, "expected a notice")
	assert.Equal(t, favouriteStationNotQueued, notice.Text)
	assert.True(t, model.(appModel).favourites.IsOpen())
}

// Case: enter on a favourite. It closes the modal, as enter does in the queue.
func TestFavourites_EnterClosesTheModal(t *testing.T) {
	m := pressKey(sizedModel(t), "f")

	model, cmd := m.Update(favouriteActionMsg{favourite: sampleFavourites()[1], action: favouritePlay})

	assert.NotNil(t, cmd)
	assert.False(t, model.(appModel).favourites.IsOpen())
}

// Case: the whole round through a real database. The modal reads the
// favourites and the type shown last, and a switch is written back.
func TestFavourites_ReadsAndRemembersThroughTheDatabase(t *testing.T) {
	db := openTestStore(t)
	require.NoError(t, db.Settings().Set(t.Context(), store.SettingFavouritesKind, string(store.FavouriteAlbum)))

	m := sizedModel(t)
	m.db = db
	model, _ := m.Update(keyMsgFor("f"))
	m = model.(appModel)
	m = deliver(m, loadFavourites(t.Context(), db)())

	content := m.favourites.Content.(favouritesContent)
	assert.Equal(t, store.FavouriteAlbum, content.kind)

	_, cmd := m.Update(favouriteKindChosenMsg{kind: store.FavouriteStation})
	require.NotNil(t, cmd)
	cmd()
	kind, err := db.Settings().Get(t.Context(), store.SettingFavouritesKind, "")
	require.NoError(t, err)
	assert.Equal(t, string(store.FavouriteStation), kind)
}

// Case: the sample favourites a migration seeds. Their service, account and
// id columns must be what the URI says, as marking will store them.
func TestFavourites_TheSampleRowsMatchTheirURIs(t *testing.T) {
	favourites, err := openTestStore(t).Favourites().List(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, favourites, "the sample favourites are missing")

	for _, fav := range favourites {
		ref, ok := sonos.ParseServiceURI(fav.URI)
		require.True(t, ok, fav.Title)
		assert.Equal(t, sonos.ServiceRef{Service: fav.Service, Account: fav.Account, ItemID: fav.ItemID}, ref, fav.Title)
		assert.True(t, strings.HasPrefix(fav.Metadata, "<DIDL-Lite"), fav.Title)
	}
}

// loadedFavourites is the modal filled with sampleFavourites, on kind.
func loadedFavourites(kind store.FavouriteKind) favouritesContent {
	return favouritesContent{listView: listView[store.Favourite]{rows: 12, width: 60}}.loaded(
		favouritesLoadedMsg{favourites: sampleFavourites(), kind: kind})
}

// sampleFavourites has a track and two playlists, and no album or station,
// so ← and → have an empty type to skip.
func sampleFavourites() []store.Favourite {
	at := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	return []store.Favourite{
		{ID: 1, Kind: store.FavouriteTrack, Title: "Paint The Town Red", Artist: "Doja Cat", AddedAt: at},
		{ID: 2, Kind: store.FavouritePlaylist, Title: "Awesome singles", AddedAt: at},
		{ID: 3, Kind: store.FavouritePlaylist, Title: "Metallica", AddedAt: at},
	}
}

func favouriteTitles(f favouritesContent) []string {
	var out []string
	for _, fav := range f.items {
		out = append(out, fav.Title)
	}
	return out
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "sonos.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

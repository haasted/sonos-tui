package main

import (
	"context"
	"fmt"
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

// Case: the heading of a short list. It names the modal on the left and the
// type at the right edge, with both arrows, and counts nothing: a few rows
// can be counted at a glance.
func TestFavourites_TheHeadingNamesTheTypeAtTheRight(t *testing.T) {
	heading := loadedFavourites(store.FavouritePlaylist).heading()

	assert.Equal(t, "Favourites", heading[:len("Favourites")])
	assert.True(t, strings.HasSuffix(heading, "‹ Playlists ›"), "the type is not at the right edge: %q", heading)
	assert.Equal(t, 60, lipgloss.Width(heading), "the heading does not reach the right edge")
	assert.NotContains(t, heading, headingSeparator, "a short list was counted")
}

// Case: a list longer than favouritesCountFloor. The count sits beside the
// modal's name, after the separator; one row fewer shows neither.
func TestFavourites_ALongerListIsCounted(t *testing.T) {
	assert.True(t, strings.HasPrefix(stationsHeading(favouritesCountFloor+1), "Favourites · 8 "))
	assert.False(t, strings.Contains(stationsHeading(favouritesCountFloor), headingSeparator),
		"a list of %d was counted", favouritesCountFloor)
}

// Case: a list longer than the box. The heading says which part is showing,
// whatever the floor, since that is not something seen at a glance.
func TestFavourites_AnOverflowingListSaysWhichPartShows(t *testing.T) {
	f := favouritesContent{listView: listView[store.Favourite]{rows: 4, width: 60}}.loaded(
		favouritesLoadedMsg{favourites: stations(5), kind: store.FavouriteStation})

	assert.True(t, strings.HasPrefix(f.heading(), "Favourites · 1-2 of 5 "), f.heading())
}

// Case: the arrows either side of the type. Both are always drawn; one
// pointing where ← or → can go is lit, one pointing nowhere is subdued. The
// name keeps the heading's colour.
func TestFavourites_AnArrowWithNowhereToGoIsSubdued(t *testing.T) {
	parts := loadedFavourites(store.FavouritePlaylist).headingParts() // the last type with favourites

	styles := map[string]headingPart{}
	for _, part := range parts {
		styles[strings.TrimSpace(part.text)] = part
	}
	assert.Equal(t, themedArrowLit(), styles["‹"].style.GetForeground(), "‹ leads back to tracks and is not lit")
	assert.Equal(t, themedArrowRest(), styles["›"].style.GetForeground(), "› leads nowhere and is not subdued")
	assert.NotEqual(t, favouritesBorderColor(), styles["Playlists"].style.GetForeground(), "the name does not stand out from the heading")
	assert.Equal(t, lipgloss.NoColor{}, styles["Playlists"].style.GetBackground(), "the name has a background")
}

// Case: → on the last type. The subdued › flashes in the border's bump
// colour, as the right border does, and ‹ stays as it was; once the bump has
// cooled, › is subdued again.
func TestFavourites_APressedArrowFlashesWithTheBorder(t *testing.T) {
	model, _ := loadedFavourites(store.FavouritePlaylist).Update(keyMsgFor("right"))
	f := model.(favouritesContent)

	styles := arrowStyles(f)
	assert.NotEqual(t, themedArrowRest(), styles["›"].GetForeground(), "› stayed subdued through the press")
	assert.True(t, f.lights(panes.ModalRight), "the right border did not light with it")
	assert.Equal(t, themedArrowLit(), styles["‹"].GetForeground(), "‹ changed though nothing pressed against it")

	f.bumpedAt = time.Now().Add(-2 * bumpDuration)
	assert.Equal(t, themedArrowRest(), arrowStyles(f)["›"].GetForeground(), "› stayed lit after the bump cooled")
}

// Case: every type, in a box of one width. The heading fills the box on
// every one, so the type's right edge never moves as ← and → switch.
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

// Case: a box too narrow for the name, the count and the type. The count
// goes first; the type, which ← and → change, stays.
func TestFavourites_ANarrowBoxDropsTheCountFirst(t *testing.T) {
	f := favouritesContent{listView: listView[store.Favourite]{rows: 20, width: 24}}.loaded(
		favouritesLoadedMsg{favourites: stations(favouritesCountFloor + 1), kind: store.FavouriteStation})

	heading := f.heading()
	assert.NotContains(t, heading, headingSeparator, "the count stayed in a narrow box")
	assert.True(t, strings.HasSuffix(heading, "‹ Stations ›"), "the type was dropped: %q", heading)
	assert.Equal(t, 24, lipgloss.Width(heading))
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

	for key, want := range map[string]favouriteAction{
		"enter": favouritePlay, "shift+enter": favouritePlayShuffled, "n": favouritePlayNext, "N": favouriteAddToQueue,
	} {
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

// Case: shift+enter on a track or a station. Neither has anything to shuffle,
// so the reader is told, nothing is sent, and the modal stays open.
func TestFavourites_OnlyAnAlbumOrPlaylistShuffles(t *testing.T) {
	m := pressKey(sizedModel(t), "f")

	for _, kind := range []store.FavouriteKind{store.FavouriteTrack, store.FavouriteStation} {
		model, cmd := m.Update(favouriteActionMsg{favourite: store.Favourite{Kind: kind, Title: "x"}, action: favouritePlayShuffled})

		require.NotNil(t, cmd, kind)
		assert.Equal(t, favouriteNotShuffled, cmd().(panes.NoticeMsg).Text, kind)
		assert.True(t, model.(appModel).favourites.IsOpen(), kind)
	}
}

// Case: shift+enter on a playlist. It closes the modal, as enter does.
func TestFavourites_ShiftEnterClosesTheModal(t *testing.T) {
	m := pressKey(sizedModel(t), "f")

	model, cmd := m.Update(favouriteActionMsg{favourite: sampleFavourites()[1], action: favouritePlayShuffled})

	assert.NotNil(t, cmd)
	assert.False(t, model.(appModel).favourites.IsOpen())
}

// Case: a playlist started shuffled. The speaker is told to shuffle and is not
// sent to track 1, which only an in-order start asks for.
func TestFavourites_AShuffledStartTellsTheSpeakerToShuffle(t *testing.T) {
	device, heard := recordingDevice(t)
	device.Speaker.USN = "uuid:RINCON_TEST01400::urn:schemas-upnp-org:device:ZonePlayer:1" // selecting the queue needs the id
	speaker := speakerModel{ctx: t.Context(), device: device}
	playable := sonos.Playable{URI: "x-rincon-cpcontainer:00060000libraryplaylist%3ap.X?sid=204&flags=0&sn=2", Metadata: "<DIDL-Lite/>", Title: "X"}

	startFavourite(speaker, playable, favouritePlayShuffled, false)()

	actions := strings.Join(heard(), " ")
	assert.Contains(t, actions, "#SetPlayMode")
	assert.NotContains(t, actions, "#Seek", "a shuffled start was sent to track 1")
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
	album := stationFavouriteRow()
	album.Kind, album.ItemID = store.FavouriteAlbum, "album:1888147421"
	_, err := db.Favourites().Add(t.Context(), stationFavouriteRow(), album)
	require.NoError(t, err)
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

// arrowStyles is the heading's two arrows by glyph.
func arrowStyles(f favouritesContent) map[string]lipgloss.Style {
	styles := map[string]lipgloss.Style{}
	for _, part := range f.headingParts() {
		if part.text == "‹" || part.text == "›" {
			styles[part.text] = part.style
		}
	}
	return styles
}

// stationsHeading is the heading of a list of n stations with room to spare.
func stationsHeading(n int) string {
	return favouritesContent{listView: listView[store.Favourite]{rows: 20, width: 60}}.loaded(
		favouritesLoadedMsg{favourites: stations(n), kind: store.FavouriteStation}).heading()
}

// stations is n station favourites, each of its own.
func stations(n int) []store.Favourite {
	favs := make([]store.Favourite, n)
	for i := range favs {
		favs[i] = store.Favourite{ID: int64(i + 1), Kind: store.FavouriteStation, Title: fmt.Sprintf("Station %d", i+1)}
	}
	return favs
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

// stationFavouriteRow is a station favourite to seed a test database with.
func stationFavouriteRow() store.Favourite {
	return store.Favourite{
		Kind: store.FavouriteStation, Title: "DR P1", URI: "x-sonosapi-hls:urn%3adr%3aradio%3achannel%3ap1?sid=290&flags=8232&sn=3",
		Metadata: "<DIDL-Lite/>", Service: 290, Account: 3, ItemID: "urn:dr:radio:channel:p1", AddedAt: time.Now(),
	}
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "sonos.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

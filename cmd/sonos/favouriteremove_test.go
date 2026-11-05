package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/store"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: x on a favourite with more below it. Once the fade is over it is off
// the list, the cursor keeps its row and so lands on the next one, and the
// delete is asked for.
func TestFavouriteRemove_TheCursorLandsOnTheNextRow(t *testing.T) {
	f := loadedFavourites(store.FavouritePlaylist)

	f, msgs := removeUnderCursor(t, f, "x")

	assert.Equal(t, []string{"Metallica"}, favouriteTitles(f))
	assert.Equal(t, 0, f.cursor)
	assert.Equal(t, []tea.Msg{favouriteRemoveMsg{favourite: sampleFavourites()[1]}}, msgs)
}

// Case: x pressed. Before the fade ends, the row is still listed but dimmed,
// the delete is asked for at once, and the cursor has moved off the row.
func TestFavouriteRemove_TheRowFadesBeforeItGoes(t *testing.T) {
	f := loadedFavourites(store.FavouritePlaylist)

	model, cmd := f.Update(keyMsgFor("x"))
	f = model.(favouritesContent)

	assert.Equal(t, []string{"Awesome singles", "Metallica"}, favouriteTitles(f), "the row left before its fade")
	assert.True(t, f.isGoing(f.items[0]))
	assert.Equal(t, 1, f.cursor, "the cursor stayed on the fading row")
	require.NotNil(t, cmd)
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)
	assert.Equal(t, favouriteRemoveMsg{favourite: sampleFavourites()[1]}, batch[0]())

	_, cmd = f.Update(keyMsgFor("x"))
	require.NotNil(t, cmd, "a second x should take the next favourite")
}

// Case: a tick from an earlier press arriving after a later one. It must not
// end the later press's fade early.
func TestFavouriteRemove_AnEarlierTickIsIgnored(t *testing.T) {
	f := loadedFavourites(store.FavouritePlaylist)
	model, _ := f.Update(keyMsgFor("x"))
	first := model.(favouritesContent).goingGen
	model, _ = model.(favouritesContent).Update(keyMsgFor("x"))

	f, _ = model.(favouritesContent).faded(first)

	assert.Len(t, f.items, 2, "the earlier tick ended the fade")
}

// Case: → pressed while a row fades. The fade ends at once, so the row is not
// found still fading on the way back.
func TestFavouriteRemove_SwitchingTypeEndsTheFade(t *testing.T) {
	f := loadedFavourites(store.FavouritePlaylist)
	model, _ := f.Update(keyMsgFor("x"))

	model, _ = model.(favouritesContent).Update(keyMsgFor("left"))
	model, _ = model.(favouritesContent).Update(keyMsgFor("right"))

	assert.Equal(t, []string{"Metallica"}, favouriteTitles(model.(favouritesContent)))
}

// Case: delete on the last row. There is no row below, so the cursor moves
// up one, as in the queue.
func TestFavouriteRemove_OnTheLastRowTheCursorMovesUp(t *testing.T) {
	f := loadedFavourites(store.FavouritePlaylist)
	model, _ := f.Update(keyMsgFor("down"))

	f, _ = removeUnderCursor(t, model.(favouritesContent), "delete")

	assert.Equal(t, []string{"Awesome singles"}, favouriteTitles(f))
	assert.Equal(t, 0, f.cursor)
}

// Case: the only favourite of a media type removed. The next type with
// favourites is shown, and remembered as ← and → would.
func TestFavouriteRemove_TheLastOfATypeMovesToTheNext(t *testing.T) {
	f, msgs := removeUnderCursor(t, loadedFavourites(store.FavouriteTrack), "x")

	assert.Equal(t, store.FavouritePlaylist, f.kind)
	assert.Equal(t, []string{"Awesome singles", "Metallica"}, favouriteTitles(f))
	assert.Contains(t, msgs, tea.Msg(favouriteKindChosenMsg{kind: store.FavouritePlaylist}))
}

// Case: the last type with favourites emptied. No type follows, so the one
// before it is shown.
func TestFavouriteRemove_TheLastTypeMovesBack(t *testing.T) {
	f := loadedFavourites(store.FavouritePlaylist)

	f, _ = removeUnderCursor(t, f, "x")
	f, msgs := removeUnderCursor(t, f, "x")

	assert.Equal(t, store.FavouriteTrack, f.kind)
	assert.Contains(t, msgs, tea.Msg(favouriteKindChosenMsg{kind: store.FavouriteTrack}))
}

// Case: the very last favourite removed. The modal says it has none, and no
// media type is remembered, since none is shown.
func TestFavouriteRemove_NoneLeftSaysSo(t *testing.T) {
	f := favouritesContent{listView: listView[store.Favourite]{rows: 12, width: 60}}.loaded(
		favouritesLoadedMsg{favourites: sampleFavourites()[:1], kind: store.FavouriteTrack})

	f, msgs := removeUnderCursor(t, f, "x")

	assert.Contains(t, plain(f.View().Content), "No favourites yet.")
	assert.Len(t, msgs, 1, "only the delete should be asked for")

	_, cmd := f.Update(keyMsgFor("x"))
	assert.Nil(t, cmd, "x on an empty list asked for something")
}

// Case: the delete through a real database. The row goes, and the notice
// says "removed", as for a track in the queue.
func TestFavouriteRemove_DeletesTheRowAndSaysSo(t *testing.T) {
	db := openTestStore(t)
	stored, err := db.Favourites().List(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, stored)
	m := sizedModel(t)
	m.db = db

	_, cmd := m.Update(favouriteRemoveMsg{favourite: stored[0]})
	require.NotNil(t, cmd)
	notice, ok := cmd().(panes.NoticeMsg)
	require.True(t, ok, "removing did not answer with a notice")

	assert.Equal(t, favouriteRemoved, notice.Text)
	assert.True(t, notice.Confirmation)
	assert.Equal(t, len(stored)-1, favouriteCount(t, db))
}

// removeUnderCursor presses key on the modal and lets the fade run out. It
// returns the modal after that, and every message asked for on the way.
func removeUnderCursor(t *testing.T, f favouritesContent, key string) (favouritesContent, []tea.Msg) {
	t.Helper()
	model, cmd := f.Update(keyMsgFor(key))
	require.NotNil(t, cmd, "%s asked for nothing", key)
	f = model.(favouritesContent)

	var msgs []tea.Msg
	for _, msg := range flatten(cmd) {
		faded, ok := msg.(favouriteFadedMsg)
		if !ok {
			msgs = append(msgs, msg)
			continue
		}
		var settled tea.Cmd
		f, settled = f.faded(faded.gen)
		msgs = append(msgs, flatten(settled)...)
	}
	return f, msgs
}

// flatten runs a command and every command a batch holds, in order.
func flatten(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var msgs []tea.Msg
	for _, branch := range batch {
		msgs = append(msgs, flatten(branch)...)
	}
	return msgs
}

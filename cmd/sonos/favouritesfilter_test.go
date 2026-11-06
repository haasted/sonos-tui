package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/store"
)

// Case: / and some letters. The list narrows to the favourites whose title
// or artist holds them, and the heading shows the box beside the name and
// how many of the type are showing.
func TestFavouritesFilter_TypingNarrowsTheType(t *testing.T) {
	f := pressOnFavourites(filteredFavourites(), "/", "m", "e", "t")

	assert.Equal(t, []string{"Metallica"}, favouriteTitles(f))
	assert.True(t, strings.HasPrefix(f.heading(), "Favourites · /met · 1 of 2"), f.heading())
	assert.True(t, strings.HasSuffix(f.heading(), "‹ Playlists ›"), "the type left the right edge")
}

// Case: letters that are keys elsewhere. While the box is open they are
// typed, not acted on: x types an x and removes nothing.
func TestFavouritesFilter_TheBoxTakesTheKeys(t *testing.T) {
	f := pressOnFavourites(filteredFavourites(), "/", "x")

	assert.Equal(t, "x", f.filter)
	assert.Empty(t, f.going, "x removed a favourite while typing")
}

// Case: ← and → after the box is left with enter. The filter stays and
// narrows the next type too, so one search runs across the pages.
func TestFavouritesFilter_TheFilterStaysAcrossTypes(t *testing.T) {
	f := pressOnFavourites(filteredFavourites(), "/", "d", "o", "j", "a", "enter")

	f = pressOnFavourites(f, "left")

	assert.Equal(t, store.FavouriteTrack, f.kind)
	assert.Equal(t, "doja", f.filter)
	assert.Equal(t, []string{"Paint The Town Red"}, favouriteTitles(f), "the artist did not match")
}

// Case: ← and → while the box is still open. They switch type, and the box
// stays open with what was typed, so the search goes on on the next page.
func TestFavouritesFilter_ArrowsSwitchTypeWhileTyping(t *testing.T) {
	f := pressOnFavourites(filteredFavourites(), "/", "d", "o", "j", "a")

	f = pressOnFavourites(f, "left")

	assert.Equal(t, store.FavouriteTrack, f.kind, "← did not switch type while typing")
	assert.True(t, f.filtering, "switching type closed the box")
	assert.Equal(t, []string{"Paint The Town Red"}, favouriteTitles(f))

	f = pressOnFavourites(f, "right")
	assert.Equal(t, store.FavouritePlaylist, f.kind, "→ did not switch back")
}

// Case: x on a filtered list. It removes the favourite shown under the
// cursor, and the list stays filtered after the fade.
func TestFavouritesFilter_RemovingWorksOnTheFilteredList(t *testing.T) {
	f := pressOnFavourites(filteredFavourites(), "/", "s", "i", "n", "enter")
	require.Equal(t, []string{"Awesome singles"}, favouriteTitles(f))

	f, msgs := removeUnderCursor(t, f, "x")

	assert.Contains(t, msgs, any(favouriteRemoveMsg{favourite: sampleFavourites()[1]}))
	assert.Equal(t, "sin", f.filter, "removing cleared the filter")
	assert.Empty(t, favouriteTitles(f))
}

// Case: esc with a filter in place. The first esc clears the filter and the
// modal stays; a second closes it.
func TestFavouritesFilter_EscClearsBeforeItCloses(t *testing.T) {
	m := pressKey(sizedModel(t), "f")
	m = deliver(m, favouritesLoadedMsg{favourites: sampleFavourites(), kind: store.FavouritePlaylist})
	m = pressKey(pressKey(pressKey(m, "/"), "m"), "enter")

	m = pressKey(m, "esc")
	require.True(t, m.favourites.IsOpen(), "the first esc closed the modal")
	assert.Equal(t, "", m.favourites.Content.(favouritesContent).filter)

	m = pressKey(m, "esc")
	assert.False(t, m.favourites.IsOpen())
}

// filteredFavourites is the sample favourites on the playlists, with room
// to show them all.
func filteredFavourites() favouritesContent {
	return loadedFavourites(store.FavouritePlaylist)
}

// pressOnFavourites presses keys on the favourites modal in order.
func pressOnFavourites(f favouritesContent, keys ...string) favouritesContent {
	for _, key := range keys {
		model, _ := f.Update(keyMsgFor(key))
		f = model.(favouritesContent)
	}
	return f
}

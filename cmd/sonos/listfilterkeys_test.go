package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/store"
)

// Case: q and ? typed into the queue's filter box. They are text: the
// program does not quit and the help does not open.
func TestListFilterKeys_QueueBoxTakesQAndQuestionMark(t *testing.T) {
	m := pressKeys(queueWithBoxOpen(t), "q", "?")

	assert.False(t, m.quitting, "q in the box quit the program")
	assert.False(t, m.help.IsOpen(), "? in the box opened the help")
	require.True(t, m.queue.IsOpen())
	assert.Equal(t, "q?", m.queue.Content.(queueContent).filter)
}

// Case: q and ? typed into the favourites' filter box. They are text, as in
// the queue's.
func TestListFilterKeys_FavouritesBoxTakesQAndQuestionMark(t *testing.T) {
	m := pressKeys(favouritesWithBoxOpen(t), "q", "?")

	assert.False(t, m.quitting, "q in the box quit the program")
	assert.False(t, m.help.IsOpen(), "? in the box opened the help")
	require.True(t, m.favourites.IsOpen())
	assert.Equal(t, "q?", m.favourites.Content.(favouritesContent).filter)
}

// Case: ctrl+c while a filter box is open. It is not text, and it quits.
func TestListFilterKeys_CtrlCQuitsWhileTyping(t *testing.T) {
	for name, m := range map[string]appModel{
		"queue":      queueWithBoxOpen(t),
		"favourites": favouritesWithBoxOpen(t),
	} {
		assert.True(t, pressKey(m, "ctrl+c").quitting, "%s: ctrl+c did not quit", name)
	}
}

// Case: q and ? on an open list with no box open, before any filter and
// after enter has kept the narrowing. They keep their global meaning.
func TestListFilterKeys_GlobalKeysWorkWithTheBoxClosed(t *testing.T) {
	for name, m := range map[string]appModel{
		"queue":               queueOpen(t),
		"queue narrowed":      pressKeys(queueWithBoxOpen(t), "1", "enter"),
		"favourites":          favouritesOpen(t),
		"favourites narrowed": pressKeys(favouritesWithBoxOpen(t), "m", "enter"),
	} {
		assert.True(t, pressKey(m, "q").quitting, "%s: q did not quit", name)
		assert.True(t, pressKey(m, "?").help.IsOpen(), "%s: ? did not open the help", name)
	}
}

// queueOpen is the model with a loaded queue open.
func queueOpen(t *testing.T) appModel {
	t.Helper()

	m, _ := deliverQueue(t, 12)
	require.True(t, m.queue.IsOpen(), "the queue did not open")
	return m
}

// queueWithBoxOpen is the open queue with its filter box open.
func queueWithBoxOpen(t *testing.T) appModel {
	t.Helper()

	m := pressKey(queueOpen(t), "/")
	require.True(t, m.queue.Content.(queueContent).filtering, "the box did not open")
	return m
}

// favouritesOpen is the model with the playlist favourites open.
func favouritesOpen(t *testing.T) appModel {
	t.Helper()

	m := pressKey(sizedModel(t), "f")
	m = deliver(m, favouritesLoadedMsg{favourites: sampleFavourites(), kind: store.FavouritePlaylist})
	require.True(t, m.favourites.IsOpen(), "the favourites did not open")
	return m
}

// favouritesWithBoxOpen is the open favourites with the filter box open.
func favouritesWithBoxOpen(t *testing.T) appModel {
	t.Helper()

	m := pressKey(favouritesOpen(t), "/")
	require.True(t, m.favourites.Content.(favouritesContent).filtering, "the box did not open")
	return m
}

// pressKeys presses keys on the model in order.
func pressKeys(m appModel, keys ...string) appModel {
	for _, key := range keys {
		m = pressKey(m, key)
	}
	return m
}

package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// Case: → on an artist in the app. The cloud is asked for that artist, and
// its albums show under their group headings, the cursor on the first album.
func TestSearchBrowse_RightOpensAnArtist(t *testing.T) {
	cloud := &fakeCloud{signedIn: true, results: artistResults(), artist: bowie()}
	m := pressKey(searchFor(t, searchModel(t, cloud), "bowie"), "down")

	m = openUnderCursor(t, m)

	assert.Equal(t, []string{"artist:517"}, cloud.opened)
	s := searchOf(m)
	assert.Equal(t, []string{"← back", "Albums", "Blackstar", "Low", "Singles & EPs", "Lazarus - Single"}, resultNames(s))
	assert.Equal(t, "Blackstar", s.items[s.cursor].Name)
	view := plain(s.View().Content)
	assert.Contains(t, view, "David Bowie\n", "the artist's name is not the heading")
	assert.Contains(t, view, "── Singles & EPs ─")
	assert.Contains(t, view, "Lazarus ", "the single's ending was shown")
	assert.NotContains(t, view, "Lazarus - Single")
	assert.Contains(t, view, "2016", "the year is not the second column")
}

// Case: an artist opened, before the cloud answers. The body says it is
// opening, under the artist's name.
func TestSearchBrowse_OpeningIsSaid(t *testing.T) {
	s, cmd := pressOnArtist(t)

	require.NotNil(t, cmd)
	assert.Equal(t, "artist:517", cmd().(searchOpenMsg).result.ID.ObjectID)
	assert.Contains(t, plain(s.View().Content), "Opening David Bowie…")
}

// Case: up and down on the artist's page. The cursor passes over the group
// headings, and stops on the back row at the top.
func TestSearchBrowse_TheCursorPassesOverHeadings(t *testing.T) {
	s := openedBowie(t)

	s = pressOnSearch(s, "down", "down")
	assert.Equal(t, "Lazarus - Single", s.items[s.cursor].Name, "the cursor stopped on a heading")
	s = pressOnSearch(s, "up")
	assert.Equal(t, "Low", s.items[s.cursor].Name)
	s = pressOnSearch(s, "home")
	assert.Equal(t, "← back", s.items[s.cursor].Name)
}

// Case: ←, enter on the back row, and esc, each from the artist's page. Each
// goes back to the results as they were, the cursor on the artist.
func TestSearchBrowse_BackRestoresTheResults(t *testing.T) {
	for name, keys := range map[string][]string{
		"←":                {"left"},
		"h":                {"h"},
		"enter on back":    {"home", "enter"},
		"esc":              {"esc"},
		"filter, then esc": {"/", "l", "o", "esc", "esc"},
	} {
		s := pressOnSearch(openedBowie(t), keys...)

		assert.Empty(t, s.levels, name)
		assert.Equal(t, sonoscloud.TypeArtist, s.kind, name)
		assert.Equal(t, "David Bowie", s.items[s.cursor].Name, "%s: the cursor moved", name)
	}
}

// Case: esc on the artist's page, in the app. It goes back to the results,
// and the search stays open; a second esc closes it.
func TestSearchBrowse_EscGoesBackBeforeItCloses(t *testing.T) {
	cloud := &fakeCloud{signedIn: true, results: artistResults(), artist: bowie()}
	m := openUnderCursor(t, pressKey(searchFor(t, searchModel(t, cloud), "bowie"), "down"))

	m = pressKey(m, "esc")
	require.True(t, m.search.IsOpen(), "esc closed the search from inside the artist")
	assert.Empty(t, searchOf(m).levels)

	m = pressKey(m, "esc")
	assert.False(t, m.search.IsOpen())
}

// Case: / and some letters on the artist's page, a space among them. The
// albums narrow to those whose title holds them, the headings go, and the
// back row stays.
func TestSearchBrowse_SlashFiltersTheAlbums(t *testing.T) {
	s := pressOnSearch(openedBowie(t), "/", "b", "l", "a", "c", "k", "space")

	assert.Equal(t, []string{"← back"}, resultNames(s), "the space was not typed")
	s = pressOnSearch(openedBowie(t), "/", "l", "a")
	assert.Equal(t, []string{"← back", "Blackstar", "Lazarus - Single"}, resultNames(s))
	assert.Contains(t, plain(s.View().Content), "David Bowie · /la")
}

// Case: tab on the artist's page. It leaves the artist and switches type in
// one press.
func TestSearchBrowse_TabLeavesAndSwitches(t *testing.T) {
	s := pressOnSearch(openedBowie(t), "tab")

	assert.Empty(t, s.levels)
	assert.Equal(t, sonoscloud.TypeAlbum, s.kind)
}

// Case: the keys that act, on an album of the artist's page. They act on it
// as on a search result; on the back row they do nothing.
func TestSearchBrowse_KeysActOnAnAlbum(t *testing.T) {
	s := openedBowie(t)

	_, cmd := s.Update(keyMsgFor("n"))
	require.NotNil(t, cmd)
	msg := cmd().(searchActionMsg)
	assert.Equal(t, "Blackstar", msg.result.Name)
	assert.Equal(t, sonoscloud.TypeAlbum, msg.result.Type)
	assert.Equal(t, "album:1", msg.result.ID.ObjectID)

	_, cmd = s.Update(keyMsgFor("a"))
	require.NotNil(t, cmd)
	assert.Equal(t, "Blackstar", cmd().(searchMarkMsg).result.Name)

	s = pressOnSearch(s, "home")
	for _, key := range []string{"n", "N", "a"} {
		_, cmd := s.Update(keyMsgFor(key))
		assert.Nil(t, cmd, "%s acted on the back row", key)
	}
}

// Case: → on an album of the artist's page, and on a track in the results.
// Neither opens yet, so the right edge lights.
func TestSearchBrowse_RightOnWhatCannotOpenLightsTheEdge(t *testing.T) {
	s := pressOnSearch(openedBowie(t), "right")
	assert.Len(t, s.levels, 1, "→ on an album opened something")
	assert.Equal(t, bumpRight, s.edge)

	s = searchContent{listView: listView[sonoscloud.Result]{rows: 20, width: 80}}.found("bowie", sampleResults())
	s = pressOnSearch(s, "right")
	assert.Empty(t, s.levels)
	assert.Equal(t, bumpRight, s.edge)
}

// Case: the artist's answer arrives after the reader has gone back. There is
// no level waiting for it, so the results stay.
func TestSearchBrowse_ALateAnswerIsIgnored(t *testing.T) {
	s, cmd := pressOnArtist(t)
	gen := cmd().(searchOpenMsg).gen

	s = pressOnSearch(s, "left").opened(gen, bowie())

	assert.Empty(t, s.levels)
	assert.Equal(t, "David Bowie", s.items[s.cursor].Name)
}

// Case: the cloud fails to open the artist. The body says why, under the
// artist's name, and ← still goes back.
func TestSearchBrowse_AFailureIsSaid(t *testing.T) {
	cloud := &fakeCloud{signedIn: true, results: artistResults(), artistErr: errors.New("dial tcp: timeout")}
	m := openUnderCursor(t, pressKey(searchFor(t, searchModel(t, cloud), "bowie"), "down"))

	assert.Contains(t, plain(searchOf(m).View().Content), searchFailed)

	m = pressKey(m, "left")
	assert.Empty(t, searchOf(m).levels)
}

// openUnderCursor presses → on the app's search, then runs the opening and
// delivers its answer.
func openUnderCursor(t *testing.T, m appModel) appModel {
	t.Helper()
	model, cmd := m.Update(keyMsgFor("right"))
	m = model.(appModel)
	for _, msg := range flatten(cmd) {
		model, cmd = m.Update(msg)
		m = model.(appModel)
		for _, answer := range flatten(cmd) {
			m = deliver(m, answer)
		}
	}
	return m
}

// pressOnArtist is the search's results on the artists, with → pressed on
// the first, and the command that asks for it.
func pressOnArtist(t *testing.T) (searchContent, func() any) {
	t.Helper()
	s := searchContent{listView: listView[sonoscloud.Result]{rows: 20, width: 80}}.found("bowie", artistResults())
	s = pressOnSearch(s, "down") // onto David Bowie, the second row, so going back must find it again
	model, cmd := s.Update(keyMsgFor("right"))
	require.NotNil(t, cmd)
	return model.(searchContent), func() any { return cmd() }
}

// openedBowie is the artist's page with its albums shown.
func openedBowie(t *testing.T) searchContent {
	t.Helper()
	s, cmd := pressOnArtist(t)
	return s.opened(cmd().(searchOpenMsg).gen, bowie())
}

// artistResults has two artists, David Bowie second, and an album.
func artistResults() sonoscloud.SearchResults {
	return sonoscloud.SearchResults{Groups: []sonoscloud.ResultGroup{
		{Name: "ARTISTS", Results: []sonoscloud.Result{
			{Type: sonoscloud.TypeArtist, Name: "Ziggy Stardust", ID: appleMusicID("artist:518")},
			{Type: sonoscloud.TypeArtist, Name: "David Bowie", ID: appleMusicID("artist:517")},
		}},
		{Name: "ALBUMS", Results: []sonoscloud.Result{{Type: sonoscloud.TypeAlbum, Name: "Low", Playable: true}}},
	}}
}

// bowie is an artist with two albums and a single, as the content API gives
// them, the single among the albums.
func bowie() sonoscloud.Artist {
	return sonoscloud.Artist{Name: "David Bowie", Albums: []sonoscloud.AlbumEntry{
		{ID: appleMusicID("album:1"), Name: "Blackstar", Artist: "David Bowie", Kind: sonoscloud.KindAlbum, Summary: "Rock - 2016", Playable: true},
		{ID: appleMusicID("album:3"), Name: "Lazarus - Single", Artist: "David Bowie", Kind: sonoscloud.KindSingle, Summary: "Rock - 2015", Playable: true},
		{ID: appleMusicID("album:2"), Name: "Low", Artist: "David Bowie", Kind: sonoscloud.KindAlbum, Summary: "Rock - 1977", Playable: true},
	}}
}

package main

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
	"github.com/haasted/sonos-tui/internal/ui/panes"
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

// Case: → on a track of an album's page, and on a station in the results.
// Neither holds anything to show, so the right edge lights.
func TestSearchBrowse_RightOnWhatCannotOpenLightsTheEdge(t *testing.T) {
	s := pressOnSearch(openedBlackstar(t), "right")
	assert.Len(t, s.levels, 2, "→ on a track opened something")
	assert.Equal(t, bumpRight, s.edge)

	s = searchContent{listView: listView[sonoscloud.Result]{rows: 20, width: 80}}.found("bowie", sampleResults())
	s = pressOnSearch(s, "tab", "right")
	assert.Equal(t, sonoscloud.TypeProgram, s.kind)
	assert.Empty(t, s.levels)
	assert.Equal(t, bumpRight, s.edge)
}

// Case: → on an album of the artist's page. Its tracks show, numbered, after
// the back row, the cursor on the first; the heading names the album and says
// the list is cut short. A track's artist shows only where it is not the
// album's.
func TestSearchBrowse_RightOpensAnAlbum(t *testing.T) {
	s, cmd := pressOnSearchCmd(openedBowie(t), "right")
	require.NotNil(t, cmd)
	open := cmd().(searchOpenMsg)
	assert.Equal(t, "album:1", open.result.ID.ObjectID)

	s = s.openedAlbum(open.gen, blackstar())

	assert.Equal(t, []string{"← back", "Blackstar", "'Tis a Pity She Was a Whore", "Lazarus"}, resultNames(s))
	assert.Equal(t, "Blackstar", s.items[s.cursor].Name)
	view := plain(s.View().Content)
	assert.Contains(t, view, "Blackstar · 3 of 7\n")
	assert.Contains(t, view, " 2  'Tis a Pity She Was a Whore")
	assert.Contains(t, view, "David Bowie & Donny McCaslin", "a guest's artist line was not shown")
	assert.Equal(t, "", s.shownDetail(s.items[1]), "the album's own artist was shown")
}

// Case: enter on an album's second track, with and without a filter. The
// album is asked to play from its second track; the filter does not change
// which track that is.
func TestSearchBrowse_EnterPlaysTheAlbumFromTheTrack(t *testing.T) {
	for name, s := range map[string]searchContent{
		"plain":    pressOnSearch(openedBlackstar(t), "down"),
		"filtered": pressOnSearch(openedBlackstar(t), "/", "p", "i", "t", "y", "enter", "down"),
	} {
		_, cmd := s.Update(keyMsgFor("enter"))
		require.NotNil(t, cmd, name)
		msg := cmd().(searchActionMsg)

		assert.Equal(t, "'Tis a Pity She Was a Whore", msg.result.Name, name)
		require.NotNil(t, msg.from, name)
		assert.Equal(t, 2, msg.from.position, name)
		assert.Equal(t, "album:1", msg.from.album.ID.ObjectID, name)
	}
}

// Case: shift+enter and n on an album's track. shift+enter plays the album
// shuffled; n queues the track alone.
func TestSearchBrowse_OtherKeysOnATrack(t *testing.T) {
	s := pressOnSearch(openedBlackstar(t), "down")

	_, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	shuffled := cmd().(searchActionMsg)
	assert.Equal(t, sonoscloud.TypeAlbum, shuffled.result.Type)
	assert.Equal(t, startPlayShuffled, shuffled.action)

	_, cmd = s.Update(keyMsgFor("n"))
	next := cmd().(searchActionMsg)
	assert.Equal(t, "'Tis a Pity She Was a Whore", next.result.Name)
	assert.Nil(t, next.from)
}

// Case: enter on an album's track, in the app. The speaker gets the album and
// is sent to the track, and the search closes, as after any play.
func TestSearchBrowse_AnAlbumStartsFromTheTrackOnTheSpeaker(t *testing.T) {
	device, heard := recordingDevice(t)
	device.Speaker.USN = "uuid:RINCON_TEST01400::urn:schemas-upnp-org:device:ZonePlayer:1" // selecting the queue needs the id
	m := pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s")
	m.tabs.Items[m.tabs.Active].Content = speakerModel{ctx: t.Context(), device: device}
	album := sonoscloud.Result{ID: appleMusicID("album:1"), Type: sonoscloud.TypeAlbum, Name: "Blackstar", Playable: true}
	track := sonoscloud.Result{ID: appleMusicID("song:2"), Type: sonoscloud.TypeTrack, Name: "Lazarus", Playable: true}

	model, cmd := m.Update(searchActionMsg{result: track, action: startPlay, from: &albumStart{album: album, position: 3}})
	msgs := flatten(cmd)

	assert.False(t, model.(appModel).search.IsOpen())
	actions := strings.Join(heard(), " ")
	assert.Contains(t, actions, "#AddURIToQueue")
	assert.Contains(t, actions, "#Seek")
	assert.Contains(t, noticeTexts(msgs), "playing Blackstar from Lazarus")
}

// Case: ← from an album opened on an artist's page, then ← again. Each goes
// back one level, the cursor where it was.
func TestSearchBrowse_BackGoesUpOneLevelAtATime(t *testing.T) {
	s := pressOnSearch(openedBlackstar(t), "left")
	require.Len(t, s.levels, 1)
	assert.Equal(t, "Blackstar", s.items[s.cursor].Name)

	s = pressOnSearch(s, "left")
	assert.Empty(t, s.levels)
	assert.Equal(t, "David Bowie", s.items[s.cursor].Name)
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

// Case: an artist whose albums the API sends out of order, one with no year
// and two of the same year. Each group is sorted newest first, the album
// with no year last, and the two of one year in the order the API sent them.
func TestSearchBrowse_AlbumsAreNewestFirst(t *testing.T) {
	album := func(id, name string, kind sonoscloud.AlbumKind, summary string) sonoscloud.AlbumEntry {
		return sonoscloud.AlbumEntry{ID: appleMusicID(id), Name: name, Kind: kind, Summary: summary, Playable: true}
	}
	artist := sonoscloud.Artist{Name: "David Bowie", Albums: []sonoscloud.AlbumEntry{
		album("album:1", "Low", sonoscloud.KindAlbum, "Rock - 1977"),
		album("album:2", "Live Somewhere", sonoscloud.KindAlbum, "Rock"),
		album("album:3", "Blackstar", sonoscloud.KindAlbum, "Rock - 2016"),
		album("album:4", "Heroes", sonoscloud.KindAlbum, "Rock - 1977"),
		album("album:5", "Lazarus - Single", sonoscloud.KindSingle, "Rock - 2015"),
		album("album:6", "Blackstar - Single", sonoscloud.KindSingle, "Rock - 2016"),
	}}

	assert.Equal(t, []string{
		"← back",
		"Albums", "Blackstar", "Low", "Heroes", "Live Somewhere",
		"Singles & EPs", "Blackstar - Single", "Lazarus - Single",
	}, resultNames(searchContent{listView: listView[sonoscloud.Result]{items: artistRows(artist)}}))
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

// pressOnSearchCmd presses key on the search modal, and gives the command it
// sends.
func pressOnSearchCmd(s searchContent, key string) (searchContent, tea.Cmd) {
	model, cmd := s.Update(keyMsgFor(key))
	return model.(searchContent), cmd
}

// openedBlackstar is the album's page, opened from the artist's page, with
// its tracks shown.
func openedBlackstar(t *testing.T) searchContent {
	t.Helper()
	s, cmd := pressOnSearchCmd(openedBowie(t), "right")
	require.NotNil(t, cmd)
	return s.openedAlbum(cmd().(searchOpenMsg).gen, blackstar())
}

// noticeTexts is the text of every notice among msgs.
func noticeTexts(msgs []tea.Msg) []string {
	var texts []string
	for _, msg := range msgs {
		if notice, ok := msg.(panes.NoticeMsg); ok {
			texts = append(texts, notice.Text)
		}
	}
	return texts
}

// blackstar is the first three of an album's seven tracks, one with a guest.
func blackstar() sonoscloud.Album {
	bowie := []sonoscloud.Ref{{Name: "David Bowie"}}
	return sonoscloud.Album{ID: appleMusicID("album:1"), Name: "Blackstar", Artist: "David Bowie", Total: 7, Tracks: []sonoscloud.Track{
		{ID: appleMusicID("song:1"), Name: "Blackstar", Artists: bowie, Ordinal: 1, Playable: true},
		{ID: appleMusicID("song:2"), Name: "'Tis a Pity She Was a Whore", Ordinal: 2, Playable: true,
			Artists: []sonoscloud.Ref{{Name: "David Bowie & Donny McCaslin"}}},
		{ID: appleMusicID("song:3"), Name: "Lazarus", Artists: bowie, Ordinal: 3, Playable: true},
	}}
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

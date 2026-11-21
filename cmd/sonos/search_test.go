package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonos"
	"github.com/haasted/sonos-tui/internal/sonoscloud"
	"github.com/haasted/sonos-tui/internal/store"
	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// Case: s on the player. The search opens with the box ready for a query.
func TestSearch_SOpensWithTheBoxReady(t *testing.T) {
	m := pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s")

	require.True(t, m.search.IsOpen())
	assert.True(t, searchOf(m).filtering, "the box did not get the keys")
	assert.Contains(t, plain(searchOf(m).View().Content), searchAskText)
}

// Case: q, ? and a space typed into the box. They are text, so the app
// neither quits nor shows the help.
func TestSearch_TheBoxTakesQAndQuestionMark(t *testing.T) {
	m := typeSearch(pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s"), "q", "space", "?")

	assert.False(t, m.quitting, "q quit the app")
	assert.False(t, m.help.IsOpen(), "? opened the help")
	assert.Equal(t, "q ?", searchOf(m).query)
}

// Case: a query and enter. The cloud is searched for it, and the results show
// on the first type that has any, the empty artists passed over. The
// playlists the answer holds are not shown.
func TestSearch_EnterSearchesAndShowsTheResults(t *testing.T) {
	cloud := &fakeCloud{signedIn: true, results: sampleResults()}
	m := searchFor(t, searchModel(t, cloud), "bowie")

	assert.Equal(t, []string{"bowie"}, cloud.queries)
	s := searchOf(m)
	assert.False(t, s.filtering, "enter did not move the keys to the list")
	assert.Equal(t, sonoscloud.TypeAlbum, s.kind)
	assert.Equal(t, []string{"Low", "Heroes"}, resultNames(s))
	assert.Contains(t, plain(s.View().Content), "Artists 0   Albums 2   Tracks 0   Stations 1")
	assert.NotContains(t, plain(s.View().Content), "Playlists")
}

// Case: tab and shift+tab through the types. The empty tracks are passed
// over, and the ends light their side instead of moving.
func TestSearch_TabSwitchesTypeAndPassesOverEmptyOnes(t *testing.T) {
	s := searchContent{listView: listView[sonoscloud.Result]{rows: 20, width: 80}}.found("bowie", sampleResults())

	s = pressOnSearch(s, "tab")
	assert.Equal(t, sonoscloud.TypeProgram, s.kind, "the empty tracks were not passed over")
	s = pressOnSearch(s, "tab")
	assert.Equal(t, sonoscloud.TypeProgram, s.kind)
	assert.Equal(t, bumpRight, s.edge)
	s = pressOnSearch(s, "shift+tab")
	assert.Equal(t, sonoscloud.TypeAlbum, s.kind)
	s = pressOnSearch(s, "shift+tab")
	assert.Equal(t, sonoscloud.TypeAlbum, s.kind)
	assert.Equal(t, bumpLeft, s.edge, "the empty artists were not an end")
}

// Case: a second query asked before the first one's answer. The first answer
// arrives late and is not shown.
func TestSearch_AnOlderAnswerIsIgnored(t *testing.T) {
	m := pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s")
	m = deliver(m, searchAskMsg{query: "first"})
	first := m.searchRun
	m = deliver(m, searchAskMsg{query: "second"})

	m = deliver(m, searchDoneMsg{gen: first, query: "first", results: sampleResults()})

	s := searchOf(m)
	assert.True(t, s.running, "the older answer ended the search")
	assert.Equal(t, "second", s.asked)
	assert.Empty(t, resultNames(s))
}

// Case: each way a search can fail. The body says what went wrong.
func TestSearch_AFailureIsSaid(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"signed out":      {sonoscloud.ErrSignedOut, searchSignedOut},
		"wrong household": {sonoscloud.ErrWrongHousehold, searchNoHousehold},
		"no household":    {sonoscloud.ErrHouseholdUnknown, searchNoHousehold},
		"no speaker":      {errNoSpeaker, searchNoSpeaker},
		"anything else":   {errors.New("dial tcp: timeout"), searchFailed},
	} {
		m := searchFor(t, searchModel(t, &fakeCloud{signedIn: true, err: tc.err}), "bowie")

		assert.Contains(t, plain(searchOf(m).View().Content), tc.want, name)
	}
}

// Case: a search that finds nothing. The body says so, with the query.
func TestSearch_NothingFoundIsSaid(t *testing.T) {
	m := searchFor(t, searchModel(t, &fakeCloud{signedIn: true}), "zzzz")

	assert.Contains(t, plain(searchOf(m).View().Content), "Nothing found for “zzzz”.")
}

// Case: no saved session. s says how to sign in, and enter asks the cloud
// nothing.
func TestSearch_SignedOutSaysHowToSignIn(t *testing.T) {
	cloud := &fakeCloud{}
	m := pressKey(searchModel(t, cloud), "s")
	assert.Contains(t, plain(searchOf(m).View().Content), searchSignedOut)

	m = deliver(m, searchAskMsg{query: "bowie"})

	assert.Empty(t, cloud.queries)
	assert.Contains(t, plain(searchOf(m).View().Content), searchSignedOut)
}

// Case: no cloud client at all. s says the cloud is not there.
func TestSearch_NoCloudIsSaid(t *testing.T) {
	m := pressKey(sizedModel(t), "s")

	assert.Contains(t, plain(searchOf(m).View().Content), searchNoCloud)
}

// Case: esc after a search, then s again. The last search is back, with the
// list, not the box, taking the keys.
func TestSearch_ReopeningShowsTheLastSearch(t *testing.T) {
	m := searchFor(t, searchModel(t, &fakeCloud{signedIn: true, results: sampleResults()}), "bowie")

	m = pressKey(m, "esc")
	require.False(t, m.search.IsOpen())
	m = pressKey(m, "s")

	s := searchOf(m)
	assert.Equal(t, []string{"Low", "Heroes"}, resultNames(s))
	assert.False(t, s.filtering)
	assert.Equal(t, "bowie", s.query)
}

// Case: / after a search. The box takes the keys again, holding the query.
func TestSearch_SlashGoesBackToTheBox(t *testing.T) {
	m := searchFor(t, searchModel(t, &fakeCloud{signedIn: true, results: sampleResults()}), "bowie")

	m = typeSearch(pressKey(m, "/"), "s")

	assert.True(t, searchOf(m).filtering)
	assert.Equal(t, "bowies", searchOf(m).query)
}

// Case: esc with text in the box. The text is cleared, and the box and the
// search stay open.
func TestSearch_EscClearsTheBox(t *testing.T) {
	m := typeSearch(pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s"), "b", "o")

	m = pressKey(m, "esc")

	require.True(t, m.search.IsOpen(), "esc closed the search")
	assert.True(t, searchOf(m).filtering, "esc moved the keys from the box")
	assert.Empty(t, searchOf(m).query)
}

// Case: esc with the box empty. The search closes.
func TestSearch_EscOnAnEmptyBoxCloses(t *testing.T) {
	m := typeSearch(pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s"), "b", "esc")
	require.True(t, m.search.IsOpen())

	m = pressKey(m, "esc")

	assert.False(t, m.search.IsOpen(), "esc on an empty box did not close the search")
}

// Case: ctrl+u with text in the box. The text is cleared, and the box keeps
// the keys.
func TestSearch_CtrlUClearsTheBox(t *testing.T) {
	m := typeSearch(pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s"), "b", "o")

	m = pressKey(m, "ctrl+u")

	require.True(t, m.search.IsOpen())
	assert.True(t, searchOf(m).filtering, "ctrl+u moved the keys from the box")
	assert.Empty(t, searchOf(m).query)
}

// Case: / after a search, then esc. The query is cleared and the box keeps
// the keys, rather than the keys going back to the results.
func TestSearch_EscAfterSlashClearsTheQuery(t *testing.T) {
	m := searchFor(t, searchModel(t, &fakeCloud{signedIn: true, results: sampleResults()}), "bowie")

	m = typeSearch(m, "/", "esc")

	require.True(t, m.search.IsOpen(), "esc closed the search")
	assert.True(t, searchOf(m).filtering, "esc moved the keys to the results")
	assert.Empty(t, searchOf(m).query)
}

// Case: the startup lookup of the household, with and without a session.
// With none, the cloud is never asked.
func TestSearch_TheHouseholdIsLookedUpOnlyWithASession(t *testing.T) {
	speaker, ok := sizedModel(t).activeSpeaker()
	require.True(t, ok)

	for name, signedIn := range map[string]bool{"signed in": true, "signed out": false} {
		m, cmd := searchModel(t, &fakeCloud{signedIn: signedIn}).lookUpHousehold(speaker.device)

		assert.Equal(t, signedIn, cmd != nil, name)
		assert.Equal(t, signedIn, m.cloudAsked, name)
	}
}

// Case: the second column of each type of result.
func TestSearch_EachTypeHasItsOwnDetail(t *testing.T) {
	bowie := []sonoscloud.Ref{{Name: "David Bowie"}}
	for name, tc := range map[string]struct {
		result sonoscloud.Result
		want   string
	}{
		"artist":  {sonoscloud.Result{Type: sonoscloud.TypeArtist, Name: "David Bowie"}, ""},
		"track":   {sonoscloud.Result{Type: sonoscloud.TypeTrack, Artists: bowie, Album: &sonoscloud.Ref{Name: "Heroes"}}, "David Bowie · Heroes"},
		"album":   {sonoscloud.Result{Type: sonoscloud.TypeAlbum, Artists: bowie, Summary: "Rock - 1977"}, "David Bowie · 1977"},
		"no year": {sonoscloud.Result{Type: sonoscloud.TypeAlbum, Artists: bowie, Summary: "Rock"}, "David Bowie"},
		"station": {sonoscloud.Result{Type: sonoscloud.TypeProgram, Summary: "Bowie and similar"}, "Bowie and similar"},
	} {
		assert.Equal(t, tc.want, resultDetail(tc.result), name)
	}
}

// Case: the keys that act on the list. Each asks for the result under the
// cursor, with its own action, as in the favourites.
func TestSearch_KeysAskForTheResultUnderTheCursor(t *testing.T) {
	s := searchContent{listView: listView[sonoscloud.Result]{rows: 20, width: 80}}.found("bowie", sampleResults())
	s = pressOnSearch(s, "down")

	for key, want := range map[string]startAction{
		"enter": startPlay, "shift+enter": startPlayShuffled, "n": startPlayNext, "N": startAddToQueue,
	} {
		var press tea.Msg = keyMsgFor(key)
		if key == "shift+enter" {
			press = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
		}
		_, cmd := s.Update(press)
		require.NotNil(t, cmd, key)
		msg := cmd().(searchActionMsg)
		assert.Equal(t, "Heroes", msg.result.Name, key)
		assert.Equal(t, want, msg.action, key)
	}
}

// Case: a key on what cannot be started: a station, an artist, and a track the
// service says is not available, each played and queued. The reader is told,
// nothing is sent, and the search stays open.
func TestSearch_WhatCannotBeStartedIsSaid(t *testing.T) {
	m := pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s")
	station := sonoscloud.Result{Type: sonoscloud.TypeProgram, Playable: true}
	artist := sonoscloud.Result{Type: sonoscloud.TypeArtist}
	unplayable := sonoscloud.Result{Type: sonoscloud.TypeTrack, ID: appleMusicID("song:1")}

	for name, tc := range map[string]struct {
		result sonoscloud.Result
		action startAction
		want   string
	}{
		"station played":       {station, startPlay, searchStationNotPlayed},
		"station queued":       {station, startPlayNext, favouriteStationNotQueued},
		"artist played":        {artist, startPlayShuffled, searchArtistNotPlayed},
		"artist queued":        {artist, startAddToQueue, searchArtistNotQueued},
		"not available played": {unplayable, startPlay, searchNotAvailable},
		"not available queued": {unplayable, startPlayNext, searchNotAvailable},
	} {
		model, cmd := m.Update(searchActionMsg{result: tc.result, action: tc.action})

		require.NotNil(t, cmd, name)
		notice, ok := cmd().(panes.NoticeMsg)
		require.True(t, ok, "%s: expected a notice", name)
		assert.Equal(t, tc.want, notice.Text, name)
		assert.True(t, model.(appModel).search.IsOpen(), name)
	}
}

// Case: enter and n on a track. Enter plays it and closes the search, as in
// the favourites; n queues it and leaves the search open.
func TestSearch_EnterClosesAndNLeavesOpen(t *testing.T) {
	m := pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s")
	track := sonoscloud.Result{ID: appleMusicID("song:1774570881"), Type: sonoscloud.TypeTrack, Name: "/=/", Playable: true}

	played, cmd := m.Update(searchActionMsg{result: track, action: startPlay})
	assert.NotNil(t, cmd)
	assert.False(t, played.(appModel).search.IsOpen(), "enter left the search open")

	queued, cmd := m.Update(searchActionMsg{result: track, action: startPlayNext})
	assert.NotNil(t, cmd)
	assert.True(t, queued.(appModel).search.IsOpen(), "n closed the search")
}

// Case: shift+enter on a track. One track has nothing to shuffle, so it plays
// in order, from its start, as enter plays it.
func TestSearch_ShiftEnterPlaysATrackInOrder(t *testing.T) {
	device, heard := recordingDevice(t)
	device.Speaker.USN = "uuid:RINCON_TEST01400::urn:schemas-upnp-org:device:ZonePlayer:1" // selecting the queue needs the id
	m := pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s")
	m.tabs.Items[m.tabs.Active].Content = speakerModel{ctx: t.Context(), device: device}
	track := sonoscloud.Result{ID: appleMusicID("song:1774570881"), Type: sonoscloud.TypeTrack, Name: "/=/", Playable: true}

	_, cmd := m.Update(searchActionMsg{result: track, action: startPlayShuffled})
	flatten(cmd)

	// Only an in-order start goes to track 1; a shuffled one does not.
	assert.Contains(t, strings.Join(heard(), " "), "#Seek", "the track was shuffled")
}

// Case: N on a track found by search. The speaker is asked to add the
// track's URI to its queue.
func TestSearch_ATrackIsAddedToTheQueue(t *testing.T) {
	device, heard := recordingDevice(t)
	speaker := speakerModel{ctx: t.Context(), device: device}
	playable, err := resultPlayable(sonoscloud.Result{
		ID: appleMusicID("song:1774570881"), Type: sonoscloud.TypeTrack, Name: "/=/", Playable: true,
	})
	require.NoError(t, err)

	startPlayable(speaker, playable, startAddToQueue, false)()

	assert.Contains(t, strings.Join(heard(), " "), "#AddURIToQueue")
}

// Case: which results are drawn as not available. An artist never plays as
// such, so only a track, an album or a station the service refuses is one.
func TestSearch_UnavailableResults(t *testing.T) {
	for name, tc := range map[string]struct {
		result sonoscloud.Result
		want   bool
	}{
		"playable track":    {sonoscloud.Result{Type: sonoscloud.TypeTrack, Playable: true}, false},
		"unplayable track":  {sonoscloud.Result{Type: sonoscloud.TypeTrack}, true},
		"unplayable album":  {sonoscloud.Result{Type: sonoscloud.TypeAlbum}, true},
		"unplayable artist": {sonoscloud.Result{Type: sonoscloud.TypeArtist}, false},
	} {
		assert.Equal(t, tc.want, unavailable(tc.result), name)
	}
}

// Case: a on the list. It asks for the result under the cursor to be marked.
func TestSearch_AAsksToMarkTheResultUnderTheCursor(t *testing.T) {
	s := searchContent{listView: listView[sonoscloud.Result]{rows: 20, width: 80}}.found("bowie", sampleResults())
	s = pressOnSearch(s, "down")

	_, cmd := s.Update(keyMsgFor("a"))

	require.NotNil(t, cmd)
	assert.Equal(t, "Heroes", cmd().(searchMarkMsg).result.Name)
}

// Case: a on what cannot be marked: a station, an artist, and a track the
// service says is not available. The reader is told and nothing is stored.
func TestSearch_WhatCannotBeMarkedIsSaid(t *testing.T) {
	m := pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s")
	m.db = openTestStore(t)

	for name, tc := range map[string]struct {
		result sonoscloud.Result
		want   string
	}{
		"station":       {sonoscloud.Result{Type: sonoscloud.TypeProgram, Playable: true}, searchStationNotMarked},
		"artist":        {sonoscloud.Result{Type: sonoscloud.TypeArtist}, searchArtistNotMarked},
		"not available": {sonoscloud.Result{Type: sonoscloud.TypeTrack, ID: appleMusicID("song:1")}, searchNotAvailable},
	} {
		_, cmd := m.Update(searchMarkMsg{result: tc.result})

		require.NotNil(t, cmd, name)
		assert.Equal(t, tc.want, cmd().(panes.NoticeMsg).Text, name)
	}
	assert.Zero(t, favouriteCount(t, m.db))
}

// Case: a on a track and on an album. Each is stored with what the
// favourites need to play it, and a second a says it is a favourite already.
func TestSearch_AMarksATrackAndAnAlbum(t *testing.T) {
	m := pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s")
	m.db = openTestStore(t)

	assert.Equal(t, favouriteMarked, markResult(t, m, eggsTrack()))
	assert.Equal(t, favouriteMarked, markResult(t, m, eggsAlbum()))
	assert.Equal(t, favouriteMarkedNot, markResult(t, m, eggsTrack()))

	stored, err := m.db.Favourites().List(t.Context())
	require.NoError(t, err)
	require.Len(t, stored, 2)
	byKind := map[store.FavouriteKind]store.Favourite{}
	for _, fav := range stored {
		byKind[fav.Kind] = fav
	}
	track, album := byKind[store.FavouriteTrack], byKind[store.FavouriteAlbum]
	assert.Equal(t, "x-sonos-http:song%3a1774570881.mp4?sid=204&flags=8232&sn=2", track.URI)
	assert.Equal(t, "Andrew Bird", track.Artist)
	assert.Equal(t, "The Mysterious Production of Eggs", track.Album)
	assert.Equal(t, "album:1774570638", album.ItemID)
	assert.Equal(t, "Andrew Bird", album.Artist)
}

// Case: a track and an album marked from the queue first, then found by
// search. a on each says it is a favourite already: both ways name the same
// service, account and item.
func TestSearch_AFavouriteFromTheQueueIsTheSameOne(t *testing.T) {
	m := pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s")
	m.db = openTestStore(t)
	fromQueue, err := trackFavourite(sonos.TrackMetadata{
		URI: "x-sonos-http:song%3a1774570881.mp4?sid=204&flags=8232&sn=2", Title: "/=/", Artist: "Andrew Bird",
	}, time.Now())
	require.NoError(t, err)
	albumFromQueue, ok := sourceFavourite(sonos.QueueSource{
		Title: "The Mysterious Production of Eggs", Class: "object.container.album.musicAlbum",
		URI: "x-rincon-cpcontainer:1004206calbum%3a1774570638?sid=204&flags=8300&sn=2",
	}, time.Now())
	require.True(t, ok)
	_, err = m.db.Favourites().Add(t.Context(), fromQueue, albumFromQueue)
	require.NoError(t, err)

	assert.Equal(t, favouriteMarkedNot, markResult(t, m, eggsTrack()))
	assert.Equal(t, favouriteMarkedNot, markResult(t, m, eggsAlbum()))
	assert.Equal(t, 2, favouriteCount(t, m.db))
}

// fakeCloud stands in for the Sonos cloud. It knows one household and
// answers every search with results, or with err.
type fakeCloud struct {
	signedIn  bool
	results   sonoscloud.SearchResults
	err       error
	queries   []string // what was searched for, in order
	artist    sonoscloud.Artist
	artistErr error
	album     sonoscloud.Album
	opened    []string // the artists and albums opened, by id, in order
}

var _ musicCloud = &fakeCloud{}

func (f *fakeCloud) SignedIn() bool { return f.signedIn }

func (f *fakeCloud) Household(context.Context, string) (sonoscloud.Household, error) {
	return sonoscloud.Household{ID: "Sonos_HOUSEHOLD.PLACEHOLDER"}, nil
}

func (f *fakeCloud) Search(_ context.Context, _ string, _ int, query string) (sonoscloud.SearchResults, error) {
	f.queries = append(f.queries, query)
	return f.results, f.err
}

func (f *fakeCloud) OpenArtist(_ context.Context, _ string, _ int, objectID string) (sonoscloud.Artist, error) {
	f.opened = append(f.opened, objectID)
	return f.artist, f.artistErr
}

func (f *fakeCloud) OpenAlbum(_ context.Context, _ string, _ int, objectID string) (sonoscloud.Album, error) {
	f.opened = append(f.opened, objectID)
	return f.album, nil
}

// searchModel is the sized app with cloud as its Sonos cloud, and the
// speakers' household id read already, so no speaker is asked for it.
func searchModel(t *testing.T, cloud *fakeCloud) appModel {
	t.Helper()
	m := sizedModel(t)
	m.cloud, m.localHousehold = cloud, "Sonos_HOUSEHOLD"
	return m
}

// searchFor opens the search, types query and presses enter, then runs the
// search and delivers its answer.
func searchFor(t *testing.T, m appModel, query string) appModel {
	t.Helper()
	m = typeSearch(pressKey(m, "s"), strings.Split(query, "")...)

	model, cmd := m.Update(keyMsgFor("enter"))
	m = model.(appModel)
	for _, msg := range flatten(cmd) {
		if ask, ok := msg.(searchAskMsg); ok {
			model, cmd = m.Update(ask)
			m = model.(appModel)
			for _, done := range flatten(cmd) {
				m = deliver(m, done)
			}
			return m
		}
	}
	t.Fatal("enter asked for no search")
	return m
}

// typeSearch presses keys in order on the app.
func typeSearch(m appModel, keys ...string) appModel {
	for _, key := range keys {
		m = pressKey(m, key)
	}
	return m
}

// pressOnSearch presses keys in order on the search modal.
func pressOnSearch(s searchContent, keys ...string) searchContent {
	for _, key := range keys {
		var msg tea.Msg = keyMsgFor(key)
		if key == "shift+tab" {
			msg = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
		}
		model, _ := s.Update(msg)
		s = model.(searchContent)
	}
	return s
}

func searchOf(m appModel) searchContent {
	s, _ := m.search.Content.(searchContent)
	return s
}

func resultNames(s searchContent) []string {
	var names []string
	for _, r := range s.items {
		names = append(names, r.Name)
	}
	return names
}

// sampleResults has albums, a playlist and a station, and no artists or
// tracks, in the groups the content API sends.
func sampleResults() sonoscloud.SearchResults {
	return sonoscloud.SearchResults{Groups: []sonoscloud.ResultGroup{
		{Name: "ALBUMS", Results: []sonoscloud.Result{
			{Type: sonoscloud.TypeAlbum, Name: "Low"},
			{Type: sonoscloud.TypeAlbum, Name: "Heroes"},
		}},
		{Name: "PLAYLISTS", Results: []sonoscloud.Result{{Type: sonoscloud.TypePlaylist, Name: "Bowie Essentials"}}},
		{Name: "PROGRAMS", Results: []sonoscloud.Result{{Type: sonoscloud.TypeProgram, Name: "Bowie Radio"}}},
	}}
}

// markResult presses a on result and returns what the notice says.
func markResult(t *testing.T, m appModel, result sonoscloud.Result) string {
	t.Helper()
	_, cmd := m.Update(searchMarkMsg{result: result})
	require.NotNil(t, cmd)
	return cmd().(panes.NoticeMsg).Text
}

// eggsTrack is the first track of the album queued on Soveværelse, as the
// content API found it.
func eggsTrack() sonoscloud.Result {
	return sonoscloud.Result{
		ID: appleMusicID("song:1774570881"), Type: sonoscloud.TypeTrack, Name: "/=/", Playable: true,
		Artists: []sonoscloud.Ref{{Name: "Andrew Bird"}}, Album: &sonoscloud.Ref{Name: "The Mysterious Production of Eggs"},
	}
}

// eggsAlbum is that album, as the content API found it.
func eggsAlbum() sonoscloud.Result {
	return sonoscloud.Result{
		ID: appleMusicID("album:1774570638"), Type: sonoscloud.TypeAlbum, Name: "The Mysterious Production of Eggs",
		Playable: true, Artists: []sonoscloud.Ref{{Name: "Andrew Bird"}},
	}
}

package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
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

// Case: → and ← through the types. The empty tracks are passed over, and
// the ends light their side instead of moving.
func TestSearch_ArrowsSwitchTypeAndPassOverEmptyOnes(t *testing.T) {
	s := searchContent{listView: listView[sonoscloud.Result]{rows: 20, width: 80}}.found("bowie", sampleResults())

	s = pressOnSearch(s, "tab")
	assert.Equal(t, sonoscloud.TypeProgram, s.kind, "the empty tracks were not passed over")
	s = pressOnSearch(s, "right")
	assert.Equal(t, sonoscloud.TypeProgram, s.kind)
	assert.Equal(t, bumpRight, s.edge)
	s = pressOnSearch(s, "shift+tab")
	assert.Equal(t, sonoscloud.TypeAlbum, s.kind)
	s = pressOnSearch(s, "left")
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

// Case: n and N on the list. Each asks for the result under the cursor, with
// its own action, as in the favourites.
func TestSearch_NAndShiftNAskForTheResultUnderTheCursor(t *testing.T) {
	s := searchContent{listView: listView[sonoscloud.Result]{rows: 20, width: 80}}.found("bowie", sampleResults())
	s = pressOnSearch(s, "down")

	for key, want := range map[string]startAction{"n": startPlayNext, "N": startAddToQueue} {
		_, cmd := s.Update(keyMsgFor(key))
		require.NotNil(t, cmd, key)
		msg := cmd().(searchActionMsg)
		assert.Equal(t, "Heroes", msg.result.Name, key)
		assert.Equal(t, want, msg.action, key)
	}
}

// Case: n on what cannot be queued: a station, an artist, and a track the
// service says is not available. The reader is told, nothing is sent, and the
// search stays open.
func TestSearch_WhatCannotBeQueuedIsSaid(t *testing.T) {
	m := pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s")

	for name, tc := range map[string]struct {
		result sonoscloud.Result
		want   string
	}{
		"station":       {sonoscloud.Result{Type: sonoscloud.TypeProgram, Playable: true}, favouriteStationNotQueued},
		"artist":        {sonoscloud.Result{Type: sonoscloud.TypeArtist}, searchArtistNotQueued},
		"not available": {sonoscloud.Result{Type: sonoscloud.TypeTrack, ID: appleMusicID("song:1")}, searchNotAvailable},
	} {
		model, cmd := m.Update(searchActionMsg{result: tc.result, action: startPlayNext})

		require.NotNil(t, cmd, name)
		notice, ok := cmd().(panes.NoticeMsg)
		require.True(t, ok, "%s: expected a notice", name)
		assert.Equal(t, tc.want, notice.Text, name)
		assert.True(t, model.(appModel).search.IsOpen(), name)
	}
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

// fakeCloud stands in for the Sonos cloud. It knows one household and
// answers every search with results, or with err.
type fakeCloud struct {
	signedIn bool
	results  sonoscloud.SearchResults
	err      error
	queries  []string // what was searched for, in order
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

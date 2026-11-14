package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/haasted/sonos-tui/internal/sonos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: each kind of playback, projected onto what the widget renders. Driven
// from real captured events, since the two assumptions this feature was first
// designed on both turned out to be wrong against real data.
func TestRadio_ProjectsEachSourceKind(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		label   string
		title   string
		album   string
	}{
		{"broadcast_radio", "live", "DR P1", ""},
		{"station_no_track", "radio", "Henrik Aasted’s Station", ""},
		{"station_with_track", "", "Party", "Akylas"},
		{"queue_track", "", "It Was Us", "Dyade"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			state := stateFromFixture(t, tc.fixture)
			track := trackFromState(state)

			assert.Equal(t, tc.label, track.StreamLabel)
			assert.Equal(t, tc.title, track.Title)
			assert.Equal(t, tc.album, track.Album)
		})
	}
}

// Case: a station playing a real track, which must keep its clock. Being a
// stream and having a usable duration are separate questions, and answering
// the second with the first would throw away an accurate readout.
func TestRadio_StationWithARealTrackKeepsItsClock(t *testing.T) {
	track := trackFromState(stateFromFixture(t, "station_with_track"))

	assert.Empty(t, track.StreamLabel, "a track with a duration must not be labelled")
	assert.NotZero(t, track.Duration)
	assert.Equal(t, "Akylas", track.Album, "the station outranks the track's own album")
}

// Case: broadcast radio, whose track metadata carries no title at all — the
// name lives only on the source, so without the fallback the row is blank.
func TestRadio_BroadcastTitleFallsBackToTheStation(t *testing.T) {
	state := stateFromFixture(t, "broadcast_radio")
	require.Empty(t, state.Current.Title, "the fixture must actually lack a track title")

	assert.Equal(t, "DR P1", trackFromState(state).Title)
}

// Case: broadcast radio, where the programme description is the only thing
// the station says about what is on. It takes the row below the station name,
// which for a broadcast is free — there is no artist.
func TestRadio_BroadcastShowsTheProgramme(t *testing.T) {
	track := trackFromState(stateFromFixture(t, "broadcast_radio"))

	assert.Contains(t, track.Artist, "P1 Morgen")
	assert.Equal(t, "DR P1", track.Title, "the station still names the row above it")
}

// Case: a track that has a real artist, whose row must not be taken by a
// programme description that happens to be present.
func TestRadio_ARealArtistOutranksTheProgramme(t *testing.T) {
	state := stateFromFixture(t, "station_with_track")
	state.Current.RadioShow = "some programme blurb"

	assert.Equal(t, "Marseaux & Billie Kark", trackFromState(state).Artist)
}

// Case: a speaker whose state has not loaded yet. Everything is zero then,
// including the duration, so a naive duration check would label every cached
// tab a stream before its first refresh.
func TestRadio_UnloadedSpeakerIsNotAStream(t *testing.T) {
	assert.Empty(t, streamLabel(sonos.DeviceState{}))
	assert.False(t, sonos.DeviceState{}.Live())
}

// Case: the label for each source kind, including ones no capture covers yet.
func TestRadio_LabelsEverySourceKind(t *testing.T) {
	for uri, want := range map[string]string{
		"x-sonosapi-hls:channel":   "live",
		"x-sonosapi-radio:station": "radio",
		"x-rincon-stream:RINCON_1": "line-in",
		"something-new:whatever":   "stream",
	} {
		state := sonos.DeviceState{Source: sonos.SourceInfo{URI: uri}}
		assert.Equal(t, want, streamLabel(state), "uri %q", uri)
	}
}

func stateFromFixture(t *testing.T, name string) sonos.DeviceState {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("..", "..", "internal", "sonos", "testdata",
		"avtransport_"+name+".xml"))
	require.NoError(t, err)

	event, err := sonos.ParseAVTransportEvent(body)
	require.NoError(t, err)

	return sonos.DeviceState{
		Transport: sonos.TransportInfo{State: event.TransportState},
		Current:   event.Current,
		Source:    event.Source,
		RelTime:   "0:01:04",
	}
}

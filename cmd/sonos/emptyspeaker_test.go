package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/haasted/sonos-tui/internal/sonos"
)

// Case: a speaker read with no track loaded. The panel says nothing is
// queued and names the keys that load something, with the volume kept.
func TestEmptySpeaker_SaysNothingIsQueued(t *testing.T) {
	track := trackFromState(sonos.DeviceState{Transport: sonos.TransportInfo{State: "STOPPED"}, TrackKnown: true,
		Source: sonos.SourceInfo{URI: "x-rincon-queue:RINCON_X#0"}, Volume: 12})

	assert.True(t, track.Empty)
	assert.Equal(t, emptyTitle, track.Title)
	assert.Equal(t, emptyHint, track.Artist)
	assert.Equal(t, 12, track.Volume)
}

// Case: a speaker not read yet, one stopped or paused with a track loaded,
// a station between tracks, and a queue with tracks but none loaded. None
// is empty: the first stays blank until it answers.
func TestEmptySpeaker_OnlyAReadSpeakerWithNoTrackIsEmpty(t *testing.T) {
	loaded := sonos.TrackMetadata{URI: "x-sonos-http:song%3a1.mp4", Title: "Heroes"}
	for name, state := range map[string]sonos.DeviceState{
		"not read": {},
		"stopped":  {Transport: sonos.TransportInfo{State: "STOPPED"}, Current: loaded, TrackKnown: true},
		"paused":   {Transport: sonos.TransportInfo{State: "PAUSED_PLAYBACK"}, Current: loaded, TrackKnown: true},
		"station":  {TrackKnown: true, Source: sonos.SourceInfo{URI: "x-sonosapi-radio:station"}},
		"queued":   {TrackKnown: true, Source: sonos.SourceInfo{URI: "x-rincon-queue:RINCON_X#0"}, Tracks: 3},
	} {
		track := trackFromState(state)
		assert.False(t, track.Empty, name)
		assert.NotEqual(t, emptyTitle, track.Title, name)
	}
}

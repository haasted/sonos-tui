package main

// TODO: TEMPORARY. Remove with trial.go.

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: each trial key, which must name its playlist and set the mode every
// time, so a plain key plays in order even after a shuffled one.
func TestTrialChoice_EachKeyNamesAPlaylistAndAMode(t *testing.T) {
	cases := map[string]struct {
		title string
		mode  sonos.PlayMode
	}{
		"z": {"Awesome singles", sonos.PlayModeNormal},
		"Z": {"Awesome singles", sonos.PlayModeShuffleNoRepeat},
		"c": {"Metallica", sonos.PlayModeNormal},
		"C": {"Metallica", sonos.PlayModeShuffleNoRepeat},
	}
	for key, want := range cases {
		playlist, mode, ok := trialChoice(key)
		assert.True(t, ok, key)
		assert.Equal(t, want.title, playlist.Title, key)
		assert.Equal(t, want.mode, mode, key)
	}

	_, _, ok := trialChoice("x")
	assert.False(t, ok, "a key that is not a trial key must be left to the player")
}

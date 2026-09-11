package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: the reported one. A personalised station refuses to go back, which the
// Sonos app shows by greying the button out. The speaker is right to refuse.
func TestRefusal_AStationHasNoPreviousTrack(t *testing.T) {
	state := sonos.DeviceState{
		Source: sonos.SourceInfo{URI: "x-sonosapi-radio:ST%3aetc"},
		Track:  1,
		Tracks: 1,
	}

	got, ok := plainRefusal(actionPrevious, transitionError(), state)
	if !ok {
		t.Fatal("a station refusing previous was not recognised")
	}
	if got == "" || containsUPnP(got) {
		t.Fatalf("the message still speaks UPnP: %q", got)
	}
}

// Case: the two ends of a queue, which refuse in the same way as a station and
// need a different sentence.
func TestRefusal_TheEndsOfAQueue(t *testing.T) {
	queue := func(track, tracks int) sonos.DeviceState {
		return sonos.DeviceState{
			Source: sonos.SourceInfo{URI: "x-rincon-queue:RINCON_1234#0"},
			Track:  track,
			Tracks: tracks,
		}
	}

	if _, ok := plainRefusal(actionPrevious, transitionError(), queue(1, 20)); !ok {
		t.Error("previous at the first track was not recognised")
	}
	if _, ok := plainRefusal(actionNext, transitionError(), queue(20, 20)); !ok {
		t.Error("next at the last track was not recognised")
	}
}

// Case: a refusal in the middle of a queue, which nothing explains. A guess
// here would hide a real fault behind a friendly sentence.
func TestRefusal_KeepsTheSpeakersWordsWhenItCannotExplain(t *testing.T) {
	middle := sonos.DeviceState{
		Source: sonos.SourceInfo{URI: "x-rincon-queue:RINCON_1234#0"},
		Track:  7,
		Tracks: 20,
	}

	if got, ok := plainRefusal(actionPrevious, transitionError(), middle); ok {
		t.Fatalf("a refusal in mid-queue was explained away as %q", got)
	}
}

// Case: every other failure. A speaker that cannot be reached is a fault, and
// must not be dressed up as a polite refusal.
func TestRefusal_OnlyTranslatesTheRefusalCode(t *testing.T) {
	station := sonos.DeviceState{Source: sonos.SourceInfo{URI: "x-sonosapi-radio:x"}}

	for _, err := range []error{
		errors.New("dial tcp 192.168.1.10:1400: connect: no route to host"),
		fmt.Errorf("previous track: %w", &sonos.UPnPError{Code: 402, Description: "invalid args"}),
	} {
		if got, ok := plainRefusal(actionPrevious, err, station); ok {
			t.Errorf("%v was translated to %q", err, got)
		}
	}
}

// Case: how a refusal reaches the rule, against how a fault does. A refusal is
// the answer to a key just pressed on the tab in front of the reader. A fault
// can arrive from a room nobody is watching, so it must name its room.
func TestRefusal_ArrivesUnnamedAndAFaultNamesItsRoom(t *testing.T) {
	m := newTestSpeaker()
	station := sonos.DeviceState{Source: sonos.SourceInfo{URI: "x-sonosapi-radio:x"}}

	refusal, ok := m.refused(actionPrevious, transitionError(), station).(panes.NoticeMsg)
	if !ok || refusal.Level != panes.NoticeInfo {
		t.Fatalf("a refusal arrived at level %v, want info", refusal.Level)
	}
	if strings.Contains(refusal.Text, m.device.DisplayName()) {
		t.Fatalf("a refusal named its room: %q", refusal.Text)
	}

	fault, ok := m.refused(actionPrevious, errors.New("no route to host"), station).(panes.NoticeMsg)
	if !ok || fault.Level != panes.NoticeError {
		t.Fatalf("a fault arrived at level %v, want error", fault.Level)
	}
	if !strings.Contains(fault.Text, m.device.DisplayName()) {
		t.Fatalf("a fault did not name its room: %q", fault.Text)
	}
}

// Case: the width of each message. The rule is the width of the tab row, and a
// message that does not fit is cut with an ellipsis.
func TestRefusal_MessagesFitTheRule(t *testing.T) {
	const budget = 30 // the rule on a narrow terminal, less the glyph and its spaces

	for _, c := range []struct {
		name  string
		state sonos.DeviceState
	}{
		{actionPrevious, sonos.DeviceState{Source: sonos.SourceInfo{URI: "x-sonosapi-radio:x"}}},
		{actionPrevious, sonos.DeviceState{Source: sonos.SourceInfo{URI: "x-rincon-queue:q"}, Track: 1, Tracks: 9}},
		{actionNext, sonos.DeviceState{Source: sonos.SourceInfo{URI: "x-rincon-queue:q"}, Track: 9, Tracks: 9}},
	} {
		got, ok := plainRefusal(c.name, transitionError(), c.state)
		if !ok {
			t.Fatalf("%s was not recognised", c.name)
		}
		if len(got) > budget {
			t.Errorf("%q is %d characters, over the %d that fit", got, len(got), budget)
		}
	}
}

func transitionError() error {
	return fmt.Errorf("previous track: %w", &sonos.UPnPError{
		Code:        sonos.UPnPTransitionNotAvailable,
		Description: "transition not available",
	})
}

// containsUPnP reports whether the speaker's own words survived into a message
// meant for a person.
func containsUPnP(s string) bool {
	for _, word := range []string{"UPnP", "701", "transition not available"} {
		if strings.Contains(s, word) {
			return true
		}
	}
	return false
}

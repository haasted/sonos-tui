package main

import (
	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// speakerMsg addresses a message to one speaker's tab. Command results carry
// no speaker of their own, and every command is a round trip, so without an
// address they reach whichever tab is active by the time they come back.
type speakerMsg struct {
	device *sonos.Device
	msg    tea.Msg
}

type speakerFoundMsg struct {
	speaker sonos.Speaker
}

type speakerReadyMsg struct {
	title   string
	ip      string
	speaker speakerModel
	// degraded describes what failed while loading, empty when all of it
	// worked. A speaker can appear with stale state or no live updates.
	degraded string
}

// speakerVerifiedMsg reports a cached speaker that answered: its state is
// already refreshed on the shared Device, so the tab just needs redrawing.
type speakerVerifiedMsg struct {
	ip       string
	title    string
	speaker  *sonos.Device
	degraded string // as speakerReadyMsg: answered, but not everything worked
}

// speakerGoneMsg reports a cached speaker that did not answer, or that turned
// out to be a different device at the same address.
type speakerGoneMsg struct {
	ip string
}

type discoveryDoneMsg struct {
	found int
}

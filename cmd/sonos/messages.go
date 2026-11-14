package main

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-tui/internal/sonos"
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

// speakerReadyMsg carries a speaker loaded from a discovery answer: a new
// one, or a cached one that answered from another address.
type speakerReadyMsg struct {
	title   string
	id      string // the RINCON id, which keys the tab
	speaker speakerModel
	// degraded describes what failed while loading, empty when all of it
	// worked. A speaker can appear with stale state or no live updates.
	degraded string
}

// speakerVerifiedMsg reports a cached speaker that answered: its state is
// already refreshed on the shared Device, so the tab just needs redrawing.
type speakerVerifiedMsg struct {
	title    string
	speaker  *sonos.Device
	degraded string // as speakerReadyMsg: answered, but not everything worked
}

// cachedCheckedMsg reports whether a cached speaker answered at its cached
// address. A failure is not yet a removal: a discovery answer may still find
// it, perhaps somewhere else.
type cachedCheckedMsg struct {
	id  string
	err error // why it did not answer, or that the address is another speaker's
}

// discoveryDoneMsg ends discovery, saying what it found and how it went, so
// a run that found nothing can say why it might have.
type discoveryDoneMsg struct {
	found  int
	report sonos.DiscoveryReport
	err    error         // why discovery could not run at all
	took   time.Duration // how long it searched
}

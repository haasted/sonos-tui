package main

import "github.com/haasted/sonos-sandbox/internal/sonos"

type speakerFoundMsg struct {
	speaker sonos.Speaker
}

type speakerReadyMsg struct {
	title   string
	ip      string
	speaker speakerModel
}

// speakerVerifiedMsg reports a cached speaker that answered: its state is
// already refreshed on the shared Device, so the tab just needs redrawing.
type speakerVerifiedMsg struct {
	ip      string
	title   string
	speaker *sonos.Device
}

// speakerGoneMsg reports a cached speaker that did not answer, or that turned
// out to be a different device at the same address.
type speakerGoneMsg struct {
	ip string
}

type discoveryDoneMsg struct {
	found int
}

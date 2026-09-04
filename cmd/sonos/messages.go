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

type discoveryDoneMsg struct {
	found int
}

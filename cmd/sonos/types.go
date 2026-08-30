package main

import "github.com/haasted/sonos-sandbox/internal/sonos"

type speakerFoundMsg struct {
	speaker sonos.Speaker
}

type roomReadyMsg struct {
	title string
	room  roomModel
}

type discoveryDoneMsg struct {
	found int
}

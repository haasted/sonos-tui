package main

import (
	"errors"
	"net"
	"net/url"

	"github.com/haasted/sonos-tui/internal/sonos"
)

// Translation: what a speaker said, turned into what a person reads. A
// failure this cannot name keeps the speaker's own words.

// ourFault are the codes meaning this program sent something wrong. Left
// untranslated: a friendly sentence over our own bug hides it.
var ourFault = map[int]bool{401: true, 402: true, 600: true}

// refusalText turns a refusal into a sentence a person would write, or
// reports false. Unnamed on purpose: it answers a key just pressed on the
// tab in view, where a fault can come from a room nobody is watching.
func refusalText(action string, err error, state sonos.DeviceState) (string, bool) {
	var upnp *sonos.UPnPError
	if !errors.As(err, &upnp) || ourFault[upnp.Code] {
		return "", false
	}
	if upnp.Code != sonos.UPnPTransitionNotAvailable {
		return "", false
	}

	kind := state.Source.Kind()
	forwardOnly := kind == sonos.SourceStation || kind == sonos.SourceBroadcast

	switch {
	case action == actionPrevious && forwardOnly:
		return "a station plays forward only", true
	// Narrower than the case above: a station does allow next, and only a
	// broadcast has nothing ahead of it.
	case action == actionNext && kind == sonos.SourceBroadcast:
		return "live radio has no next track", true
	case action == actionPrevious && state.Track <= 1:
		return "first track in the queue", true
	case action == actionNext && state.Tracks > 0 && state.Track >= state.Tracks:
		return "last track in the queue", true
	}
	return "", false
}

// faultText is what an error notice shows: a plain sentence where the failure
// is one we can name, and otherwise the error exactly as it arrived.
func faultText(err error) string {
	if unreachable(err) {
		return "not responding"
	}
	return err.Error()
}

// unreachable reports a speaker that never answered, as against one that
// answered by refusing.
func unreachable(err error) bool {
	if _, ok := errors.AsType[net.Error](err); ok {
		return true
	}

	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

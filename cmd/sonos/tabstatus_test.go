package main

import (
	"context"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: a speaker that starts playing, whose tab glyph must follow without any
// code path having to remember to update it.
func TestPlayingStateReachesTheTab(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")
	if m.tabs.Items[0].Playing {
		t.Fatal("a speaker with no known state must start out idle")
	}

	startPlaying(t, m, "192.168.1.10")

	m = pump(m)
	if !m.tabs.Items[0].Playing {
		t.Fatal("tab did not pick up that its speaker started playing")
	}
	if m.tabs.Items[1].Playing {
		t.Fatal("one speaker playing must not mark the others as playing")
	}
}

// Case: a message that no branch of Update handles, which must still refresh
// the tabs rather than leaving the glyphs behind.
func TestUnhandledMessageStillSyncsTheTabs(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")
	startPlaying(t, m, "192.168.1.12")

	m = pump(m)
	if !m.tabs.Items[2].Playing {
		t.Fatal("syncing must not depend on which branch of Update ran")
	}
}

// startPlaying drives a device into PLAYING via an event carrying its own
// position, so nothing reaches for the network to fill one in.
func startPlaying(t *testing.T, m appModel, ip string) {
	t.Helper()

	device, ok := m.deviceFor(ip)
	if !ok {
		t.Fatalf("no device for %s", ip)
	}
	body := []byte(`<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0"><e:property><LastChange>` +
		`&lt;Event xmlns="urn:schemas-upnp-org:metadata-1-0/AVT/"&gt;&lt;InstanceID val="0"&gt;` +
		`&lt;TransportState val="PLAYING"/&gt;&lt;RelativeTimePosition val="0:00:12"/&gt;` +
		`&lt;/InstanceID&gt;&lt;/Event&gt;</LastChange></e:property></e:propertyset>`)
	if err := device.HandleAVTransportEvent(context.Background(), body); err != nil {
		t.Fatalf("handling event: %v", err)
	}
}

// pump sends a message Update has no branch for, returning the model it
// produced.
func pump(m appModel) appModel {
	model, _ := m.Update(struct{ sonos.Speaker }{})
	return model.(appModel)
}

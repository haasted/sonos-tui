package main

import (
	"context"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: startup with a cache, where tabs must exist on the very first frame
// rather than after any network round trip.
func TestCachedSpeakersAppearImmediately(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")

	if got := len(m.tabs.Items); got != 3 {
		t.Fatalf("expected 3 tabs from the cache before any I/O, got %d", got)
	}
	if title := m.tabs.Items[0].Title; title != "Kitchen" {
		t.Fatalf("cached tab should already show its room name, got %q", title)
	}
}

// Case: the speaker selected when the program last exited, which must be
// re-selected rather than defaulting to the first tab.
func TestCachedSelectionIsRestored(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "uuid:C")

	if got := m.tabs.Items[m.tabs.Active].Title; got != "Office" {
		t.Fatalf("expected the previously selected speaker, got %q", got)
	}
}

// Case: an unknown selection, e.g. that speaker is gone from the cache.
func TestUnknownSelectionFallsBackToTheFirstTab(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "uuid:NOPE")

	if m.tabs.Active != 0 {
		t.Fatalf("an unknown selection should leave the first tab active, got index %d", m.tabs.Active)
	}
}

// Case: a cached speaker that failed verification, whose tab must go.
func TestFailedVerificationRemovesTheTab(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")

	updated, _ := m.Update(speakerGoneMsg{ip: "192.168.1.11"})
	m = updated.(appModel)

	if got := len(m.tabs.Items); got != 2 {
		t.Fatalf("expected the unreachable speaker's tab to be dropped, got %d tabs", got)
	}
	for _, tab := range m.tabs.Items {
		if tab.Title == "Bedroom" {
			t.Fatal("the unreachable speaker is still on screen")
		}
	}
	if m.hasSpeaker("192.168.1.11") {
		t.Fatal("the removed speaker is still in the IP index")
	}
}

// Case: discovery finding a speaker the cache already produced, which must not
// create a second tab for it.
func TestDiscoveryDoesNotDuplicateACachedSpeaker(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")

	updated, cmd := m.Update(speakerFoundMsg{speaker: cachedSpeakers()[0]})
	m = updated.(appModel)

	if cmd != nil {
		t.Fatal("a speaker already on screen should not be loaded again")
	}
	if got := len(m.tabs.Items); got != 3 {
		t.Fatalf("expected no new tab, got %d", got)
	}
}

// Case: discovery finding a speaker absent from the cache, which must be added.
func TestDiscoveryAddsANewSpeaker(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")

	_, cmd := m.Update(speakerFoundMsg{speaker: sonos.NewSpeaker(
		"192.168.1.99", "http://192.168.1.99:1400/xml/d.xml", "uuid:NEW",
		sonos.DeviceDescription{RoomName: "Garage"})})

	if cmd == nil {
		t.Fatal("a speaker missing from the cache should be loaded")
	}
}

func cachedSpeakers() []sonos.Speaker {
	return []sonos.Speaker{
		sonos.NewSpeaker("192.168.1.10", "http://192.168.1.10:1400/xml/d.xml", "uuid:A",
			sonos.DeviceDescription{RoomName: "Kitchen"}),
		sonos.NewSpeaker("192.168.1.11", "http://192.168.1.11:1400/xml/d.xml", "uuid:B",
			sonos.DeviceDescription{RoomName: "Bedroom"}),
		sonos.NewSpeaker("192.168.1.12", "http://192.168.1.12:1400/xml/d.xml", "uuid:C",
			sonos.DeviceDescription{RoomName: "Office"}),
	}
}

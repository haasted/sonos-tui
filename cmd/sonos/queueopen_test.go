package main

import (
	"context"
	"testing"
)

// Case: opening a queue already known when the modal opens -- the common
// case, once the startup read has landed and events have kept it current.
// The cursor lands on the playing track at once, with nothing to wait for.
func TestQueueOpen_LandsOnThePlayingTrackWhenAlreadyKnown(t *testing.T) {
	m, device, _ := withQueueServer(t, 20)

	body := []byte(`<?xml version="1.0"?><e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0">` +
		`<e:property><LastChange>` +
		`&lt;Event xmlns="urn:schemas-upnp-org:metadata-1-0/AVT/"&gt;&lt;InstanceID val="0"&gt;` +
		`&lt;TransportState val="PLAYING"/&gt;&lt;RelativeTimePosition val="0:00:12"/&gt;` +
		`&lt;CurrentTrack val="10"/&gt;&lt;AVTransportURI val="x-rincon-queue:RINCON_TEST#0"/&gt;` +
		`&lt;/InstanceID&gt;&lt;/Event&gt;</LastChange></e:property></e:propertyset>`)
	if err := device.HandleAVTransportEvent(context.Background(), body); err != nil {
		t.Fatalf("setting up the playing track: %v", err)
	}

	model, cmd := m.Update(keyMsgFor("enter"))
	m = model.(appModel)
	content := queueOf(t, m)

	if content.cursor != 9 {
		t.Fatalf("opening landed on row %d, want row 9 for the track at position 10", content.cursor)
	}
	if want := content.land().offset; content.offset != want {
		t.Fatalf("opening put the window at %d, want %d to leave room for what comes next", content.offset, want)
	}
	if fetches(cmd) {
		t.Fatal("the queue was already known; opening should not have asked to read it again")
	}
}

// Case: opening before the startup read has landed. The cursor starts at the
// top with nothing to show, and once the read arrives it should land on the
// playing track -- the same answer as the already-known case, just delayed
// by the read it had to wait for.
func TestQueueOpen_LandsOnThePlayingTrackOnceTheFirstReadArrives(t *testing.T) {
	m, _ := deliverQueueWithPlaying(t, 20, 10)

	content := queueOf(t, m)
	if content.cursor != 9 {
		t.Fatalf("the first load landed on row %d, want row 9 for the track at position 10", content.cursor)
	}
}

// Case: a later reload of a queue already on screen -- an unrelated
// ContentDirectory event, say. This must not repeat the jump: the reader may
// have moved on since opening, and a second unasked-for jump would undo that.
func TestQueueOpen_ALaterReloadDoesNotRepeatTheJump(t *testing.T) {
	m, _ := deliverQueueWithPlaying(t, 20, 10)
	speaker, _ := m.activeSpeaker()

	model, _ := m.Update(keyMsgFor("end")) // the reader moves away on their own
	m = model.(appModel)
	if queueOf(t, m).cursor != 19 {
		t.Fatal("the reader's own move did not land where expected, so this proves nothing")
	}

	model, _ = m.Update(queueLoadedMsg{
		device: speaker.device, room: speaker.device.RoomName(),
		items: queueItems(20), playing: 10,
	})
	m = model.(appModel)

	if got := queueOf(t, m).cursor; got != 19 {
		t.Fatalf("a later reload moved the cursor to row %d; it should have stayed on row 19", got)
	}
}

// Case: a station playing, or nothing playing at all. There is no track for
// the cursor to land on, so opening must leave it at the top rather than
// erroring or landing somewhere arbitrary -- the same answer c already gives.
func TestQueueOpen_StaysAtTheTopWithNothingPlaying(t *testing.T) {
	m, _ := deliverQueueWithPlaying(t, 20, 0)

	if got := queueOf(t, m).cursor; got != 0 {
		t.Fatalf("opening with nothing playing landed on row %d, want row 0", got)
	}
}

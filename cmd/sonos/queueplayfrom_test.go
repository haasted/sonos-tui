package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: the key, which must ask for the track the cursor is on rather than
// the row it sits at. A queue scrolled down has the two far apart.
func TestQueuePlayFrom_AsksForTheTrackUnderTheCursor(t *testing.T) {
	q := navQueue(60, 42)

	_, cmd := q.Update(keyMsgFor("enter"))
	if cmd == nil {
		t.Fatal("enter asked for nothing")
	}

	msg, ok := cmd().(queuePlayFromMsg)
	if !ok {
		t.Fatalf("enter produced %T", cmd())
	}
	if msg.position != 43 {
		t.Fatalf("enter asked for position %d, want 43 for row 42", msg.position)
	}
}

// Case: an empty queue, and one still being read. There is no track under the
// cursor, so there is nothing to play.
func TestQueuePlayFrom_AsksForNothingWithNoTrackUnderTheCursor(t *testing.T) {
	for name, q := range map[string]queueContent{
		"empty":   {room: "Køkken", listView: listView[sonos.QueueItem]{rows: 12, width: 70}},
		"loading": {room: "Køkken", listView: listView[sonos.QueueItem]{rows: 12, width: 70}, loading: true},
	} {
		if _, cmd := q.Update(keyMsgFor("enter")); cmd != nil {
			t.Fatalf("a %s queue asked to play a track", name)
		}
	}
}

// Case: the cursor already on the track playing. Restarting it is a real
// answer to enter, not a state playFromCursor should refuse.
func TestQueuePlayFrom_ThePlayingTrackIsAskedForToo(t *testing.T) {
	q := navQueue(20, 9) // cursor on Track 10, position 10
	q.playing = 10

	_, cmd := q.Update(keyMsgFor("enter"))
	msg, ok := cmd().(queuePlayFromMsg)
	if !ok {
		t.Fatalf("enter produced %T", cmd())
	}
	if msg.position != 10 {
		t.Fatalf("enter asked for position %d, want 10", msg.position)
	}
}

// Case: the whole path, from the key to the speaker. The modal closes on the
// way, since the reader asked to listen to something -- not to keep browsing.
func TestQueuePlayFrom_ClosesTheModalAndConfirms(t *testing.T) {
	m, device, _ := withQueueServer(t, 20)

	model, _ := m.Update(keyMsgFor("enter")) // open the modal
	m = model.(appModel)
	if !m.queue.IsOpen() {
		t.Fatal("the modal never opened, so this proves nothing")
	}

	updated, cmd := m.Update(queuePlayFromMsg{position: 3})
	m = updated.(appModel)
	if cmd == nil {
		t.Fatal("nothing was sent to the speaker")
	}
	if m.queue.IsOpen() {
		t.Fatal("the modal is still open after playing from the queue")
	}

	notice, ok := cmd().(panes.NoticeMsg)
	if !ok {
		t.Fatalf("playing from the queue produced %T", cmd())
	}
	if !notice.Confirmation {
		t.Fatal("playing from the queue must answer as a confirmation, which yields the rule")
	}
	if notice.Level != panes.NoticeInfo {
		t.Fatalf("playing a track that worked reported level %v", notice.Level)
	}
	_ = device
}

var _ tea.Msg = queuePlayFromMsg{}

package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: the key, which must ask for the track the cursor is on rather than the
// row it sits at. A queue scrolled down has the two far apart.
func TestQueueRemove_AsksForTheTrackUnderTheCursor(t *testing.T) {
	for _, key := range []string{"x", "delete"} {
		q := navQueue(60, 42)

		_, cmd := q.Update(keyMsgFor(key))
		if cmd == nil {
			t.Fatalf("%q asked for nothing", key)
		}

		msg, ok := cmd().(queueRemoveMsg)
		if !ok {
			t.Fatalf("%q produced %T", key, cmd())
		}
		if msg.position != 43 {
			t.Fatalf("%q asked for position %d, want 43 for row 42", key, msg.position)
		}
	}
}

// Case: the list itself, which must not shorten. The removal raises an event
// that brings the new queue in, and a second answer here would disagree with
// that one while it was in flight.
func TestQueueRemove_TheListIsNotEditedHere(t *testing.T) {
	q := navQueue(60, 10)

	updated, _ := q.Update(keyMsgFor("x"))
	if got := len(updated.(queueContent).items); got != 60 {
		t.Fatalf("the list now holds %d items; it must wait for the speaker", got)
	}
}

// Case: an empty queue, and one still being read. There is no track under the
// cursor, so there is nothing to ask for.
func TestQueueRemove_AsksForNothingWithNoTrackUnderTheCursor(t *testing.T) {
	for name, q := range map[string]queueContent{
		"empty":   {room: "Køkken", rows: 12, width: 70},
		"loading": {room: "Køkken", rows: 12, width: 70, loading: true},
	} {
		if _, cmd := q.Update(keyMsgFor("x")); cmd != nil {
			t.Fatalf("a %s queue asked for a track to be removed", name)
		}
	}
}

// Case: the whole path, from the key to the speaker. The speaker here answers
// every command, so a confirmation is what should come back.
func TestQueueRemove_ConfirmsWhatHappened(t *testing.T) {
	m, device, _ := withQueueServer(t, 20)

	model, _ := m.Update(keyMsgFor("enter"))
	m = model.(appModel)

	_, cmd := m.Update(queueRemoveMsg{position: 3})
	if cmd == nil {
		t.Fatal("nothing was sent to the speaker")
	}

	notice, ok := cmd().(panes.NoticeMsg)
	if !ok {
		t.Fatalf("the removal produced %T", cmd())
	}
	if !notice.Confirmation {
		t.Fatal("a removal must answer as a confirmation, which yields the rule")
	}
	if notice.Level != panes.NoticeInfo {
		t.Fatalf("a removal that worked reported level %v", notice.Level)
	}
	_ = device
}

var _ tea.Msg = queueRemoveMsg{}

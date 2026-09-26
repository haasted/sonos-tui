package main

import (
	"slices"
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
		if !slices.Equal(msg.positions, []int{43}) {
			t.Fatalf("%q asked for positions %v, want just 43 for row 42", key, msg.positions)
		}
	}
}

// Case: a selection, which the key must act on instead of the cursor's track.
// The positions are the picked tracks, whether or not the cursor is on one.
func TestQueueRemove_AsksForTheWholeSelection(t *testing.T) {
	q := navQueue(20, 0)
	q = moveQueue(moveQueue(moveQueue(q, "s"), "s"), "s") // rows 0, 1 and 2
	q = moveQueue(q, "end")                               // the cursor leaves them

	_, cmd := q.Update(keyMsgFor("x"))
	msg, ok := cmd().(queueRemoveMsg)
	if !ok {
		t.Fatalf("x produced %T", cmd())
	}
	if !slices.Equal(msg.positions, []int{1, 2, 3}) {
		t.Fatalf("x asked for %v, want the three picked tracks and not the cursor's", msg.positions)
	}
}

// Case: what the selection is afterwards. The tracks are about to stop
// existing, so the marks go with them rather than being left to puzzle over.
func TestQueueRemove_DropsTheSelection(t *testing.T) {
	q := moveQueue(moveQueue(navQueue(20, 0), "s"), "s")

	updated, _ := q.Update(keyMsgFor("x"))
	if got := updated.(queueContent).selectionCount(); got != 0 {
		t.Fatalf("%d tracks are still picked after removing them", got)
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

	_, cmd := m.Update(queueRemoveMsg{positions: []int{3}})
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

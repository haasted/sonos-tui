package main

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: the key, which must take the track the cursor is on rather than the
// row it sits at. A queue scrolled down has the two far apart.
func TestQueueRemove_TakesTheTrackUnderTheCursor(t *testing.T) {
	for _, key := range []string{"x", "delete"} {
		q := navQueue(60, 42)

		after := moveQueue(q, key)

		positions, pending := after.pendingRemove()
		if !pending {
			t.Fatalf("%q took nothing off the list", key)
		}
		if !slices.Equal(positions, []int{43}) {
			t.Fatalf("%q is removing %v, want just 43 for row 42", key, positions)
		}
	}
}

// Case: the row itself, which must go AT ONCE. The speaker is told once the
// keys are still, and a reader who saw nothing happen in between would think
// the key was broken -- the reason shift edits the list too.
func TestQueueRemove_TheRowGoesAtOnce(t *testing.T) {
	q := navQueue(60, 42)
	going := q.items[42].URI

	after := moveQueue(q, "x")

	if len(after.items) != 59 {
		t.Fatalf("the list holds %d rows, want 59 -- the row must go on the keypress", len(after.items))
	}
	for _, item := range after.items {
		if item.URI == going {
			t.Fatal("the removed track is still in the list")
		}
	}
	if after.items[42].Position != 44 {
		t.Fatalf("row 42 now holds position %d, want 44 -- the rest keep the speaker's numbering",
			after.items[42].Position)
	}
}

// Case: a run of presses, which must reach the speaker as ONE command. Each
// press restarts the wait, so a reader clearing several pays one round trip
// rather than one each.
func TestQueueRemove_ARunOfPressesIsOneCall(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	model, _ := m.Update(keyMsgFor("enter"))
	m = model.(appModel)

	for range 3 {
		model, _ = m.Update(keyMsgFor("x"))
		m = model.(appModel)
	}
	if got := len(queueOf(t, m).items); got != 17 {
		t.Fatalf("the list holds %d rows after three presses, want 17", got)
	}

	// One window holding all three, so closing it is one command rather than
	// the three a press-by-press send would make.
	positions, pending := queueOf(t, m).pendingRemove()
	if !pending {
		t.Fatal("three presses left nothing waiting to be sent")
	}
	if !slices.Equal(positions, []int{1, 2, 3}) {
		t.Fatalf("the window holds %v, want all three tracks", positions)
	}

	gen := queueOf(t, m).removeGen
	model, cmd := m.Update(queueRemoveCommitMsg{gen: gen})
	m = model.(appModel)

	if cmd == nil {
		t.Fatal("the window closed without telling the speaker")
	}
	if _, pending := queueOf(t, m).pendingRemove(); pending {
		t.Fatal("the window is still open after committing")
	}
}

// Case: a tick from a window a later press superseded. It must send nothing:
// the newer window carries the same tracks and more.
func TestQueueRemove_AStaleTickSendsNothing(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	model, _ := m.Update(keyMsgFor("enter"))
	m = model.(appModel)

	model, _ = m.Update(keyMsgFor("x"))
	m = model.(appModel)
	stale := queueOf(t, m).removeGen

	model, _ = m.Update(keyMsgFor("x"))
	m = model.(appModel)

	if _, cmd := m.Update(queueRemoveCommitMsg{gen: stale}); cmd != nil {
		t.Fatal("a superseded window sent a removal")
	}
}

// Case: a selection, which the key must act on instead of the cursor's track.
// The positions are the picked tracks, whether or not the cursor is on one.
func TestQueueRemove_AsksForTheWholeSelection(t *testing.T) {
	q := navQueue(20, 0)
	q = moveQueue(moveQueue(moveQueue(q, "s"), "s"), "s") // rows 0, 1 and 2
	q = moveQueue(q, "end")                               // the cursor leaves them

	positions, pending := moveQueue(q, "x").pendingRemove()
	if !pending {
		t.Fatal("x took nothing off the list")
	}
	if !slices.Equal(positions, []int{1, 2, 3}) {
		t.Fatalf("x is removing %v, want the three picked tracks and not the cursor's", positions)
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

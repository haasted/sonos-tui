package main

import (
	"testing"
	"time"
)

// Case: play next lands, and the cursor stays where remove leaves it. The
// track that moved should still flash, since nothing else on screen shows
// where it went.
func TestQueuePlayNext_HighlightsTheTrackItMoved(t *testing.T) {
	m, _ := deliverQueueWithPlaying(t, 20, 10)
	speaker, _ := m.activeSpeaker()

	// Opening lands on the playing track (position 10); return to the top
	// first so "down" is a known move to Track 2, position 2.
	model, _ := m.Update(keyMsgFor("home"))
	model, _ = model.(appModel).Update(keyMsgFor("down"))
	m = model.(appModel)

	model, cmd := m.Update(keyMsgFor("n"))
	m = model.(appModel)

	sent, ok := playNextOne(t, cmd)
	if !ok {
		t.Fatal("nothing was asked for")
	}

	moved := queueItems(20)[1] // Track 2, the one that moved
	reordered := movedItems(queueItems(20), sent.from-1, sent.to-1)

	model, _ = m.Update(queueLoadedMsg{
		device: speaker.device, room: speaker.device.RoomName(),
		items: reordered, playing: 10,
	})
	m = model.(appModel)
	content := queueOf(t, m)

	if content.cursor != 1 {
		t.Fatalf("the cursor moved to row %d; play next must leave it where it was", content.cursor)
	}
	if glow := content.highlightGlow(moved.URI); glow <= 0 {
		t.Fatalf("the moved track is not highlighted, glow=%v", glow)
	}
}

// Case: a committed shift, which already follows the track with the cursor.
// It does not also flash: the cursor marker is the answer for this move, and
// a second effect on top of it was judged to be one too many.
func TestQueueMove_DoesNotHighlightTheTrackItMoved(t *testing.T) {
	m, _ := deliverQueue(t, 20)
	speaker, _ := m.activeSpeaker()

	model, _ := m.Update(keyMsgFor("J")) // move Track 1 down one place
	m = model.(appModel)
	moved := queueOf(t, m).items[1].URI

	model, _ = m.Update(queueMoveCommitMsg{gen: queueOf(t, m).moveGen})
	m = model.(appModel)

	reordered := movedItems(queueItems(20), 0, 1)
	model, _ = m.Update(queueLoadedMsg{
		device: speaker.device, room: speaker.device.RoomName(), items: reordered,
	})
	m = model.(appModel)
	content := queueOf(t, m)

	if content.items[content.cursor].URI != moved {
		t.Fatal("the cursor did not follow the moved track, so this proves nothing")
	}
	if content.highlighting() {
		t.Fatal("a committed shift move should not also highlight the track")
	}
}

// Case: the flash's own lifetime, decaying from lit to nothing without
// anything else acting on it.
func TestQueueHighlight_FadesToZeroAfterItsDuration(t *testing.T) {
	const uri = "x-sonos-http:track-1.mp3"
	q := navQueue(5, 0).highlight(map[string]bool{uri: true})

	if !q.highlighting() {
		t.Fatal("a highlight just set should be active")
	}
	if glow := q.highlightGlow(uri); glow <= 0 {
		t.Fatalf("a highlight just set should glow, got %v", glow)
	}

	q.highlightAt = time.Now().Add(-highlightDuration - time.Millisecond)

	if q.highlighting() {
		t.Fatal("a highlight past its duration should be done")
	}
	if glow := q.highlightGlow(uri); glow != 0 {
		t.Fatalf("a highlight past its duration should not glow, got %v", glow)
	}
}

// Case: the moment right after landing, which must read as fully lit rather
// than already mid-fade -- a flash with no hold is only ever seen fading.
func TestQueueHighlight_HoldsAtFullGlowBeforeFading(t *testing.T) {
	const uri = "x-sonos-http:track-1.mp3"
	q := navQueue(5, 0).highlight(map[string]bool{uri: true})

	withinHold := time.Duration(float64(highlightDuration) * highlightHold / 2)
	q.highlightAt = time.Now().Add(-withinHold)
	if glow := q.highlightGlow(uri); glow != 1 {
		t.Fatalf("still within the hold, glow is %v, want 1", glow)
	}

	pastHold := time.Duration(float64(highlightDuration) * (1 + highlightHold) / 2)
	q.highlightAt = time.Now().Add(-pastHold)
	if glow := q.highlightGlow(uri); glow <= 0 || glow >= 1 {
		t.Fatalf("past the hold, glow should be easing between 0 and 1, got %v", glow)
	}
}

// Case: the queue that lands no longer holds the track that moved -- another
// controller removed it while the move was in flight. Nothing should light
// up, and nothing should error looking for a row that is not there.
func TestQueuePlayNext_NoHighlightIfTheTrackIsGone(t *testing.T) {
	m, _ := deliverQueueWithPlaying(t, 20, 10)
	speaker, _ := m.activeSpeaker()

	// Opening lands on the playing track (position 10); return to the top
	// first so "down" is a known move to Track 2.
	model, _ := m.Update(keyMsgFor("home"))
	model, _ = model.(appModel).Update(keyMsgFor("down"))
	m = model.(appModel)

	_, cmd := m.Update(keyMsgFor("n"))
	if _, ok := playNextSent(cmd); !ok {
		t.Fatal("nothing was asked for, so this proves nothing")
	}

	without := queueItems(20)[2:] // Track 1 and Track 2 both gone
	model, _ = m.Update(queueLoadedMsg{
		device: speaker.device, room: speaker.device.RoomName(),
		items: without, playing: 8,
	})
	m = model.(appModel)

	if queueOf(t, m).highlighting() {
		t.Fatal("a track no longer in the queue should not be highlighted")
	}
}

// Case: a queue short enough to need no scroll arrows. The flash still has to
// repaint on its own, since nothing else would ask for another frame.
func TestQueuePlayNext_RepaintsEvenWhenTheQueueFits(t *testing.T) {
	m, _ := deliverQueueWithPlaying(t, 3, 1)
	speaker, _ := m.activeSpeaker()

	model, _ := m.Update(keyMsgFor("end")) // cursor to Track 3, position 3
	m = model.(appModel)

	model, cmd := m.Update(keyMsgFor("n"))
	m = model.(appModel)

	sent, ok := playNextOne(t, cmd)
	if !ok {
		t.Fatal("nothing was asked for")
	}

	reordered := movedItems(queueItems(3), sent.from-1, sent.to-1)
	_, cmd = m.Update(queueLoadedMsg{
		device: speaker.device, room: speaker.device.RoomName(),
		items: reordered, playing: 1,
	})
	if cmd == nil {
		t.Fatal("a landing highlight on a queue that fits should still start repainting")
	}
}

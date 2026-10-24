package main

import "testing"

// Case: the flash after play next, in a queue holding one track twice. Only
// the copy that moved lit up; the other is still where it was.
func TestQueueDuplicates_OnlyTheMovedCopyFlashes(t *testing.T) {
	q := dupQueue([]string{"A", "B", "X", "C", "X"}, 4, 2) // cursor on the second X, B playing

	moved, cmd := q.Update(keyMsgFor("n"))
	sent, ok := movesSent(cmd)
	if !ok {
		t.Fatal("play next sent nothing")
	}
	content := moved.(queueContent)
	content.items = itemsFor(applyMoves(queueURIs(q.items), sent.moves)) // A B X X C
	content = content.highlightSettled()

	if content.highlightGlow(selectedTrack{uri: "X", nth: 0}) <= 0 {
		t.Fatal("the copy that moved is not flashing")
	}
	if content.highlightGlow(selectedTrack{uri: "X", nth: 1}) > 0 {
		t.Fatal("the copy that stayed is flashing too")
	}
}

// Case: a J/K shift with another copy of the track above where it lands. The
// cursor follows the copy the reader moved, not the first one in the list.
func TestQueueDuplicates_TheCursorFollowsTheCopyThatMoved(t *testing.T) {
	q := dupQueue([]string{"X", "A", "X", "B", "C"}, 2, 0) // cursor on the second X

	content := settleShift(t, q, "J") // X A B X C

	if content.cursor != 3 {
		t.Fatalf("the cursor is on row %d, want 3: the copy that moved", content.cursor)
	}
}

// Case: the cursor sitting on one copy while the other is played next. It
// stays with the copy it was on, which did not move.
func TestQueueDuplicates_TheCursorStaysOnTheCopyThatStayed(t *testing.T) {
	q := dupQueue([]string{"A", "B", "X", "C", "X"}, 4, 2)
	picked, _ := q.Update(keyMsgFor("s")) // the second X
	q = picked.(queueContent)
	q.cursor = 2 // back onto the first X, which is staying put

	moved, cmd := q.Update(keyMsgFor("n"))
	sent, _ := movesSent(cmd)
	content := moved.(queueContent)
	content.items = itemsFor(applyMoves(queueURIs(q.items), sent.moves)) // A B X X C
	content = content.highlightSettled()

	if content.cursor != 3 {
		t.Fatalf("the cursor is on row %d (%s), want 3: the copy that stayed",
			content.cursor, content.items[content.cursor].URI)
	}
}

// Case: a queue changed elsewhere while a move was landing, so the track is no
// longer where it was sent. The first copy of its URI is the best left to find.
func TestQueueDuplicates_AChangedQueueFallsBackToTheURI(t *testing.T) {
	q := dupQueue([]string{"A", "B", "C", "X"}, 0, 0).awaitHighlight(landing{at: 2, uri: "X"})

	q = q.highlightSettled()

	if q.highlightGlow(selectedTrack{uri: "X"}) <= 0 {
		t.Fatal("a track that landed somewhere else was not found at all")
	}
}

// dupQueue is a queue built from URIs, which may repeat, with the cursor on
// row cursor and the track at position playing playing.
func dupQueue(uris []string, cursor, playing int) queueContent {
	q := queueContent{room: "Køkken", items: itemsFor(uris), rows: 12, width: 70,
		cursor: cursor, playing: playing}
	return q.follow()
}

// settleShift presses key once, commits the shift the way the app does, and
// hands back the content once the speaker's reordered queue has arrived.
func settleShift(t *testing.T, q queueContent, key string) queueContent {
	t.Helper()

	before := queueURIs(q.items)
	pressed, _ := q.Update(keyMsgFor(key))
	content := pressed.(queueContent)

	from, to, pending := content.pendingMove()
	if !pending {
		t.Fatalf("%s opened no move", key)
	}
	content = content.clearMove().settleMove(landing{at: to, uri: content.items[content.cursor].URI})
	content.items = itemsFor(applyMoves(before, []queueMove{{from: from, to: to}}))
	return content.followSettled()
}

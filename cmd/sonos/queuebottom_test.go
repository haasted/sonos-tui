package main

import (
	"slices"
	"testing"
)

// Case: one track, which must land at the very bottom rather than one short
// of it. The off-by-one in MoveQueueTrack makes the last place the awkward
// one: InsertBefore names a spot in the queue with the track still in it.
func TestQueueBottom_SendsTheCursorsTrackToTheEnd(t *testing.T) {
	q := navQueue(8, 2) // Track 3

	_, cmd := q.Update(keyMsgFor("N"))
	sent, ok := movesSent(cmd)
	if !ok {
		t.Fatal("N asked for nothing")
	}

	after := applyMoves(queueURIs(q.everything()), sent.moves)
	if got := after[len(after)-1]; got != uriOf(3) {
		t.Fatalf("the bottom track is %q, want the one the cursor was on", got)
	}
	if len(after) != 8 {
		t.Fatalf("the queue now holds %d tracks", len(after))
	}
}

// Case: several picked, which must land in the order they were picked --
// the same rule play next follows, read the other way up.
func TestQueueBottom_KeepsThePressOrder(t *testing.T) {
	q := navQueue(8, 0)
	q = pickRow(q, 4) // Track 5, picked first
	q = pickRow(q, 1) // Track 2, picked second
	q = pickRow(q, 6) // Track 7, picked third

	_, cmd := q.Update(keyMsgFor("N"))
	sent, _ := movesSent(cmd)

	after := applyMoves(queueURIs(q.everything()), sent.moves)
	want := []string{uriOf(5), uriOf(2), uriOf(7)}
	if got := after[len(after)-3:]; !slices.Equal(got, want) {
		t.Fatalf("the last three are %v, want them in the order picked %v", got, want)
	}
}

// Case: the playing track, which MAY be sent down. It cannot be next of
// itself, which is why play next refuses it, but the bottom is a real place
// for it to go.
func TestQueueBottom_TakesThePlayingTrackToo(t *testing.T) {
	q := navQueue(8, 3) // Track 4
	q.playing = 4

	_, cmd := q.Update(keyMsgFor("N"))
	sent, ok := movesSent(cmd)
	if !ok {
		t.Fatal("N refused the playing track")
	}

	after := applyMoves(queueURIs(q.everything()), sent.moves)
	if got := after[len(after)-1]; got != uriOf(4) {
		t.Fatalf("the bottom track is %q, want the one that was playing", got)
	}
}

// Case: a track already at the bottom. There is nowhere to send it, and a
// request the speaker cannot act on is worth saying rather than sending.
func TestQueueBottom_SaysSoWhenThereIsNowhereToGo(t *testing.T) {
	q := navQueue(8, 7) // the last row

	after, cmd := q.Update(keyMsgFor("N"))

	if _, ok := movesSent(cmd); ok {
		t.Fatal("a track already at the bottom was sent there again")
	}
	if !noticed(cmd, queueAlreadyLast) {
		t.Fatal("the reader was not told there was nowhere to send it")
	}
	if len(after.(queueContent).items) != 8 {
		t.Fatal("the list changed for a move that never happened")
	}
}

// Case: the selection and the cursor afterwards, which must go the way play
// next leaves them.
func TestQueueBottom_DropsTheSelectionAndFlashesWhatMoved(t *testing.T) {
	q := navQueue(8, 0)
	q = pickRow(q, 1)
	q = pickRow(q, 2)

	moved, cmd := q.Update(keyMsgFor("N"))
	sent, _ := movesSent(cmd)
	content := moved.(queueContent)

	if content.selectionCount() != 0 {
		t.Fatalf("%d tracks are still picked", content.selectionCount())
	}

	content.items = itemsFor(applyMoves(queueURIs(q.everything()), sent.moves))
	content = content.highlightSettled()
	for _, track := range []int{2, 3} {
		if content.highlightGlow(selectedTrack{uri: uriOf(track)}) <= 0 {
			t.Fatalf("Track %d moved but is not flashing", track)
		}
	}
}

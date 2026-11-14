package main

import (
	"fmt"
	"slices"
	"testing"

	"github.com/haasted/sonos-tui/internal/sonos"
)

// Case: several picked tracks, which must end up directly after the one
// playing IN THE ORDER THEY WERE PICKED. The moves are sent one at a time,
// each against the queue the one before it left, so the test walks the queue
// through them rather than trusting the numbers.
func TestPlayNextMany_LandsThemInPressOrder(t *testing.T) {
	q := navQueue(20, 9)
	q.playing = 1         // Track 1
	q = moveQueue(q, "s") // Track 10, picked first
	q = pickRow(q, 4)     // Track 5, picked second
	q = pickRow(q, 14)    // Track 15, picked third

	_, cmd := q.Update(keyMsgFor("n"))
	sent, ok := movesSent(cmd)
	if !ok {
		t.Fatal("n asked for nothing")
	}

	after := applyMoves(queueURIs(q.items), sent.moves)
	want := []string{uriOf(10), uriOf(5), uriOf(15)}
	if got := after[1:4]; !slices.Equal(got, want) {
		t.Fatalf("the tracks after the playing one are %v,\nwant them in the order they were picked %v", got, want)
	}
	if after[0] != uriOf(1) {
		t.Fatalf("the playing track moved: position 1 now holds %q", after[0])
	}
}

// Case: the playing track picked along with others. It cannot be next of
// itself, so it stays and the rest go after it.
func TestPlayNextMany_LeavesThePlayingTrackAlone(t *testing.T) {
	q := navQueue(20, 0)
	q.playing = 1
	q = pickRow(q, 0)  // Track 1, the one playing
	q = pickRow(q, 7)  // Track 8
	q = pickRow(q, 11) // Track 12

	_, cmd := q.Update(keyMsgFor("n"))
	sent, _ := movesSent(cmd)

	after := applyMoves(queueURIs(q.items), sent.moves)
	want := []string{uriOf(1), uriOf(8), uriOf(12)}
	if got := after[0:3]; !slices.Equal(got, want) {
		t.Fatalf("the queue starts %v, want the playing track left where it is: %v", got, want)
	}
}

// Case: a picked track already directly after the playing one, and picked
// last. It is already where it belongs, so nothing is sent for it.
func TestPlayNextMany_SendsNothingForATrackAlreadyInPlace(t *testing.T) {
	q := navQueue(20, 0)
	q.playing = 1
	q = pickRow(q, 5) // Track 6
	q = pickRow(q, 1) // Track 2, already next, and picked last

	_, cmd := q.Update(keyMsgFor("n"))
	sent, _ := movesSent(cmd)

	after := applyMoves(queueURIs(q.items), sent.moves)
	want := []string{uriOf(1), uriOf(6), uriOf(2)}
	if got := after[0:3]; !slices.Equal(got, want) {
		t.Fatalf("the queue starts %v, want %v", got, want)
	}
	if len(sent.moves) != 1 {
		t.Fatalf("%d moves were sent; the track already in place needs none", len(sent.moves))
	}
}

// Case: the cursor, which must not follow the tracks away. It settles on the
// nearest track that did not move, once the reordered queue arrives.
func TestPlayNextMany_TheCursorSettlesOnATrackThatStayed(t *testing.T) {
	q := navQueue(20, 0)
	q.playing = 1
	q = pickRow(q, 6) // Track 7
	q = pickRow(q, 7) // Track 8
	q.cursor = 6      // the reader is sitting on one of the picked tracks

	moved, cmd := q.Update(keyMsgFor("n"))
	sent, _ := movesSent(cmd)
	content := moved.(queueContent)

	// The queue comes back as the speaker would leave it.
	content.items = itemsFor(applyMoves(queueURIs(q.items), sent.moves))
	content = content.highlightSettled()

	if content.cursor >= len(content.items) {
		t.Fatalf("the cursor ran off the end at row %d", content.cursor)
	}
	landed := content.items[content.cursor].URI
	if landed == uriOf(7) || landed == uriOf(8) {
		t.Fatalf("the cursor followed a moved track to %q", landed)
	}
}

// Case: the flash, which must reach every track that moved rather than only
// the first of them.
func TestPlayNextMany_EveryMovedTrackFlashes(t *testing.T) {
	q := navQueue(20, 0)
	q.playing = 1
	q = pickRow(q, 6) // Track 7
	q = pickRow(q, 9) // Track 10

	moved, cmd := q.Update(keyMsgFor("n"))
	sent, _ := movesSent(cmd)
	content := moved.(queueContent)
	content.items = itemsFor(applyMoves(queueURIs(q.items), sent.moves))
	content = content.highlightSettled()

	for _, track := range []int{7, 10} {
		if content.highlightGlow(selectedTrack{uri: uriOf(track)}) <= 0 {
			t.Fatalf("Track %d moved but is not flashing", track)
		}
	}
	if content.highlightGlow(selectedTrack{uri: uriOf(3)}) > 0 {
		t.Fatal("a track that did not move is flashing")
	}
}

// Case: nothing picked, which must still act on the track under the cursor.
func TestPlayNextMany_AnEmptySelectionActsOnTheCursor(t *testing.T) {
	q := navQueue(20, 11)
	q.playing = 1

	_, cmd := q.Update(keyMsgFor("n"))
	sent, ok := movesSent(cmd)
	if !ok {
		t.Fatal("n asked for nothing")
	}
	if len(sent.moves) != 1 || sent.moves[0].from != 12 {
		t.Fatalf("n sent %v, want one move of the cursor's track at position 12", sent.moves)
	}
}

// Case: the selection afterwards, which goes the way removal's does.
func TestPlayNextMany_DropsTheSelection(t *testing.T) {
	q := navQueue(20, 0)
	q.playing = 1
	q = pickRow(q, 6)
	q = pickRow(q, 9)

	moved, _ := q.Update(keyMsgFor("n"))
	if got := moved.(queueContent).selectionCount(); got != 0 {
		t.Fatalf("%d tracks are still picked after moving them", got)
	}
}

// pickRow puts the cursor on a row and picks it, leaving the cursor where s
// left it.
func pickRow(q queueContent, row int) queueContent {
	q.cursor = row
	return moveQueue(q, "s")
}

func queueURIs(items []sonos.QueueItem) []string {
	uris := make([]string, 0, len(items))
	for _, item := range items {
		uris = append(uris, item.URI)
	}
	return uris
}

// applyMoves walks a queue through a run of moves, as the speaker would.
func applyMoves(uris []string, moves []queueMove) []string {
	out := slices.Clone(uris)
	for _, move := range moves {
		from := move.from - 1
		uri := out[from]
		out = slices.Delete(out, from, from+1)
		out = slices.Insert(out, move.to-1, uri)
	}
	return out
}

func itemsFor(uris []string) []sonos.QueueItem {
	items := make([]sonos.QueueItem, 0, len(uris))
	for i, uri := range uris {
		items = append(items, sonos.QueueItem{
			Position:      i + 1,
			TrackMetadata: sonos.TrackMetadata{URI: uri, Title: uri, Artist: "An Artist"},
		})
	}
	return items
}

func uriOf(track int) string { return fmt.Sprintf("x-sonos-http:track-%d.mp3", track) }

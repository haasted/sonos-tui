package main

import (
	"strings"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: / narrowing the list as the reader types, matching the title and the
// artist alike and caring about neither's case.
func TestQueueFilter_NarrowsOnTitleAndArtist(t *testing.T) {
	for _, typed := range []string{"nick", "NICK", "cave"} {
		q := typeInto(mixedQueue(), typed)

		if len(q.items) != 2 {
			t.Fatalf("%q showed %d tracks, want the two by Nick Cave", typed, len(q.items))
		}
	}

	q := typeInto(mixedQueue(), "horses")
	if len(q.items) != 1 || q.items[0].Title != "Bright Horses" {
		t.Fatalf("a title match showed %d tracks", len(q.items))
	}
}

// Case: the heading, which must say the list is narrowed. Its count is of
// the rows on screen, and would otherwise read as the whole queue.
func TestQueueFilter_TheHeadingSaysSo(t *testing.T) {
	q := typeInto(mixedQueue(), "nick")

	if head := q.heading(); !strings.Contains(head, "/nick") {
		t.Fatalf("the heading reads %q and does not say the list is filtered", head)
	}
}

// Case: esc, which the modal takes for itself unless the content wants it.
// The first clears the filter; only then does the modal get it.
func TestQueueFilter_EscClearsBeforeTheModalCloses(t *testing.T) {
	q := typeInto(mixedQueue(), "nick")
	if !q.ClaimsEsc() {
		t.Fatal("a narrowed list let esc through to close the modal")
	}

	cleared := moveQueue(q, "esc")
	if len(cleared.items) != 4 {
		t.Fatalf("esc left %d tracks, want the whole queue back", len(cleared.items))
	}
	if cleared.ClaimsEsc() {
		t.Fatal("a second esc would be claimed again rather than closing the modal")
	}
}

// Case: THE POINT OF THE FEATURE. Narrow, pick out what is shown, clear the
// filter, and the picks are still there to act on.
func TestQueueFilter_PicksSurviveClearingIt(t *testing.T) {
	// enter leaves the box, so the keys that act on tracks reach the rows it
	// narrowed to rather than being typed into it.
	q := moveQueue(typeInto(mixedQueue(), "nick"), "enter")
	q = moveQueue(q, "*") // every Nick Cave track

	if q.selectionCount() != 2 {
		t.Fatalf("%d tracks picked out of the two shown", q.selectionCount())
	}

	cleared := moveQueue(q, "esc")
	if cleared.selectionCount() != 2 {
		t.Fatalf("%d tracks are picked once the filter went, want the same two",
			cleared.selectionCount())
	}
	for _, position := range cleared.selectedPositions() {
		if artist := cleared.items[position-1].Artist; artist != "Nick Cave" {
			t.Fatalf("position %d is by %q, so the picks moved to other tracks", position, artist)
		}
	}
}

// Case: inverting while narrowed, which must reach only what is shown. A
// reader cannot mean tracks the filter is hiding from them.
func TestQueueFilter_InvertReachesOnlyWhatIsShown(t *testing.T) {
	q := moveQueue(typeInto(mixedQueue(), "nick"), "enter")

	q = moveQueue(q, "*")
	cleared := moveQueue(q, "esc")

	if cleared.selectionCount() != 2 {
		t.Fatalf("invert picked %d tracks; only the two shown were its business",
			cleared.selectionCount())
	}
}

// Case: typing while the box is open, which must not also be moving the
// cursor. A title holds j and k like any other letters.
func TestQueueFilter_TypingDoesNotMoveTheCursor(t *testing.T) {
	q := typeInto(mixedQueue(), "j")

	if q.filter != "j" {
		t.Fatalf("the filter reads %q; j moved the cursor instead of being typed", q.filter)
	}
}

// Case: enter, which leaves the box but keeps the narrowing, so the keys
// that act on tracks reach the rows it left.
func TestQueueFilter_EnterLeavesTheBoxAndKeepsTheRows(t *testing.T) {
	q := moveQueue(typeInto(mixedQueue(), "nick"), "enter")

	if q.filtering {
		t.Fatal("enter left the filter box open")
	}
	if len(q.items) != 2 {
		t.Fatalf("enter showed %d tracks again, want the narrowing kept", len(q.items))
	}
}

// Case: backspace, widening the list again a letter at a time.
func TestQueueFilter_BackspaceWidensIt(t *testing.T) {
	q := moveQueue(typeInto(mixedQueue(), "nick"), "backspace")

	if q.filter != "nic" {
		t.Fatalf("the filter reads %q after one backspace", q.filter)
	}
}

// mixedQueue is four tracks by two artists, for cases about what a filter
// keeps and what it hides.
func mixedQueue() queueContent {
	tracks := []struct{ title, artist string }{
		{"Bright Horses", "Nick Cave"},
		{"Waiting for You", "Nick Cave"},
		{"Night Raid", "Sleaford Mods"},
		{"Jolly Fucker", "Sleaford Mods"},
	}

	items := make([]sonos.QueueItem, 0, len(tracks))
	for i, track := range tracks {
		items = append(items, sonos.QueueItem{
			Position: i + 1,
			TrackMetadata: sonos.TrackMetadata{
				Title: track.title, Artist: track.artist,
				URI: "x-sonos-http:track-" + string(rune('a'+i)) + ".mp3",
			},
		})
	}
	return queueContent{room: "Køkken", items: items, rows: 12, width: 70}
}

// typeInto opens the filter and types, a key at a time, as a reader would.
func typeInto(q queueContent, text string) queueContent {
	q = moveQueue(q, "/")
	for _, r := range text {
		q = moveQueue(q, string(r))
	}
	return q
}

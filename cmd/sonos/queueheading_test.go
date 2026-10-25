package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: nothing picked, which is the ordinary state. The heading says only
// what it always said -- a count of zero is noise on every queue nobody is
// selecting in.
func TestQueueHeading_SaysNothingWithNothingPicked(t *testing.T) {
	head := navQueue(20, 0).heading()

	if strings.Contains(head, "selected") {
		t.Fatalf("the heading talks about a selection nobody made: %q", head)
	}
}

// Case: tracks picked, which the rows alone cannot report to a reader who
// has scrolled away from them.
func TestQueueHeading_CountsWhatIsPicked(t *testing.T) {
	q := pickRuns(navQueue(20, 0), 5)

	if got := q.heading(); !strings.HasSuffix(got, "· 5 selected") {
		t.Fatalf("the heading reads %q, want it to end with the count of five", got)
	}
}

// Case: a handful of tracks, which the reader can see marked and has not
// lost count of. A number appearing on the first press is noise.
func TestQueueHeading_SaysNothingUntilThereAreEnough(t *testing.T) {
	for picked := 1; picked <= selectionCountFloor; picked++ {
		q := pickRuns(navQueue(20, 0), picked)
		if got := q.heading(); strings.Contains(got, "selected") {
			t.Fatalf("%d picked and the heading already counts them: %q", picked, got)
		}
	}

	q := pickRuns(navQueue(20, 0), selectionCountFloor+1)
	if got := q.heading(); !strings.Contains(got, "selected") {
		t.Fatalf("%d picked and the heading says nothing: %q", selectionCountFloor+1, got)
	}
}

// Case: the count beside the range, on a queue too long to fit. Both belong
// in the heading, and the selection comes last: the range answers where the
// reader is, which they need first.
func TestQueueHeading_SitsAfterTheRange(t *testing.T) {
	q := pickRuns(navQueue(60, 0), 5)

	head := q.heading()
	if !strings.Contains(head, " of 60") {
		t.Fatalf("the heading lost its range: %q", head)
	}
	if !strings.HasSuffix(head, "· 5 selected") {
		t.Fatalf("the heading reads %q, want the count after the range", head)
	}
}

// Case: a picked track that a reload took away. It stays in the selection on
// purpose, so a read landing on a part-written queue does not drop it -- but
// the heading counts what is HERE, or it reports tracks the reader cannot see.
func TestQueueHeading_CountsOnlyWhatIsStillInTheQueue(t *testing.T) {
	q := pickRuns(navQueue(20, 0), 5)
	q.items = q.items[5:] // every picked track leaves the queue

	if len(q.selected) != 5 {
		t.Fatal("the selection dropped the tracks, so this proves nothing")
	}
	if got := q.heading(); strings.Contains(got, "selected") {
		t.Fatalf("the heading counts tracks no longer in the queue: %q", got)
	}
}

// Case: the heading as drawn. The count has to reach the screen, not just
// the string the test asks for.
func TestQueueHeading_TheCountIsDrawn(t *testing.T) {
	q := pickRuns(navQueue(20, 0), 5)

	first := strings.Split(q.View().Content, "\n")[0]
	if !strings.Contains(plain(first), "5 selected") {
		t.Fatalf("the heading row reads %q", plain(first))
	}

	// And it is drawn apart from the room name, which is the point of
	// putting it there: a reader must be able to tell the two at a glance.
	room, count := strings.Split(q.room, " ")[0], "5 selected"
	if styleOf(first, room) == styleOf(first, count) {
		t.Fatal("the count is drawn in the room name's own colour")
	}
}

// Case: the box, which must not change width as tracks are picked. This is
// what son-4zo.17 bought, and the reason the count can be shown at all.
func TestQueueHeading_PickingDoesNotResizeTheBox(t *testing.T) {
	modal := fixedQueueModal(20)
	before, _ := modal.BoxSize()

	modal.Content = moveQueue(moveQueue(navQueue(20, 0), "*"), "home")
	after, _ := modal.BoxSize()

	if before != after {
		t.Fatalf("the box was %d wide and became %d when everything was picked", before, after)
	}
}

// Case: a box too narrow to hold the range and the count together. The count
// is what goes: the range says where in the queue the reader is, which they
// need before they need to know how many are picked. Neither may overflow
// the box, which wraps what it cannot fit and pushes a row off the bottom.
func TestQueueHeading_DropsTheCountBeforeItOverflows(t *testing.T) {
	q := queueContent{listView: listView[sonos.QueueItem]{items: queueItems(130), rows: 16, width: 32}, room: "Soveværelse"}
	q = moveQueue(q, "*") // everything picked, the longest the count can be

	head := q.heading()

	if lipgloss.Width(head) > 32 {
		t.Fatalf("the heading is %d wide in a box of 32: %q", lipgloss.Width(head), head)
	}
	if strings.Contains(head, "selected") {
		t.Fatalf("the count stayed and something else was cut: %q", head)
	}
	if !strings.Contains(head, "of 130") {
		t.Fatalf("the range went instead of the count: %q", head)
	}
}

// Case: a heading too long even without a count, which a long room name on a
// narrow terminal gives. It is clipped rather than wrapped.
func TestQueueHeading_ClipsWhatCannotFitAtAll(t *testing.T) {
	q := queueContent{listView: listView[sonos.QueueItem]{items: queueItems(130), rows: 16, width: 32}, room: strings.Repeat("Soveværelse ", 4)}

	if got := lipgloss.Width(q.heading()); got > 32 {
		t.Fatalf("the heading is %d wide in a box of 32: %q", got, q.heading())
	}
}

// pickRuns picks the first n tracks, the cursor stepping down as s leaves it.
func pickRuns(q queueContent, n int) queueContent {
	for range n {
		q = moveQueue(q, "s")
	}
	return q
}

// styleOf is the escape sequence a piece of text is drawn with, for a case
// about two parts of one line looking different.
func styleOf(line, text string) string {
	at := strings.Index(plain(line), text)
	if at < 0 {
		return ""
	}
	// The run of escapes immediately before the text in the raw line.
	raw := line
	for _, prefix := range strings.Split(raw, "\x1b[") {
		if strings.Contains(prefix, text) {
			return strings.SplitN(prefix, "m", 2)[0]
		}
	}
	return ""
}

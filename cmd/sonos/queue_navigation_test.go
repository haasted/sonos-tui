package main

import (
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: the keys that move by one row, including the vi pair the player binds
// to something else. j is the previous track outside this list.
func TestQueueNav_MovesOneRow(t *testing.T) {
	for _, key := range []string{"down", "j"} {
		q := navQueue(60, 20)
		if got := cursorAfter(q, key); got != 21 {
			t.Fatalf("%q moved to row %d, not one row down", key, got)
		}
	}
	for _, key := range []string{"up", "k"} {
		q := navQueue(60, 20)
		if got := cursorAfter(q, key); got != 19 {
			t.Fatalf("%q moved to row %d, not one row up", key, got)
		}
	}
}

// Case: a whole window at a time, which is what a queue of 250 items needs.
func TestQueueNav_MovesOneWindow(t *testing.T) {
	page := navQueue(60, 0).page()
	if page < 2 {
		t.Fatalf("the test window holds only %d rows", page)
	}

	for _, key := range []string{"pgdown", "f", "space"} {
		if got := cursorAfter(navQueue(60, 0), key); got != page {
			t.Fatalf("%q moved to row %d, not one window of %d", key, got, page)
		}
	}
	for _, key := range []string{"pgup", "b"} {
		if got := cursorAfter(navQueue(60, 2*page), key); got != page {
			t.Fatalf("%q moved to row %d, not one window of %d", key, got, page)
		}
	}
}

// Case: the half-window move, which keeps half the rows on screen so the
// reader stays oriented without reading the heading.
func TestQueueNav_MovesHalfAWindow(t *testing.T) {
	half := navQueue(60, 0).halfPage()
	if half < 1 {
		t.Fatal("a half window must always move at least one row")
	}

	if got := cursorAfter(navQueue(60, 0), "ctrl+d"); got != half {
		t.Fatalf("ctrl+d moved to row %d, not half a window of %d", got, half)
	}
	if got := cursorAfter(navQueue(60, 2*half), "ctrl+u"); got != half {
		t.Fatalf("ctrl+u moved to row %d, not half a window of %d", got, half)
	}
}

// Case: reaching either end in one action, which is the other half of what a
// long queue needs.
func TestQueueNav_JumpsToEachEnd(t *testing.T) {
	for _, key := range []string{"end", "G"} {
		q := navQueue(60, 0)
		if got := cursorAfter(q, key); got != q.lastRow() {
			t.Fatalf("%q moved to row %d, not the last row at %d", key, got, q.lastRow())
		}
	}
	for _, key := range []string{"home", "g"} {
		if got := cursorAfter(navQueue(60, 40), key); got != 0 {
			t.Fatalf("%q moved to row %d, not the top", key, got)
		}
	}
}

// Case: the reader wants the playing track without hunting for it, wherever
// it sits relative to the row they are on.
func TestQueueNav_CJumpsToWhatIsPlaying(t *testing.T) {
	q := navQueue(60, 40)
	q.playing = 6

	moved := moveQueue(q, "c")
	if moved.cursor != 5 {
		t.Fatalf("c landed on row %d, want row 5 for the track at position 6", moved.cursor)
	}
	if moved.offset > 5 {
		t.Fatalf("c left row 5 outside the window, offset %d", moved.offset)
	}
}

// Case: c far down a long queue. The playing track lands two rows below the
// top, so the rest of the window shows the tracks to come.
func TestQueueNav_CLeavesRoomForWhatComesNext(t *testing.T) {
	q := navQueue(60, 0)
	q.playing = 31

	moved := moveQueue(q, "c")
	if moved.cursor != 30 || moved.offset != 28 {
		t.Fatalf("c landed on row %d with the window at %d, want row 30 at 28", moved.cursor, moved.offset)
	}
}

// Case: c on the first tracks and the last ones. The window cannot go above
// the first row or below the last, so the landing clamps without empty rows.
func TestQueueNav_CClampsAtEachEnd(t *testing.T) {
	for _, tc := range []struct{ playing, offset int }{{1, 0}, {2, 0}, {58, 50}, {60, 50}} {
		q := navQueue(60, 30)
		q.playing = tc.playing

		if moved := moveQueue(q, "c"); moved.offset != tc.offset {
			t.Fatalf("c on the track at %d put the window at %d, want %d", tc.playing, moved.offset, tc.offset)
		}
	}
}

// Case: c in a short window. Two rows above would push the track to the
// bottom or off it, so fewer are kept: one when three tracks show, none
// when two or one do.
func TestQueueNav_CKeepsLessAboveInAShortWindow(t *testing.T) {
	for _, tc := range []struct{ rows, above int }{{5, 1}, {4, 0}, {3, 0}} {
		q := navQueue(60, 0)
		q.rows, q.playing = tc.rows, 31

		if moved := moveQueue(q, "c"); moved.cursor-moved.offset != tc.above {
			t.Fatalf("%d rows kept %d above the track, want %d", tc.rows, moved.cursor-moved.offset, tc.above)
		}
	}
}

// Case: c with the playing track already on screen. Only the cursor moves;
// the window stays, so nothing on screen jumps.
func TestQueueNav_CKeepsTheWindowWhenTheTrackShows(t *testing.T) {
	q := navQueue(60, 40) // the window shows rows 31 to 40
	q.playing = 39

	moved := moveQueue(q, "c")
	if moved.cursor != 38 || moved.offset != q.offset {
		t.Fatalf("c landed on row %d with the window at %d, want row 38 with the window still at %d",
			moved.cursor, moved.offset, q.offset)
	}
}

// Case: moving on from where c landed. Only the landing changed: the cursor
// still walks the window before the window scrolls.
func TestQueueNav_MovingAfterCScrollsTheLeast(t *testing.T) {
	q := navQueue(60, 0)
	q.playing = 31
	q = moveQueue(q, "c")

	if moved := moveQueue(q, "down"); moved.offset != q.offset {
		t.Fatalf("down from the landing scrolled the window from %d to %d", q.offset, moved.offset)
	}
	if moved := moveQueue(moveQueue(moveQueue(q, "up"), "up"), "up"); moved.offset != q.offset-1 {
		t.Fatalf("three rows up put the window at %d, want %d", moved.offset, q.offset-1)
	}
}

// Case: nothing plays from the queue -- a station, say -- so c has no row to
// aim at and must leave the cursor exactly where it was.
func TestQueueNav_CDoesNothingWithNoTrackPlaying(t *testing.T) {
	q := navQueue(60, 40) // playing is left at its zero value: nothing

	if got := cursorAfter(q, "c"); got != 40 {
		t.Fatalf("c moved the cursor to %d with nothing playing, want it to stay at 40", got)
	}
}

// Case: a page move near the end, which must land on the last window rather
// than run past it or refuse to move at all.
func TestQueueNav_StopsAtTheEndRatherThanRunningPast(t *testing.T) {
	q := navQueue(60, 58) // one short of the last row

	moved := moveQueue(q, "pgdown")
	if moved.cursor != q.lastRow() {
		t.Fatalf("a page down near the end landed on row %d, not %d", moved.cursor, q.lastRow())
	}
	if moved.bumping() {
		t.Fatal("a move that went somewhere must not light an edge")
	}
}

// Case: a move at an end, which cannot go anywhere. It lights the edge instead,
// which is what tells a held key apart from a program that has stopped.
func TestQueueNav_LightsTheEdgeOnlyWhereNothingMoves(t *testing.T) {
	for _, key := range []string{"up", "k", "pgup", "b", "ctrl+u", "home", "g"} {
		if moved := moveQueue(navQueue(60, 0), key); !moved.bumping() {
			t.Fatalf("%q at the top did not light the edge", key)
		}
	}

	end := navQueue(60, 59) // the last row
	for _, key := range []string{"down", "j", "pgdown", "f", "space", "ctrl+d", "end", "G"} {
		if moved := moveQueue(end, key); !moved.bumping() {
			t.Fatalf("%q at the end did not light the edge", key)
		}
	}
}

// Case: a queue shorter than its window, where every move is a move to nowhere.
func TestQueueNav_AShortQueueNeverScrolls(t *testing.T) {
	for _, key := range []string{"down", "pgdown", "ctrl+d", "end", "up", "home"} {
		if moved := moveQueue(navQueue(2, 0), key); moved.offset != 0 {
			t.Fatalf("%q scrolled a queue of two items to row %d", key, moved.offset)
		}
	}
}

// navQueue is a list with the cursor already on a given row, and the window
// wherever that row put it.
func navQueue(items, cursor int) queueContent {
	q := queueContent{listView: listView[sonos.QueueItem]{items: queueItems(items), rows: 12, width: 70, cursor: cursor}, room: "Køkken"}
	return q.follow()
}

// moveQueue applies one key to the list, as the modal would.
func moveQueue(q queueContent, key string) queueContent {
	updated, _ := q.Update(keyMsgFor(key))
	return updated.(queueContent)
}

func cursorAfter(q queueContent, key string) int { return moveQueue(q, key).cursor }

// Case: a queue of one item, where the first row is also the last. The edge a
// key lights must follow the key, not the row it lands on.
func TestQueueNav_AOneItemQueueLightsTheEdgeTheKeyMeans(t *testing.T) {
	if moved := moveQueue(navQueue(1, 0), "end"); moved.edge != bumpBottom {
		t.Fatal("end on a one-item queue lit the top edge")
	}
	if moved := moveQueue(navQueue(1, 0), "home"); moved.edge != bumpTop {
		t.Fatal("home on a one-item queue lit the bottom edge")
	}
}

// Case: the window, which is not what the keys move. It follows the cursor,
// and only by as much as it must to keep the cursor on screen.
func TestQueueNav_TheWindowFollowsTheCursor(t *testing.T) {
	q := navQueue(60, 0)
	last := q.visibleRows() - 1

	onScreen := moveQueue(q, "ctrl+d")
	if onScreen.offset != 0 {
		t.Fatalf("the window moved to %d for a cursor still on screen", onScreen.offset)
	}

	q.cursor = last
	pushed := moveQueue(q, "down")
	if pushed.offset != 1 {
		t.Fatalf("the window moved to %d, not the one row the cursor needed", pushed.offset)
	}
}

// Case: "?" inside the queue, which must explain the list rather than the
// player, and leave the queue open underneath.
func TestQueueNav_QuestionMarkShowsTheQueueKeys(t *testing.T) {
	m := pressKey(sizedModel(t), "enter")
	if !m.queue.IsOpen() {
		t.Fatal("enter did not open the queue")
	}

	m = pressKey(m, "?")
	if !m.help.IsOpen() {
		t.Fatal("? did not open the help popup over the queue")
	}
	if !m.queue.IsOpen() {
		t.Fatal("the queue closed when its help opened")
	}

	content, ok := m.help.Content.(helpContent)
	if !ok {
		t.Fatalf("the help modal holds %T", m.help.Content)
	}
	if !helpKeySet(content.table)["pgup"] {
		t.Fatal("the popup over the queue does not document the queue keys")
	}
}

// Case: "?" over the player, which must go back to the player's own keys after
// the queue has shown its own. One modal serves both.
func TestQueueNav_QuestionMarkOverThePlayerShowsThePlayerKeys(t *testing.T) {
	m := pressKey(pressKey(pressKey(sizedModel(t), "enter"), "?"), "?") // open, help, close help
	m = pressKey(m, "esc")                                              // close the queue

	m = pressKey(m, "?")
	content := m.help.Content.(helpContent)
	if helpKeySet(content.table)["pgup"] {
		t.Fatal("the player's popup still shows the queue keys")
	}
	if !helpKeySet(content.table)["enter"] {
		t.Fatal("the player's popup lost its own keys")
	}
}

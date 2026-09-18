package main

import (
	"testing"
)

// Case: the keys that move by one row, including the vi pair the player binds
// to something else. j is the previous track outside this list.
func TestQueueNav_MovesOneRow(t *testing.T) {
	for _, key := range []string{"down", "j"} {
		q := navQueue(60, 20)
		if got := offsetAfter(q, key); got != 21 {
			t.Fatalf("%q moved to row %d, not one row down", key, got)
		}
	}
	for _, key := range []string{"up", "k"} {
		q := navQueue(60, 20)
		if got := offsetAfter(q, key); got != 19 {
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
		if got := offsetAfter(navQueue(60, 0), key); got != page {
			t.Fatalf("%q moved to row %d, not one window of %d", key, got, page)
		}
	}
	for _, key := range []string{"pgup", "b"} {
		if got := offsetAfter(navQueue(60, 2*page), key); got != page {
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

	if got := offsetAfter(navQueue(60, 0), "ctrl+d"); got != half {
		t.Fatalf("ctrl+d moved to row %d, not half a window of %d", got, half)
	}
	if got := offsetAfter(navQueue(60, 2*half), "ctrl+u"); got != half {
		t.Fatalf("ctrl+u moved to row %d, not half a window of %d", got, half)
	}
}

// Case: reaching either end in one action, which is the other half of what a
// long queue needs.
func TestQueueNav_JumpsToEachEnd(t *testing.T) {
	for _, key := range []string{"end", "G"} {
		q := navQueue(60, 0)
		if got := offsetAfter(q, key); got != q.maxOffset() {
			t.Fatalf("%q moved to row %d, not the last window at %d", key, got, q.maxOffset())
		}
	}
	for _, key := range []string{"home", "g"} {
		if got := offsetAfter(navQueue(60, 40), key); got != 0 {
			t.Fatalf("%q moved to row %d, not the top", key, got)
		}
	}
}

// Case: a page move near the end, which must land on the last window rather
// than run past it or refuse to move at all.
func TestQueueNav_StopsAtTheEndRatherThanRunningPast(t *testing.T) {
	q := navQueue(60, 0)
	q.offset = q.maxOffset() - 1

	moved := moveQueue(q, "pgdown")
	if moved.offset != q.maxOffset() {
		t.Fatalf("a page down near the end landed on row %d, not %d", moved.offset, q.maxOffset())
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

	end := navQueue(60, 0)
	end.offset = end.maxOffset()
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

func navQueue(items, offset int) queueContent {
	return queueContent{room: "Køkken", items: queueItems(items), rows: 12, width: 70, offset: offset}
}

// moveQueue applies one key to the list, as the modal would.
func moveQueue(q queueContent, key string) queueContent {
	updated, _ := q.Update(keyMsgFor(key))
	return updated.(queueContent)
}

func offsetAfter(q queueContent, key string) int { return moveQueue(q, key).offset }

// Case: a queue that fits its window, where the top row is also the last one.
// The edge a key lights must follow the key, not the row it lands on.
func TestQueueNav_AShortQueueLightsTheEdgeTheKeyMeans(t *testing.T) {
	if moved := moveQueue(navQueue(2, 0), "end"); moved.edge != bumpBottom {
		t.Fatal("end on a short queue lit the top edge")
	}
	if moved := moveQueue(navQueue(2, 0), "home"); moved.edge != bumpTop {
		t.Fatal("home on a short queue lit the bottom edge")
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

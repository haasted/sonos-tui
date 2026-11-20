package main

import (
	"slices"
	"testing"

	"github.com/haasted/sonos-tui/internal/sonos"
)

// Case: J on a narrowed list. The track goes below the next track shown, and
// the speaker is asked for that place in the whole queue, not for the row.
func TestQueueMoveFiltered_JGoesBelowTheNextTrackShown(t *testing.T) {
	q := moveQueue(typeInto(mixedQueue(), "mods"), "enter") // positions 3 and 4
	if len(q.items) != 2 || q.items[q.cursor].Position != 3 {
		t.Fatalf("the narrowed list is not positions 3 and 4 with the cursor on 3: %+v", q.items)
	}
	whole := slices.Clone(q.everything())

	q = moveQueue(q, "J")

	if q.items[1].Position != 3 || q.cursor != 1 {
		t.Fatalf("the track did not move below position 4 on screen: %v, cursor %d", positionsOf(q.items), q.cursor)
	}
	assertMove(t, q, whole, 3, 4)
}

// Case: K on a narrowed list, the other way.
func TestQueueMoveFiltered_KGoesAboveTheTrackShownBefore(t *testing.T) {
	q := moveQueue(moveQueue(typeInto(mixedQueue(), "mods"), "enter"), "down") // on 4
	whole := slices.Clone(q.everything())

	q = moveQueue(q, "K")

	if q.items[0].Position != 4 || q.cursor != 0 {
		t.Fatalf("the track did not move above position 3 on screen: %v, cursor %d", positionsOf(q.items), q.cursor)
	}
	assertMove(t, q, whole, 4, 3)
}

// Case: visible neighbours far apart in the queue. One press passes every
// hidden track between them, in both directions.
func TestQueueMoveFiltered_ThePressPassesTheHiddenTracks(t *testing.T) {
	q := shownQueue(12, 2, 7, 11)
	whole := slices.Clone(q.everything())

	down := moveQueue(q, "J")
	assertMove(t, down, whole, 2, 7)
	if got := positionsOf(down.items); !slices.Equal(got, []int{7, 2, 11}) {
		t.Fatalf("the narrowed list reads %v, want [7 2 11]", got)
	}

	up := moveQueue(moveQueue(q, "end"), "K")
	assertMove(t, up, whole, 11, 7)
	if got := positionsOf(up.items); !slices.Equal(got, []int{2, 11, 7}) {
		t.Fatalf("the narrowed list reads %v, want [2 11 7]", got)
	}
}

// Case: several presses in one window. They make one move from where the
// track started to where it is now shown.
func TestQueueMoveFiltered_SeveralPressesAreOneMove(t *testing.T) {
	q := shownQueue(12, 2, 7, 11)
	whole := slices.Clone(q.everything())

	q = moveQueue(moveQueue(q, "J"), "J")
	assertMove(t, q, whole, 2, 11)
	if got := positionsOf(q.items); !slices.Equal(got, []int{7, 11, 2}) {
		t.Fatalf("the narrowed list reads %v, want [7 11 2]", got)
	}

	q = moveQueue(q, "K")
	assertMove(t, q, whole, 2, 10)
	if got := positionsOf(q.items); !slices.Equal(got, []int{7, 2, 11}) {
		t.Fatalf("the narrowed list reads %v, want [7 2 11]", got)
	}

	// Back on the first row, but just above position 7: the hidden tracks
	// it passed stay above it.
	q = moveQueue(q, "K")
	assertMove(t, q, whole, 2, 6)
	if got := positionsOf(q.items); !slices.Equal(got, []int{2, 7, 11}) {
		t.Fatalf("the narrowed list reads %v, want [2 7 11]", got)
	}
}

// Case: the first and the last row of a narrowed list. A key that cannot move
// the track lights the edge and opens no window, even with tracks hidden
// beyond it.
func TestQueueMoveFiltered_TheEndsAnswerRatherThanMove(t *testing.T) {
	top, cmd := shownQueue(12, 2, 7, 11).Update(keyMsgFor("K"))
	if !top.(queueContent).bumping() || cmd != nil {
		t.Fatal("K on the first row shown did not light the edge, or opened a window")
	}

	end, cmd := moveQueue(shownQueue(12, 2, 7, 11), "end").Update(keyMsgFor("J"))
	if !end.(queueContent).bumping() || cmd != nil {
		t.Fatal("J on the last row shown did not light the edge, or opened a window")
	}
}

// Case: the filter closed while the window is open. The whole queue shows the
// move, and the move to send stays the same.
func TestQueueMoveFiltered_ClosingTheFilterKeepsTheMove(t *testing.T) {
	q := shownQueue(12, 2, 7, 11)
	whole := slices.Clone(q.everything())

	q = moveQueue(moveQueue(q, "J"), "esc")
	if q.filter != "" {
		t.Fatal("esc did not clear the filter")
	}
	assertMove(t, q, whole, 2, 7)
	if q.items[q.cursor].Position != 2 {
		t.Fatalf("the cursor sits on position %d, not on the moved track", q.items[q.cursor].Position)
	}
}

// Case: the commit, through the whole program. The speaker is told the
// queue position and looks for the track there.
func TestQueueMoveFiltered_TheCommitNamesTheQueuePosition(t *testing.T) {
	m, _ := deliverQueue(t, 12)
	for _, key := range []string{"home", "/", "1", "enter", "J"} {
		m = pressKey(m, key)
	}
	content := queueOf(t, m)
	if got := positionsOf(content.items); !slices.Equal(got, []int{10, 1, 11, 12}) {
		t.Fatalf("the narrowed list reads %v, want [10 1 11 12]", got)
	}

	model, cmd := m.Update(queueMoveCommitMsg{gen: content.moveGen})
	if cmd == nil {
		t.Fatal("the window closed without sending anything")
	}
	settling := queueOf(t, model.(appModel)).settling
	want := landing{at: 10, uri: "x-sonos-http:track-1.mp3"}
	if len(settling) != 1 || settling[0] != want {
		t.Fatalf("the commit looks for %+v, want %+v", settling, want)
	}
}

// Case: no filter. The move is still the row the track sits on.
func TestQueueMoveFiltered_WithoutAFilterTheMoveIsUnchanged(t *testing.T) {
	q := navQueue(20, 4)
	whole := slices.Clone(q.everything())

	q = moveQueue(moveQueue(moveQueue(q, "J"), "J"), "J")
	assertMove(t, q, whole, 5, 8)
	if q.cursor != 7 {
		t.Fatalf("the cursor is on row %d, want 7", q.cursor)
	}
}

// shownQueue is a queue of n tracks narrowed to the positions in shown, with
// the filter box closed and the cursor on the first row.
func shownQueue(n int, shown ...int) queueContent {
	items := queueItems(n)
	for i := range items {
		if slices.Contains(shown, items[i].Position) {
			items[i].Artist = "Shown Artist"
		}
	}
	q := queueContent{listView: listView[sonos.QueueItem]{items: items, rows: 12, width: 70}, room: "Køkken"}
	return moveQueue(typeInto(q, "shown"), "enter")
}

// assertMove checks that the pending move is from -> to, and that the speaker,
// doing that move on the whole queue it holds, gets the queue drawn here.
func assertMove(t *testing.T, q queueContent, whole []sonos.QueueItem, from, to int) {
	t.Helper()

	gotFrom, gotTo, pending := q.pendingMove()
	if !pending || gotFrom != from || gotTo != to {
		t.Fatalf("the move is %d -> %d (pending %v), want %d -> %d", gotFrom, gotTo, pending, from, to)
	}

	speaker := positionsOf(movedItems(whole, from-1, to-1))
	if drawn := positionsOf(q.everything()); !slices.Equal(speaker, drawn) {
		t.Fatalf("the speaker would hold %v, the list holds %v", speaker, drawn)
	}
}

func positionsOf(items []sonos.QueueItem) []int {
	out := make([]int, len(items))
	for i, item := range items {
		out[i] = item.Position
	}
	return out
}

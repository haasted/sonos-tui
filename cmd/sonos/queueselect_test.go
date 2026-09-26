package main

import (
	"slices"
	"strings"
	"testing"
)

// Case: the key itself, which must pick the track out and then step down, so
// marking a run is s, s, s rather than a step between each.
func TestQueueSelect_TogglesAndStepsDown(t *testing.T) {
	after := moveQueue(navQueue(20, 4), "s")

	if !pickedRow(after, 4) {
		t.Fatal("s did not pick the track under the cursor out")
	}
	if after.cursor != 5 {
		t.Fatalf("s left the cursor on row %d, want row 5 -- it should step down", after.cursor)
	}
}

// Case: right, the second key for the same toggle. It must leave the list
// exactly as s does, at an ordinary row and at the last one where the step
// down turns into a bump.
func TestQueueSelect_RightTogglesJustAsSDoes(t *testing.T) {
	for _, row := range []int{4, 19} {
		pressedS := moveQueue(navQueue(20, row), "s")
		pressedRight := moveQueue(navQueue(20, row), "right")

		if !slices.Equal(pressedS.selected, pressedRight.selected) {
			t.Fatalf("row %d: s picked %v, right picked %v", row, pressedS.selected, pressedRight.selected)
		}
		if pressedS.cursor != pressedRight.cursor {
			t.Fatalf("row %d: s left the cursor on %d, right on %d", row, pressedS.cursor, pressedRight.cursor)
		}
		if pressedS.bumping() != pressedRight.bumping() {
			t.Fatalf("row %d: s bumped %v, right bumped %v", row, pressedS.bumping(), pressedRight.bumping())
		}
	}
}

// Case: the two keys inside one run, which a reader switching hands produces.
// One binding rather than two that agree, so either key puts back what the
// other picked.
func TestQueueSelect_RightAndSReachTheSameMark(t *testing.T) {
	picked := moveQueue(navQueue(20, 4), "s")
	putBack := moveQueue(moveQueue(picked, "up"), "right")

	if putBack.selectionCount() != 0 {
		t.Fatalf("right did not put back what s picked: %d still picked", putBack.selectionCount())
	}
}

// Case: pressing s again on a track already picked, which must put it back
// rather than pick it twice.
func TestQueueSelect_TogglesBackOff(t *testing.T) {
	on := moveQueue(navQueue(20, 4), "s")
	off := moveQueue(moveQueue(on, "up"), "s")

	if pickedRow(off, 4) {
		t.Fatal("pressing s twice on one track left it picked")
	}
	if len(off.selected) != 0 {
		t.Fatalf("%d tracks are still held, want none", len(off.selected))
	}
}

// Case: the same track sitting in the queue twice, which a URI alone cannot
// tell apart. Picking one copy must leave the other alone: once a command
// acts on the selection, removing "that track" would otherwise take the
// wrong row away.
func TestQueueSelect_TellsTwoCopiesOfOneTrackApart(t *testing.T) {
	for _, pick := range []int{0, 4} {
		q := duplicateQueue(6, 0, 4)
		q.cursor = pick

		after := moveQueue(q, "s")
		rows := after.pickedPositions()

		if !pickedRow(after, pick) {
			t.Fatalf("picking row %d did not mark it", pick)
		}
		if len(rows) != 1 {
			t.Fatalf("picking row %d marked %d rows, want 1 -- the other copy came too",
				pick, len(rows))
		}
	}
}

// Case: an empty queue, and one still being read. There is no track under
// the cursor to pick.
func TestQueueSelect_PicksNothingWithNoTrackUnderTheCursor(t *testing.T) {
	for name, q := range map[string]queueContent{
		"empty":   {room: "Køkken", rows: 12, width: 70},
		"loading": {room: "Køkken", rows: 12, width: 70, loading: true},
	} {
		if after := moveQueue(q, "s"); after.selectionCount() != 0 {
			t.Fatalf("a %s queue picked something out", name)
		}
	}
}

// Case: S, which empties the selection whatever the cursor is doing and
// leaves the cursor where it found it.
func TestQueueSelect_ClearEmptiesAndLeavesTheCursor(t *testing.T) {
	q := moveQueue(moveQueue(moveQueue(navQueue(20, 4), "s"), "s"), "s")
	if q.selectionCount() != 3 {
		t.Fatalf("%d tracks are picked before clearing, want 3", q.selectionCount())
	}
	was := q.cursor

	after := moveQueue(q, "S")

	if after.selectionCount() != 0 {
		t.Fatalf("%d tracks survived S", after.selectionCount())
	}
	if after.cursor != was {
		t.Fatalf("S moved the cursor from row %d to row %d", was, after.cursor)
	}
}

// Case: * with something picked, which must swap the two sides and leave the
// cursor alone.
func TestQueueSelect_InvertSwapsBothSides(t *testing.T) {
	q := moveQueue(moveQueue(navQueue(20, 0), "s"), "s") // rows 0 and 1
	was := q.cursor

	after := moveQueue(q, "*")
	rows := after.pickedPositions()

	if pickedRow(after, 0) || pickedRow(after, 1) {
		t.Fatal("invert left a row picked that was picked before")
	}
	if len(rows) != 18 {
		t.Fatalf("invert picked %d of 20 rows, want the other 18", len(rows))
	}
	if after.cursor != was {
		t.Fatalf("invert moved the cursor from row %d to row %d", was, after.cursor)
	}
}

// Case: * on an empty selection, which picks the whole queue out. Select-all
// comes free rather than needing a key of its own.
func TestQueueSelect_InvertOnNothingPicksEverything(t *testing.T) {
	after := moveQueue(navQueue(20, 0), "*")

	if after.selectionCount() != 20 {
		t.Fatalf("invert on an empty selection picked %d of 20", after.selectionCount())
	}
}

// Case: the order the reader picked them in, which is what the selection is
// a slice for. Nothing reads it yet; the commands that act on a selection
// will.
func TestQueueSelect_KeepsThePressOrder(t *testing.T) {
	q := navQueue(20, 10)
	q = moveQueue(q, "s")                    // the track at row 10, picked first
	q = moveQueue(moveQueue(q, "home"), "s") // the track at row 0, picked second

	want := []selectedTrack{{uri: q.items[10].URI}, {uri: q.items[0].URI}}
	if !slices.Equal(q.selected, want) {
		t.Fatalf("the selection reads %v, want them in the order they were pressed: %v",
			q.selected, want)
	}
}

// Case: the queue reloading under a selection -- an event, or a move landing.
// A picked track keeps its mark wherever it has moved to, which is why the
// selection is held by track and not by a row.
func TestQueueSelect_SurvivesAReload(t *testing.T) {
	m, _ := deliverQueueWithPlaying(t, 20, 10)
	speaker, _ := m.activeSpeaker()

	model, _ := m.Update(keyMsgFor("home"))
	model, _ = model.(appModel).Update(keyMsgFor("s")) // pick row 0
	m = model.(appModel)
	picked := queueItems(20)[0].URI

	// The same tracks come back in a different order, as a move would leave
	// them: the picked track is now the last row rather than the first.
	reordered := movedItems(queueItems(20), 0, 19)
	model, _ = m.Update(queueLoadedMsg{
		device: speaker.device, room: speaker.device.RoomName(),
		items: reordered, playing: 10,
	})
	content := queueOf(t, model.(appModel))

	if content.items[19].URI != picked {
		t.Fatal("the reordered queue did not put the picked track last, so this proves nothing")
	}
	rows := content.pickedPositions()
	if !pickedRow(content, 19) {
		t.Fatal("the picked track lost its mark when the queue came back reordered")
	}
	if len(rows) != 1 {
		t.Fatalf("%d rows are picked after the reload, want 1", len(rows))
	}
}

// Case: a picked track that leaves the queue. It stays in the selection and
// simply stops resolving to a row, rather than being pruned -- a read landing
// on a part-written queue would otherwise drop a selection about to come back.
func TestQueueSelect_AVanishedTrackIsIgnoredRatherThanPruned(t *testing.T) {
	q := moveQueue(navQueue(20, 0), "s")
	q.items = q.items[1:] // the track is gone from the list, not from the selection

	if q.selectionCount() != 0 {
		t.Fatal("a track no longer in the queue still resolves to a row")
	}
	if len(q.selected) != 1 {
		t.Fatal("the selection dropped the track rather than holding it against its return")
	}
}

// Case: the marking on screen, which must set a selected row apart from one
// that is merely under the cursor or merely playing.
func TestQueueSelect_APickedRowLooksDifferent(t *testing.T) {
	// Both end with the cursor on row 5, so row 4 differs by the selection
	// alone rather than by the cursor having moved off it.
	plainRow := strings.Split(navQueue(20, 5).View().Content, "\n")[queueHeaderRows+4]
	pickedRow := strings.Split(moveQueue(navQueue(20, 4), "s").View().Content, "\n")[queueHeaderRows+4]

	if plainRow == pickedRow {
		t.Fatal("picking a track out changed nothing about how its row is drawn")
	}
	if plain(plainRow) != plain(pickedRow) {
		t.Fatalf("picking a track changed its text, not just its colour:\n%q\n%q",
			plain(plainRow), plain(pickedRow))
	}
}

// Case: the copy every update of this content makes. The selection is the one
// field backed by an array copies share, so a careless append would reach
// copies that were meant to be thrown away.
func TestQueueSelect_OneCopyDoesNotWriteIntoAnother(t *testing.T) {
	picked := moveQueue(navQueue(20, 0), "s")

	// A second press against the SAME starting content, as a superseded
	// update would leave behind.
	other := moveQueue(picked, "s")

	if picked.selectionCount() != 1 {
		t.Fatalf("a later copy's change reached back: the first holds %d picked tracks, want 1",
			picked.selectionCount())
	}
	if other.selectionCount() != 2 {
		t.Fatalf("the later copy holds %d picked tracks, want 2", other.selectionCount())
	}
}

// Case: a command with nothing picked, which must still answer for the track
// under the cursor. An empty selection means "this one", not "none of them".
func TestQueueSelect_AnEmptySelectionLeavesTheCursorInCharge(t *testing.T) {
	q := navQueue(20, 4)

	positions, pending := moveQueue(q, "x").pendingRemove()
	if !pending {
		t.Fatal("x took nothing off the list")
	}
	if len(positions) != 1 || positions[0] != 5 {
		t.Fatalf("x is removing %v, want just position 5 for the cursor on row 4", positions)
	}
}

// duplicateQueue builds a queue where the item at copy holds the same track as
// the item at original, as a reader adding one song to the queue twice leaves
// it. Only the URI is shared; the position still counts up.
func duplicateQueue(items, original, copy int) queueContent {
	q := navQueue(items, 0)
	position := q.items[copy].Position
	q.items[copy] = q.items[original]
	q.items[copy].Position = position
	return q
}

// pickedRow reports whether the track on a row is picked. The selection is
// held by the speaker's position, not by the row, so a test that means "this
// row" has to ask the row which position it carries.
func pickedRow(q queueContent, row int) bool {
	if row < 0 || row >= len(q.items) {
		return false
	}
	return q.pickedPositions()[q.items[row].Position]
}

package main

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: the track following the key. It has to appear to move on every press,
// which is the one place the list edits itself rather than waiting for the
// speaker.
func TestQueueMove_TheTrackMovesOnScreenAtOnce(t *testing.T) {
	for _, key := range []string{"J", "shift+down"} {
		q := navQueue(60, 10)
		was := q.items[10].Title

		moved, _ := q.Update(keyMsgFor(key))
		content := moved.(queueContent)

		if content.items[11].Title != was {
			t.Fatalf("%q left %q at row 11", key, content.items[11].Title)
		}
		if content.cursor != 11 {
			t.Fatalf("%q left the cursor at %d, not on the track it moved", key, content.cursor)
		}
	}
	for _, key := range []string{"K", "shift+up"} {
		q := navQueue(60, 10)
		was := q.items[10].Title

		moved, _ := q.Update(keyMsgFor(key))
		content := moved.(queueContent)

		if content.items[9].Title != was {
			t.Fatalf("%q left %q at row 9", key, content.items[9].Title)
		}
		if content.cursor != 9 {
			t.Fatalf("%q left the cursor at %d", key, content.cursor)
		}
	}
}

// Case: the list either side of the moved track, which must be the same tracks
// in the same order. A move that reorders anything else is a bug that reads as
// a move.
func TestQueueMove_NothingElseMoves(t *testing.T) {
	q := navQueue(10, 2)
	before := titles(q)

	moved, _ := q.Update(keyMsgFor("J"))
	after := titles(moved.(queueContent))

	before[2], before[3] = before[3], before[2]
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("row %d is %q, want %q\nbefore %v\nafter  %v", i, after[i], before[i], before, after)
		}
	}
}

// Case: a run of presses, which must reach the speaker as ONE move from where
// the track started. Ten presses are not ten reorders.
//
// Each press restarts the wait and so leaves a tick of its own behind. All but
// the last are superseded by generation and send nothing, which is what counts
// here rather than how many ticks were scheduled on the way.
func TestQueueMove_ARunOfPressesIsOneMove(t *testing.T) {
	m, _ := deliverQueue(t, 60)

	var gens []int
	for range 5 {
		model, _ := m.Update(keyMsgFor("J"))
		m = model.(appModel)
		gens = append(gens, queueOf(t, m).moveGen)
	}

	from, to, pending := queueOf(t, m).pendingMove()
	if !pending {
		t.Fatal("nothing is waiting to be sent")
	}
	if from != 1 || to != 6 {
		t.Fatalf("the move is %d -> %d, want 1 -> 6", from, to)
	}

	var sent int
	for _, gen := range gens {
		if _, cmd := m.Update(queueMoveCommitMsg{gen: gen}); cmd != nil {
			sent++
		}
	}
	if sent != 1 {
		t.Fatalf("five presses sent %d moves, want 1", sent)
	}
}

// Case: the wait must be trailing rather than a fixed window from the first
// press. A fixed one closes while the reader is still pressing, and the settle
// that follows then discards whatever comes next.
func TestQueueMove_EachPressRestartsTheWait(t *testing.T) {
	first, _ := navQueue(60, 10).Update(keyMsgFor("J"))
	second, cmd := first.(queueContent).Update(keyMsgFor("J"))

	if first.(queueContent).moveGen == second.(queueContent).moveGen {
		t.Fatal("the second press joined the first one's window instead of restarting the wait")
	}
	if cmd == nil {
		t.Fatal("the second press left no wait of its own behind")
	}
}

// Case: the defect this replaced. A fixed window committed part-way through a
// run, and the settle that followed swallowed the presses that came next --
// measured at forty-four percent of them, with nothing said. Every press in a
// run must move the track.
func TestQueueMove_ARunLosesNoPresses(t *testing.T) {
	q := navQueue(60, 10)
	moving := q.items[10].URI

	model := any(q)
	for range 20 {
		updated, _ := model.(queueContent).Update(keyMsgFor("J"))
		model = updated
	}

	content := model.(queueContent)
	if content.cursor != 30 {
		t.Fatalf("twenty presses left the cursor on row %d, want 30", content.cursor)
	}
	if content.items[30].URI != moving {
		t.Fatalf("twenty presses left %q on row 30, not the track being moved",
			content.items[30].Title)
	}
}

// Case: the ends of the list, where a track cannot go further. The edge bump
// already answers a key that cannot move.
func TestQueueMove_TheEndsAnswerRatherThanMove(t *testing.T) {
	top := navQueue(20, 0)
	moved, cmd := top.Update(keyMsgFor("K"))
	if !moved.(queueContent).bumping() {
		t.Fatal("a track at the top did not light the edge")
	}
	if cmd != nil {
		t.Fatal("a move that did not happen opened a window")
	}

	end := navQueue(20, 19)
	moved, _ = end.Update(keyMsgFor("J"))
	if !moved.(queueContent).bumping() {
		t.Fatal("a track at the end did not light the edge")
	}
}

// Case: the queue changing under an open window. The move was built from
// positions in the old queue, so it cannot be sent — what sat at those
// positions may not be there now.
func TestQueueMove_AChangedQueueAbandonsAPendingMove(t *testing.T) {
	m, _ := deliverQueue(t, 40)
	speaker, _ := m.activeSpeaker()

	model, _ := m.Update(keyMsgFor("J"))
	m = model.(appModel)
	if _, _, pending := queueOf(t, m).pendingMove(); !pending {
		t.Fatal("no move is waiting, so this proves nothing")
	}

	model, cmd := m.Update(queueLoadedMsg{
		device: speaker.device, room: speaker.device.RoomName(), items: queueItems(40),
	})
	m = model.(appModel)

	if _, _, pending := queueOf(t, m).pendingMove(); pending {
		t.Fatal("a move built from the old queue survived the new one")
	}
	if !noticed(cmd, queueMoveAbandoned) {
		t.Fatal("the reader was not told the move was dropped")
	}
}

// Case: a tick from a window the reader has left behind. Three faults in this
// project came from a chain that outlived what started it.
func TestQueueMove_AStaleWindowCommitsNothing(t *testing.T) {
	m, _ := deliverQueue(t, 40)

	model, _ := m.Update(keyMsgFor("J"))
	m = model.(appModel)
	stale := queueOf(t, m).moveGen

	_, cmd := m.Update(queueMoveCommitMsg{gen: stale + 1})
	if cmd != nil {
		t.Fatal("a tick from another window sent a move")
	}
}

// Case: the window closing, which is where the speaker is finally told.
func TestQueueMove_TheWindowCommitsOnce(t *testing.T) {
	m, device, _ := withQueueServer(t, 20)
	model, _ := m.Update(keyMsgFor("enter"))
	m = model.(appModel)

	model, _ = m.Update(keyMsgFor("J"))
	m = model.(appModel)
	gen := queueOf(t, m).moveGen

	model, cmd := m.Update(queueMoveCommitMsg{gen: gen})
	m = model.(appModel)
	if cmd == nil {
		t.Fatal("the window closed without sending anything")
	}
	if _, _, pending := queueOf(t, m).pendingMove(); pending {
		t.Fatal("the window is still open after committing")
	}

	notice, ok := cmd().(panes.NoticeMsg)
	if !ok {
		t.Fatalf("the commit produced %T", cmd())
	}
	if !notice.Confirmation {
		t.Fatal("a move must answer as a confirmation")
	}
	_ = device
}

// noticed reports whether a command raises a notice carrying text. A command
// here is usually a batch, since the arrows re-arm their repaint on the same
// message, so both shapes are walked.
func noticed(cmd tea.Cmd, text string) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case panes.NoticeMsg:
		return msg.Text == text
	case tea.BatchMsg:
		for _, branch := range msg {
			if noticed(branch, text) {
				return true
			}
		}
	}
	return false
}

func titles(q queueContent) []string {
	out := make([]string, len(q.items))
	for i, item := range q.items {
		out[i] = item.Title
	}
	return out
}

// Case: a removal while a move is still waiting to be sent. Both address
// tracks by position, and a removal shifts every position below it — so a move
// committed afterwards would name a track the reader never chose.
func TestQueueMove_ARemovalDropsAPendingMove(t *testing.T) {
	q := navQueue(40, 10)

	moved, _ := q.Update(keyMsgFor("J"))
	q = moved.(queueContent)
	if _, _, pending := q.pendingMove(); !pending {
		t.Fatal("no move is waiting, so this proves nothing")
	}

	after, cmd := q.Update(keyMsgFor("x"))
	if _, _, pending := after.(queueContent).pendingMove(); pending {
		t.Fatal("a move survived a removal that moves every position under it")
	}
	if cmd == nil {
		t.Fatal("the removal asked for nothing")
	}
}

// Case: the queue a move itself produced. It is not a change under the reader
// but the answer they were waiting for, and treating it as a change is what
// made tracks appear to jump: the move was dropped and the cursor was left on
// a row that now held something else.
func TestQueueMove_TheQueueAMoveProducedIsNotAChangeUnderTheReader(t *testing.T) {
	m, _ := deliverQueue(t, 20)
	speaker, _ := m.activeSpeaker()

	model, _ := m.Update(keyMsgFor("J")) // move Track 1 down one place
	m = model.(appModel)
	moved := queueOf(t, m).items[1].URI

	model, _ = m.Update(queueMoveCommitMsg{gen: queueOf(t, m).moveGen})
	m = model.(appModel)

	// The speaker answers with an order of its own. It need not match the rows
	// drawn here: the off-by-one in ReorderTracksInQueue is unverified, and
	// another controller can have moved something in the meantime. Wherever the
	// track really went is where the cursor belongs.
	reordered := queueItems(20)
	track := reordered[0]
	reordered = append(reordered[:0:0], reordered[1:]...)
	reordered = append(reordered[:3], append([]sonos.QueueItem{track}, reordered[3:]...)...)

	model, cmd := m.Update(queueLoadedMsg{
		device: speaker.device, room: speaker.device.RoomName(), items: reordered,
	})
	m = model.(appModel)
	content := queueOf(t, m)

	if noticed(cmd, queueMoveAbandoned) {
		t.Fatal("the reader was told their own move changed the queue under them")
	}
	if content.items[content.cursor].URI != moved {
		t.Fatalf("the cursor sits on %q, not on the track it moved",
			content.items[content.cursor].Title)
	}
}

// Case: presses arriving between a move being sent and its queue coming back.
// Every position held then describes the queue from before the move, so a
// second move built on them would name the wrong track.
func TestQueueMove_PressesWaitForTheQueueToCatchUp(t *testing.T) {
	q := navQueue(40, 10)
	moved, _ := q.Update(keyMsgFor("J"))
	q = moved.(queueContent).clearMove().settleMove("x-sonos-http:track-11.mp3")

	before := titles(q)
	after, cmd := q.Update(keyMsgFor("J"))

	if cmd != nil {
		t.Fatal("a press opened a window while the last move was still landing")
	}
	for i, title := range titles(after.(queueContent)) {
		if title != before[i] {
			t.Fatal("the list moved while it was waiting to be replaced")
		}
	}
}

// Case: a queue that never comes back. The event that would end the wait can
// be dropped, and a reader must not be left unable to move anything.
func TestQueueMove_WaitingForAQueueExpires(t *testing.T) {
	q := navQueue(40, 10).settleMove("x-sonos-http:track-11.mp3")
	if !q.awaitingQueue() {
		t.Fatal("a move just sent should be waiting")
	}

	q.awaitingAt = time.Now().Add(-queueWriteSettle - time.Second)
	if q.awaitingQueue() {
		t.Fatal("a wait with no answer must give up rather than strand the reader")
	}
	if _, cmd := q.Update(keyMsgFor("J")); cmd == nil {
		t.Fatal("moving is still blocked after the wait expired")
	}
}

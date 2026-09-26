package main

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: the key, which must take the track the cursor is on rather than the
// row it sits at. A queue scrolled down has the two far apart.
func TestQueueRemove_TakesTheTrackUnderTheCursor(t *testing.T) {
	for _, key := range []string{"x", "delete"} {
		q := navQueue(60, 42)

		after := moveQueue(q, key)

		positions, pending := after.pendingRemove()
		if !pending {
			t.Fatalf("%q took nothing off the list", key)
		}
		if !slices.Equal(positions, []int{43}) {
			t.Fatalf("%q is removing %v, want just 43 for row 42", key, positions)
		}
	}
}

// Case: the row itself, which must answer AT ONCE but not disappear. The
// speaker is told once the keys are still; a row that vanished first would
// shorten the list under the reader and hide what is on its way out.
func TestQueueRemove_TheRowIsMarkedAtOnce(t *testing.T) {
	q := navQueue(60, 42)

	after := moveQueue(q, "x")

	if len(after.items) != 60 {
		t.Fatalf("the list holds %d rows, want all 60 until the speaker answers", len(after.items))
	}
	if !after.isRemoving(43) {
		t.Fatal("the track under the cursor is not marked as going")
	}
	if after.isRemoving(44) {
		t.Fatal("the row below was marked too")
	}
}

// Case: how a going row is drawn. It has to read as leaving rather than as
// another kind of important, and only its colouring may change -- a row whose
// text changed would move the columns under the reader.
func TestQueueRemove_AGoingRowIsDimmed(t *testing.T) {
	plainRow := strings.Split(navQueue(20, 5).View().Content, "\n")[queueHeaderRows+4]
	goingRow := strings.Split(moveQueue(navQueue(20, 4), "x").View().Content, "\n")[queueHeaderRows+4]

	if plainRow == goingRow {
		t.Fatal("removing a track changed nothing about how its row is drawn")
	}
	if plain(plainRow) != plain(goingRow) {
		t.Fatalf("the row's text changed, not just its colouring:\n%q\n%q",
			plain(plainRow), plain(goingRow))
	}
}

// Case: the cursor, which must not be left on a row that is going. x would
// do nothing there, and a key that does nothing reads as a broken one.
func TestQueueRemove_TheCursorLeavesTheGoingRow(t *testing.T) {
	after := moveQueue(navQueue(20, 4), "x")

	if after.isRemoving(after.items[after.cursor].Position) {
		t.Fatalf("the cursor sits on row %d, which is on its way out", after.cursor)
	}
	if after.cursor != 5 {
		t.Fatalf("the cursor is on row %d, want the next row that is staying", after.cursor)
	}
}

// Case: pressing x twice on one track. The second naming would reach
// whatever the speaker puts in its place, so it must be ignored.
func TestQueueRemove_WillNotSendOneTrackTwice(t *testing.T) {
	q := navQueue(20, 4)
	once := moveQueue(q, "x")

	twice := moveQueue(moveQueue(once, "up"), "x")

	if got, _ := twice.pendingRemove(); len(got) != 1 {
		t.Fatalf("the window holds %v, want the track named once", got)
	}
}

// Case: a run of presses, which must reach the speaker as ONE command. Each
// press restarts the wait, so a reader clearing several pays one round trip
// rather than one each.
func TestQueueRemove_ARunOfPressesIsOneCall(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	model, _ := m.Update(keyMsgFor("enter"))
	m = model.(appModel)

	for range 3 {
		model, _ = m.Update(keyMsgFor("x"))
		m = model.(appModel)
	}
	if got := len(queueOf(t, m).items); got != 20 {
		t.Fatalf("the list holds %d rows, want all 20 until the speaker answers", got)
	}

	// One window holding all three, so closing it is one command rather than
	// the three a press-by-press send would make.
	positions, pending := queueOf(t, m).pendingRemove()
	if !pending {
		t.Fatal("three presses left nothing waiting to be sent")
	}
	if !slices.Equal(positions, []int{1, 2, 3}) {
		t.Fatalf("the window holds %v, want all three tracks", positions)
	}

	gen := queueOf(t, m).removeGen
	model, cmd := m.Update(queueRemoveCommitMsg{gen: gen})
	m = model.(appModel)

	if cmd == nil {
		t.Fatal("the window closed without telling the speaker")
	}
	if _, pending := queueOf(t, m).pendingRemove(); pending {
		t.Fatal("the window is still open after committing")
	}
}

// Case: a tick from a window a later press superseded. It must send nothing:
// the newer window carries the same tracks and more.
func TestQueueRemove_AStaleTickSendsNothing(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	model, _ := m.Update(keyMsgFor("enter"))
	m = model.(appModel)

	model, _ = m.Update(keyMsgFor("x"))
	m = model.(appModel)
	stale := queueOf(t, m).removeGen

	model, _ = m.Update(keyMsgFor("x"))
	m = model.(appModel)

	if _, cmd := m.Update(queueRemoveCommitMsg{gen: stale}); cmd != nil {
		t.Fatal("a superseded window sent a removal")
	}
}

// Case: a selection, which the key must act on instead of the cursor's track.
// The positions are the picked tracks, whether or not the cursor is on one.
func TestQueueRemove_AsksForTheWholeSelection(t *testing.T) {
	q := navQueue(20, 0)
	q = moveQueue(moveQueue(moveQueue(q, "s"), "s"), "s") // rows 0, 1 and 2
	q = moveQueue(q, "end")                               // the cursor leaves them

	positions, pending := moveQueue(q, "x").pendingRemove()
	if !pending {
		t.Fatal("x took nothing off the list")
	}
	if !slices.Equal(positions, []int{1, 2, 3}) {
		t.Fatalf("x is removing %v, want the three picked tracks and not the cursor's", positions)
	}
}

// Case: what the selection is afterwards. The tracks are about to stop
// existing, so the marks go with them rather than being left to puzzle over.
func TestQueueRemove_DropsTheSelection(t *testing.T) {
	q := moveQueue(moveQueue(navQueue(20, 0), "s"), "s")

	updated, _ := q.Update(keyMsgFor("x"))
	if got := updated.(queueContent).selectionCount(); got != 0 {
		t.Fatalf("%d tracks are still picked after removing them", got)
	}
}

// Case: an empty queue, and one still being read. There is no track under the
// cursor, so there is nothing to ask for.
func TestQueueRemove_AsksForNothingWithNoTrackUnderTheCursor(t *testing.T) {
	for name, q := range map[string]queueContent{
		"empty":   {room: "Køkken", rows: 12, width: 70},
		"loading": {room: "Køkken", rows: 12, width: 70, loading: true},
	} {
		if _, cmd := q.Update(keyMsgFor("x")); cmd != nil {
			t.Fatalf("a %s queue asked for a track to be removed", name)
		}
	}
}

// Case: the whole path, from the key to the speaker. The speaker here answers
// every command, so a confirmation is what should come back.
func TestQueueRemove_ConfirmsWhatHappened(t *testing.T) {
	m, device, _ := withQueueServer(t, 20)

	model, _ := m.Update(keyMsgFor("enter"))
	m = model.(appModel)

	_, cmd := m.Update(queueRemoveMsg{positions: []int{3}})
	if cmd == nil {
		t.Fatal("nothing was sent to the speaker")
	}

	notice, ok := cmd().(panes.NoticeMsg)
	if !ok {
		t.Fatalf("the removal produced %T", cmd())
	}
	if !notice.Confirmation {
		t.Fatal("a removal must answer as a confirmation, which yields the rule")
	}
	if notice.Level != panes.NoticeInfo {
		t.Fatalf("a removal that worked reported level %v", notice.Level)
	}
	_ = device
}

var _ tea.Msg = queueRemoveMsg{}

// Case: a removal attempted while an earlier change is still unanswered. The
// positions on this list describe the queue from before that change, so a
// removal built on them would take the wrong track -- and the version check
// would not catch it, the version being the one that change produced.
func TestQueueRemove_RefusesWhileTheSpeakerHasNotAnswered(t *testing.T) {
	q := navQueue(20, 5).awaitQueue()

	after, cmd := q.Update(keyMsgFor("x"))

	if len(after.(queueContent).items) != 20 {
		t.Fatal("a row went while the queue was waiting to be replaced")
	}
	if cmd != nil {
		t.Fatal("a removal was sent against positions that have since moved")
	}
}

// Case: sending the removal, which must start that wait. Nothing naming a
// position may follow until the speaker answers.
func TestQueueRemove_CommittingWaitsForTheQueue(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	model, _ := m.Update(keyMsgFor("enter"))
	m = model.(appModel)

	model, _ = m.Update(keyMsgFor("x"))
	m = model.(appModel)
	if queueOf(t, m).awaitingQueue() {
		t.Fatal("the wait began before anything was sent")
	}

	model, _ = m.Update(queueRemoveCommitMsg{gen: queueOf(t, m).removeGen})
	m = model.(appModel)

	if !queueOf(t, m).awaitingQueue() {
		t.Fatal("the removal went to the speaker without waiting for its queue")
	}
}

// Case: the queue coming back after a removal. It names no track to follow
// or flash -- the tracks are gone -- but it must still end the wait, or the
// reader is locked out until the wait expires.
func TestQueueRemove_TheQueueComingBackEndsTheWait(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	model, _ := m.Update(keyMsgFor("enter"))
	m = model.(appModel)
	speaker, _ := m.activeSpeaker()

	model, _ = m.Update(keyMsgFor("x"))
	m = model.(appModel)
	model, _ = m.Update(queueRemoveCommitMsg{gen: queueOf(t, m).removeGen})
	m = model.(appModel)

	model, _ = m.Update(queueLoadedMsg{
		device: speaker.device, room: speaker.device.RoomName(),
		items: queueItems(19), playing: 1,
	})
	m = model.(appModel)

	if queueOf(t, m).awaitingQueue() {
		t.Fatal("the queue came back and the wait did not end")
	}
	if _, cmd := m.Update(keyMsgFor("x")); cmd == nil {
		t.Fatal("removing is still blocked after the queue came back")
	}
}

// Case: the queue coming back without the tracks. The marks go with it: the
// speaker's own list is what is staying, so nothing is left struck through.
func TestQueueRemove_TheMarksGoWhenTheQueueLands(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	model, _ := m.Update(keyMsgFor("enter"))
	m = model.(appModel)
	speaker, _ := m.activeSpeaker()

	model, _ = m.Update(keyMsgFor("x"))
	m = model.(appModel)
	model, _ = m.Update(queueRemoveCommitMsg{gen: queueOf(t, m).removeGen})
	m = model.(appModel)

	if content := queueOf(t, m); !content.isRemoving(1) {
		t.Fatal("the track stopped being marked as soon as it was sent")
	}

	model, _ = m.Update(queueLoadedMsg{
		device: speaker.device, room: speaker.device.RoomName(),
		items: queueItems(19), playing: 1,
	})
	content := queueOf(t, model.(appModel))

	if _, pending := content.pendingRemove(); pending {
		t.Fatal("the window is still open after the queue landed")
	}
	if content.isRemoving(1) {
		t.Fatal("a row is still struck through after the speaker answered")
	}
}

// Case: the size of the list through a whole removal. It must not shorten on
// the keypress -- that is what made the modal jump, and what hid from the
// reader which tracks were on their way out.
func TestQueueRemove_TheListKeepsItsLengthUntilTheSpeakerAnswers(t *testing.T) {
	q := navQueue(20, 0)

	for range 4 {
		q = moveQueue(q, "x")
		if len(q.items) != 20 {
			t.Fatalf("the list is down to %d rows before the speaker said anything", len(q.items))
		}
	}
}

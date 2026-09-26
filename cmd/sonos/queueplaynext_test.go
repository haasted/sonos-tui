package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Case: a track below the playing one, moved up to sit right after it. This is
// the direction that needs no off-by-one correction inside MoveQueueTrack.
func TestQueuePlayNext_FromBelowMovesUpOnePlace(t *testing.T) {
	q := navQueue(20, 11) // cursor on Track 12, position 12
	q.playing = 10

	_, cmd := q.Update(keyMsgFor("n"))
	sent, ok := playNextOne(t, cmd)
	if !ok {
		t.Fatal("nothing was asked for")
	}
	if sent.from != 12 || sent.to != 11 {
		t.Fatalf("the move is %d -> %d, want 12 -> 11", sent.from, sent.to)
	}
}

// Case: a track above the playing one, moved down past it. Lifting a track
// from above shifts the playing one up by one place first, so the target here
// is the playing position itself, not one past it.
func TestQueuePlayNext_FromAboveMovesDownOnePlace(t *testing.T) {
	q := navQueue(20, 8) // cursor on Track 9, position 9
	q.playing = 10

	_, cmd := q.Update(keyMsgFor("n"))
	sent, ok := playNextOne(t, cmd)
	if !ok {
		t.Fatal("nothing was asked for")
	}
	if sent.from != 9 || sent.to != 10 {
		t.Fatalf("the move is %d -> %d, want 9 -> 10", sent.from, sent.to)
	}
}

// Case: the cursor already on the track playing. It is already the current
// one, and the key must answer rather than appear broken.
func TestQueuePlayNext_ThePlayingTrackAnswersRatherThanMoving(t *testing.T) {
	q := navQueue(20, 9) // cursor on Track 10, position 10
	q.playing = 10

	moved, cmd := q.Update(keyMsgFor("n"))
	if _, ok := playNextSent(cmd); ok {
		t.Fatal("the playing track was sent a move")
	}
	if !noticed(cmd, queueAlreadyPlaying) {
		t.Fatal("the reader was not told the track is already playing")
	}
	if moved.(queueContent).settling != nil {
		t.Fatal("nothing was sent, so nothing should be waited on")
	}
}

// Case: the cursor already on the track directly after the one playing. It is
// already next, and sending a move for it would ask the speaker for nothing.
func TestQueuePlayNext_TheTrackAlreadyNextAnswersRatherThanMoving(t *testing.T) {
	q := navQueue(20, 10) // cursor on Track 11, position 11
	q.playing = 10

	_, cmd := q.Update(keyMsgFor("n"))
	if _, ok := playNextSent(cmd); ok {
		t.Fatal("a track already next was sent a move")
	}
	if !noticed(cmd, queueAlreadyNext) {
		t.Fatal("the reader was not told the track is already next")
	}
}

// Case: a station playing. Its queue can be seen but nothing in it is
// current, so there is no "next" for the key to mean.
func TestQueuePlayNext_AStationAnswersRatherThanSilence(t *testing.T) {
	q := navQueue(20, 5)
	q.playing = 0 // what playingPosition returns for a source other than the queue

	_, cmd := q.Update(keyMsgFor("n"))
	if _, ok := playNextSent(cmd); ok {
		t.Fatal("a station's queue was sent a move")
	}
	if !noticed(cmd, queuePlayNextNoTrack) {
		t.Fatal("the reader was not told there is nothing playing from the queue")
	}
}

// Case: the cursor on the last row, past the end of a shorter queue. There is
// no track under it to move.
func TestQueuePlayNext_NoTrackUnderTheCursorSendsNothing(t *testing.T) {
	q := navQueue(0, 0)
	q.playing = 1

	_, cmd := q.Update(keyMsgFor("n"))
	if cmd != nil {
		t.Fatal("an empty queue was sent a move")
	}
}

// Case: the queue the move itself produces, once it lands. The cursor stays
// on the row it was on, exactly as remove leaves it, so it now shows the
// track that was beneath the one that moved away.
func TestQueuePlayNext_CursorStaysAndShowsWhatWasBeneath(t *testing.T) {
	m, _ := deliverQueueWithPlaying(t, 20, 10)
	speaker, _ := m.activeSpeaker()

	// Opening lands on the playing track (position 10); return to the top
	// first so "down" is a known move to Track 2, position 2.
	model, _ := m.Update(keyMsgFor("home"))
	model, _ = model.(appModel).Update(keyMsgFor("down"))
	m = model.(appModel)

	model, cmd := m.Update(keyMsgFor("n"))
	m = model.(appModel)

	sent, ok := playNextOne(t, cmd)
	if !ok {
		t.Fatal("nothing was asked for")
	}

	wasBeneath := queueItems(20)[2] // Track 3, directly beneath Track 2
	reordered := movedItems(queueItems(20), sent.from-1, sent.to-1)

	model, _ = m.Update(queueLoadedMsg{
		device: speaker.device, room: speaker.device.RoomName(),
		items: reordered, playing: 10,
	})
	m = model.(appModel)
	content := queueOf(t, m)

	if content.cursor != 1 {
		t.Fatalf("the cursor moved to row %d; it should stay on row 1", content.cursor)
	}
	if content.items[content.cursor].URI != wasBeneath.URI {
		t.Fatalf("row 1 shows %q, want %q -- the track that was beneath the one that moved",
			content.items[content.cursor].Title, wasBeneath.Title)
	}
}

// Case: the bottom row, which has nothing beneath it to slide up. The reader
// asked about this one directly: the cursor should land on the track that was
// above it instead, the same clamp remove already relies on.
func TestQueuePlayNext_TheBottomRowLandsOnTheTrackBefore(t *testing.T) {
	m, _ := deliverQueueWithPlaying(t, 20, 10)
	speaker, _ := m.activeSpeaker()

	model, _ := m.Update(keyMsgFor("end")) // cursor to Track 20, the last row
	m = model.(appModel)

	model, cmd := m.Update(keyMsgFor("n"))
	m = model.(appModel)

	sent, ok := playNextOne(t, cmd)
	if !ok {
		t.Fatal("nothing was asked for")
	}

	wasAbove := queueItems(20)[18] // Track 19, directly above Track 20
	reordered := movedItems(queueItems(20), sent.from-1, sent.to-1)

	model, _ = m.Update(queueLoadedMsg{
		device: speaker.device, room: speaker.device.RoomName(),
		items: reordered, playing: 10,
	})
	m = model.(appModel)
	content := queueOf(t, m)

	if content.cursor != content.lastRow() {
		t.Fatalf("the cursor sits at row %d, not on the last row", content.cursor)
	}
	if content.items[content.cursor].URI != wasAbove.URI {
		t.Fatalf("the last row shows %q, want %q -- the track that was above the one that moved",
			content.items[content.cursor].Title, wasAbove.Title)
	}
}

// Case: a shift already waiting to be sent. Both commands address tracks by
// position, and a played-next track shifts everything between where it was
// and where it lands — so a shift committed afterwards would name a track the
// reader never chose.
func TestQueuePlayNext_APendingShiftIsDropped(t *testing.T) {
	q := navQueue(20, 5)
	q.playing = 10

	moved, _ := q.Update(keyMsgFor("J"))
	q = moved.(queueContent)
	if _, _, pending := q.pendingMove(); !pending {
		t.Fatal("no shift is waiting, so this proves nothing")
	}

	after, cmd := q.Update(keyMsgFor("n"))
	if _, _, pending := after.(queueContent).pendingMove(); pending {
		t.Fatal("a shift survived a played-next move that shifts positions under it")
	}
	if !noticed(cmd, queueMoveAbandoned) {
		t.Fatal("the reader was not told the shift was dropped")
	}
}

// playNextOne reports the single move a command asks for, for the cases that
// act on one track. It fails the test if the command asks for several.
func playNextOne(t *testing.T, cmd tea.Cmd) (queueMove, bool) {
	t.Helper()

	sent, ok := playNextSent(cmd)
	if !ok {
		return queueMove{}, false
	}
	if len(sent.moves) != 1 {
		t.Fatalf("the command asks for %d moves, want one", len(sent.moves))
	}
	return sent.moves[0], true
}

// playNextSent reports the queuePlayNextMsg a command asks for, unpacking a
// batch the way noticed does for a notice.
func playNextSent(cmd tea.Cmd) (queuePlayNextMsg, bool) {
	if cmd == nil {
		return queuePlayNextMsg{}, false
	}
	switch msg := cmd().(type) {
	case queuePlayNextMsg:
		return msg, true
	case tea.BatchMsg:
		for _, branch := range msg {
			if branch == nil {
				continue
			}
			if sent, ok := playNextSent(branch); ok {
				return sent, true
			}
		}
	}
	return queuePlayNextMsg{}, false
}

// deliverQueueWithPlaying is deliverQueue with a playing position, for cases
// that need a current track to answer "next" about.
func deliverQueueWithPlaying(t *testing.T, items, playing int) (appModel, tea.Cmd) {
	t.Helper()

	m := pressKey(sizedModel(t), "enter")
	speaker, ok := m.activeSpeaker()
	if !ok {
		t.Fatal("no speaker on screen")
	}

	updated, cmd := m.Update(queueLoadedMsg{
		device:  speaker.device,
		room:    speaker.device.RoomName(),
		items:   queueItems(items),
		playing: playing,
	})
	return updated.(appModel), cmd
}

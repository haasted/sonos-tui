package main

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Case: one notch of the wheel. It moves the cursor exactly as the arrow key
// does, scrolling the list with it.
func TestQueueWheel_ANotchIsAnArrowPress(t *testing.T) {
	q := navQueue(40, 10)

	byWheel := notch(q, tea.MouseWheelDown)
	byKey := moveQueue(q, "down")

	if byWheel.cursor != byKey.cursor || byWheel.offset != byKey.offset {
		t.Fatalf("the wheel put the cursor on %d (offset %d), the arrow on %d (offset %d)",
			byWheel.cursor, byWheel.offset, byKey.cursor, byKey.offset)
	}
	if up := notch(q, tea.MouseWheelUp); up.cursor != 9 {
		t.Fatalf("a notch up put the cursor on %d, want 9", up.cursor)
	}
}

// Case: a burst of notches from a flicked trackpad. Those arriving inside the
// step are dropped, so the cursor stops when the fingers do.
func TestQueueWheel_ABurstIsThinnedOut(t *testing.T) {
	q := notch(navQueue(40, 10), tea.MouseWheelDown)

	q = notch(q, tea.MouseWheelDown)
	if q.cursor != 11 {
		t.Fatalf("a notch inside the step moved the cursor to %d", q.cursor)
	}

	q.wheeledAt = time.Now().Add(-listWheelStep)
	q = notch(q, tea.MouseWheelDown)
	if q.cursor != 12 {
		t.Fatalf("a notch after the step did not move the cursor; it is on %d", q.cursor)
	}
}

// Case: one swipe into the end of the list, whose momentum keeps sending
// notches long after the pulse it launched has run out. One swipe, one
// pulse; only the next swipe lights the end again.
func TestQueueWheel_OneSwipeLightsTheEndOnce(t *testing.T) {
	q := notch(navQueue(40, 39), tea.MouseWheelDown)
	if !q.bumping() {
		t.Fatal("scrolling into the end did not light it")
	}
	launched := q.bumpedAt

	q = sustained(q, tea.MouseWheelDown)
	if !q.bumpedAt.Equal(launched) {
		t.Fatal("a notch relaunched a pulse that was still running")
	}

	q.bumpedAt = time.Now().Add(-bumpDuration)
	q = sustained(q, tea.MouseWheelDown)
	if q.bumping() {
		t.Fatal("the same swipe lit the end a second time once its pulse had run out")
	}

	q.swipe.lastAt = time.Now().Add(-swipeQuietTime - time.Millisecond)
	q = notch(afterAPause(q), tea.MouseWheelDown)
	if !q.bumping() {
		t.Fatal("a new swipe into the end did not light it")
	}
}

// Case: notches the rate limit dropped. They are still the swipe going on, so
// the gesture lasts as long as notches keep arriving, not only those that
// moved the cursor.
func TestQueueWheel_DroppedNotchesKeepTheSwipeGoing(t *testing.T) {
	q := upToSpeed(navQueue(400, 0))

	q.wheeledAt = time.Now().Add(-2 * listWheelRunGap)     // nothing moved for a while...
	q.wheelSeenAt = time.Now().Add(-10 * time.Millisecond) // ...but notches kept arriving
	before := q.cursor
	q = notch(q, tea.MouseWheelDown)

	if q.cursor-before != listWheelMaxRows {
		t.Fatalf("the swipe was taken as over: a notch moved %d rows, want %d",
			q.cursor-before, listWheelMaxRows)
	}
}

// Case: the wheel kept turning. Each notch moves further the longer the run
// goes on, up to the most a notch may move.
func TestQueueWheel_AKeptScrollSpeedsUp(t *testing.T) {
	q := navQueue(400, 0)

	var steps []int
	for range 3 * listWheelRampNotches {
		before := q.cursor
		q = sustained(q, tea.MouseWheelDown)
		steps = append(steps, q.cursor-before)
	}

	for i, step := range steps {
		want := min(1+i/listWheelRampNotches, listWheelMaxRows)
		if step != want {
			t.Fatalf("notch %d moved %d rows, want %d (all: %v)", i, step, want, steps)
		}
	}
}

// Case: a pause in the scroll. The run is over, so the next notch is back to
// one row, and a reader lining up a track is not thrown past it.
func TestQueueWheel_APauseStartsSlowAgain(t *testing.T) {
	q := upToSpeed(navQueue(400, 0))

	before := q.cursor
	q = notch(afterAPause(q), tea.MouseWheelDown)

	if q.cursor-before != 1 {
		t.Fatalf("a notch after a pause moved %d rows, want 1", q.cursor-before)
	}
}

// Case: scrolling back the other way, which is what a reader who overshot
// does. Back to one row, however fast the run before it was.
func TestQueueWheel_ReversingStartsSlowAgain(t *testing.T) {
	q := upToSpeed(navQueue(400, 0))

	before := q.cursor
	q = sustained(q, tea.MouseWheelUp)

	if before-q.cursor != 1 {
		t.Fatalf("the first notch back moved %d rows, want 1", before-q.cursor)
	}
}

// Case: the wheel while the filter box has the keyboard. Down hands focus to
// the list, as the down arrow does there.
func TestQueueWheel_DownLeavesTheFilterBox(t *testing.T) {
	q := navQueue(40, 0).openFilter()

	q = notch(q, tea.MouseWheelDown)

	if q.filtering {
		t.Fatal("a notch down left the filter box holding the keyboard")
	}
}

// Case: a sideways scroll. Nothing in the list moves sideways.
func TestQueueWheel_SidewaysDoesNothing(t *testing.T) {
	q := navQueue(40, 10)

	if moved := notch(q, tea.MouseWheelRight); moved.cursor != 10 {
		t.Fatalf("a sideways scroll moved the cursor to %d", moved.cursor)
	}
}

// Case: the program asking for the mouse. Only while the queue is the topmost
// thing on screen: under the help popup, or over the player, the terminal
// keeps it and the reader keeps their text selection.
func TestQueueWheel_TheMouseIsAskedForOnlyOverTheQueue(t *testing.T) {
	m := sizedModel(t)
	if m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("the player asked for the mouse")
	}

	m = openedQueue(t, 40)
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("the open queue did not ask for the mouse")
	}

	m = pressKey(m, "?")
	if m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("the queue kept the mouse under the help popup")
	}
	before := queueOf(t, m).cursor
	m = deliver(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if queueOf(t, m).cursor != before {
		t.Fatal("a notch over the help popup moved the queue behind it")
	}
}

// Case: the queue hint waiting to show. A notch is the reader at work, as a
// keypress is, and cancels it.
func TestQueueWheel_ANotchCancelsTheHint(t *testing.T) {
	m := openedQueue(t, 40)
	due := listHintDueMsg{gen: m.queueHintGen}

	m = deliver(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m = deliver(m, due)

	if m.queue.NoticeShowing() {
		t.Fatal("the hint showed after the reader had scrolled")
	}
}

// sustained is a notch arriving as part of a continuous scroll: just past the
// rate limit, well inside the gap that would end the run.
func sustained(q queueContent, button tea.MouseButton) queueContent {
	q.wheeledAt = time.Now().Add(-listWheelStep)
	return notch(q, button)
}

// afterAPause is the wheel left still for longer than a swipe's gap.
func afterAPause(q queueContent) queueContent {
	paused := time.Now().Add(-listWheelRunGap - time.Millisecond)
	q.wheeledAt, q.wheelSeenAt = paused, paused
	return q
}

// upToSpeed scrolls down long enough to reach the most a notch may move.
func upToSpeed(q queueContent) queueContent {
	for range listWheelRampNotches * listWheelMaxRows {
		q = sustained(q, tea.MouseWheelDown)
	}
	return q
}

// notch sends one wheel notch to the list, as the modal would.
func notch(q queueContent, button tea.MouseButton) queueContent {
	moved, _ := q.Update(tea.MouseWheelMsg{Button: button})
	return moved.(queueContent)
}

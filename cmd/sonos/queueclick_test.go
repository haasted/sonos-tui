package main

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a click on a row in a scrolled list. The cursor moves to that row,
// and the window stays where it is.
func TestQueueClick_PutsTheCursorOnTheRow(t *testing.T) {
	q := navQueue(40, 20)
	offset := q.offset

	q, cmd := clickLine(q, listHeaderRows+3, time.Now())

	assert.Equal(t, offset+3, q.cursor)
	assert.Equal(t, offset, q.offset, "the window moved")
	assert.Nil(t, cmd, "a single click asked for something")
}

// Case: two clicks on one row inside the double-click time. The second plays
// from that track, as enter does.
func TestQueueClick_ADoubleClickPlaysFromTheRow(t *testing.T) {
	at := time.Now()
	q, _ := clickLine(navQueue(40, 0), listHeaderRows+4, at)

	_, cmd := clickLine(q, listHeaderRows+4, at.Add(doubleClickTime))

	require.NotNil(t, cmd, "the double-click asked for nothing")
	assert.Equal(t, queuePlayFromMsg{position: 5}, cmd())
}

// Case: second clicks that do not make a double-click: on another row, after
// the double-click time, and a third click straight after a double-click.
func TestQueueClick_NotADoubleClick(t *testing.T) {
	at := time.Now()
	first, _ := clickLine(navQueue(40, 0), listHeaderRows+4, at)

	_, cmd := clickLine(first, listHeaderRows+5, at.Add(time.Millisecond))
	assert.Nil(t, cmd, "a click on another row played")

	_, cmd = clickLine(first, listHeaderRows+4, at.Add(doubleClickTime+time.Millisecond))
	assert.Nil(t, cmd, "a slow second click played")

	doubled, _ := clickLine(first, listHeaderRows+4, at.Add(time.Millisecond))
	_, cmd = clickLine(doubled, listHeaderRows+4, at.Add(2*time.Millisecond))
	assert.Nil(t, cmd, "a third click played again")
}

// Case: clicks on the heading, on the rule, below the last row of a short
// list, and on a list still loading. None moves the cursor.
func TestQueueClick_OffTheRowsDoesNothing(t *testing.T) {
	for name, line := range map[string]int{"heading": 0, "rule": 1, "below the rows": listHeaderRows + 5} {
		q, _ := clickLine(navQueue(5, 2), line, time.Now())
		assert.Equal(t, 2, q.cursor, name)
	}

	loading := navQueue(5, 2)
	loading.loading = true
	q, _ := clickLine(loading, listHeaderRows, time.Now())
	assert.Equal(t, 2, q.cursor, "a loading list")
}

// Case: a click while the filter box has the keyboard. The reader has moved
// into the list, as down does there, and the narrowing stays.
func TestQueueClick_LeavesTheFilterBox(t *testing.T) {
	q := moveQueue(navQueue(40, 0).openFilter(), "1")

	q, _ = clickLine(q, listHeaderRows+1, time.Now())

	assert.False(t, q.filtering, "the filter box kept the keyboard")
	assert.Equal(t, "1", q.filter)
	assert.Equal(t, 1, q.cursor)
}

// Case: a click on the screen cell a track is drawn in, the whole way from
// the terminal's row. The cursor lands on that track, and a double-click
// there plays it.
func TestQueueClick_FromTheTerminalsRowToTheTrack(t *testing.T) {
	m, _ := deliverQueue(t, 40)
	x, y := screenCellOf(t, m, "Track 7 ")

	m = deliver(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	assert.Equal(t, 6, queueOf(t, m).cursor)

	_, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	assert.Contains(t, flatten(cmd), tea.Msg(queuePlayFromMsg{position: 7}))
}

// Case: clicks the queue must not take: on its border, with the right
// button, and on a track hidden under the help popup.
func TestQueueClick_IgnoredOffTheListOrUnderAnotherModal(t *testing.T) {
	m, _ := deliverQueue(t, 40)
	x, y := screenCellOf(t, m, "Track 7 ")
	_, border := screenCellOf(t, m, "╰")

	for name, click := range map[string]tea.MouseClickMsg{
		"border":       {X: x, Y: border, Button: tea.MouseLeft},
		"right button": {X: x, Y: y, Button: tea.MouseRight},
	} {
		assert.Equal(t, 0, queueOf(t, deliver(m, click)).cursor, name)
	}

	m = deliver(pressKey(m, "?"), tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	assert.Equal(t, 0, queueOf(t, m).cursor, "under the help popup")
}

// Case: the queue hint waiting to show. A click is the reader at work, as a
// keypress is, and cancels it.
func TestQueueClick_CancelsTheHint(t *testing.T) {
	m := openedQueue(t, 40)
	due := listHintDueMsg{gen: m.queueHintGen}
	x, y := screenCellOf(t, m, "Track 3 ")

	m = deliver(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m = deliver(m, due)

	assert.False(t, m.queue.NoticeShowing(), "the hint showed after the reader clicked")
}

// Case: the frame's top row on screen. It starts where the cursor was, moves
// up as far as the terminal scrolls to make room for the frame, and never
// comes back down.
func TestQueueClick_TheFrameTopFollowsTheScroll(t *testing.T) {
	m := sizedModel(t)
	m.screenTop = 3
	m = deliver(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	assert.Equal(t, 3, m.screenTop, "a frame with room below it moved")

	m.screenTop = 21
	m = deliver(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	assert.Equal(t, 24-playerHeightRows, m.screenTop, "the player's scroll was missed")

	m = pressKey(m, "enter")
	require.Greater(t, m.frameHeight(), playerHeightRows, "the queue did not grow the frame")
	assert.Equal(t, 24-m.frameHeight(), m.screenTop, "the queue's scroll was missed")

	m = pressKey(m, "esc")
	assert.Equal(t, 24-m.frameHeight(), m.screenTop, "the top came back down")
}

// clickLine sends a click on a line of the queue's content, as the modal would.
func clickLine(q queueContent, line int, at time.Time) (queueContent, tea.Cmd) {
	updated, cmd := q.Update(listClickMsg{line: line, at: at})
	return updated.(queueContent), cmd
}

// screenCellOf is the terminal cell where text is first drawn in the frame.
func screenCellOf(t *testing.T, m appModel, text string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if at := strings.Index(line, text); at >= 0 {
			return ansi.StringWidth(line[:at]), m.screenTop + y
		}
	}
	t.Fatalf("%q is not drawn", text)
	return 0, 0
}

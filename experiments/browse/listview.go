package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// A list inside a modal: a heading, a rule, and rows with a cursor. The queue
// and the favourites are both one. What a row says, and what a key does to the
// row under the cursor, stay with the content that holds the list.

const (
	listHeaderRows = 2 // the heading and the rule below it
	listMarkWidth  = 2 // the cursor and the space after it
	listArrowWidth = 1 // the scroll arrow at the right edge of a row
	listGap        = 2 // between one column and the next

	// The floors the two columns keep when the modal cannot hold them both.
	// Below these a column stops carrying information and only carries an
	// ellipsis.
	listFirstMin  = 20
	listSecondMin = 12

	// listPulseInterval is how often the arrows repaint. A breath takes five
	// seconds, so twenty frames a second is more than the eye asks for.
	listPulseInterval = 50 * time.Millisecond

	// bumpDuration is how long the edge stays lit after the reader presses
	// against it. Short: it answers a keypress and then gets out of the way.
	bumpDuration = 320 * time.Millisecond

	// listWheelStep is the least time between two rows moved by the wheel. A
	// flicked trackpad sends notches in a burst; those inside it are dropped,
	// so the cursor stops when the fingers do rather than running on.
	listWheelStep = 50 * time.Millisecond

	// A scroll speeds up one row per listWheelRampNotches, up to
	// listWheelMaxRows, and starts slow again after a pause longer than
	// listWheelRunGap or a change of direction.
	listWheelMaxRows     = 3
	listWheelRampNotches = 4
	listWheelRunGap      = 250 * time.Millisecond

	// headingShortest is the fewest characters of a shrinking heading part
	// worth showing. Clipped any shorter, it is dropped instead.
	headingShortest  = 6
	headingSeparator = " · "

	// landingContext is how many rows a landing keeps above the row it lands
	// on. The rest of the window shows what comes after that row.
	landingContext = 2
)

// listCursorGlyph marks the row the reader is on. The cursor moves, and so
// needs a stronger signal than the colour a playing track is drawn in.
const listCursorGlyph = "▶"

// bumpEdge is which edge the reader pressed against: an end of the list, or
// a side, for a list that pages sideways as well.
type bumpEdge int

const (
	bumpNone bumpEdge = iota
	bumpTop
	bumpBottom
	bumpLeft
	bumpRight
)

// listView is the part of a list that knows nothing of what it lists: the
// rows, the cursor among them, the window onto them, and how the reader moves
// it with keys and the wheel.
type listView[T any] struct {
	items []T

	cursor   int       // the row the reader is on, as an index into items
	offset   int       // first visible item
	rows     int       // how many rows the modal will show, its frame excluded
	width    int       // how many columns it will show, likewise
	openedAt time.Time // when the list appeared, which is when the breath began

	edge     bumpEdge  // which end the reader last pressed against
	bumpedAt time.Time // when they did; zero when nothing is lit

	wheeledAt   time.Time // when the wheel last moved the cursor
	wheelSeenAt time.Time // when the last notch arrived, moved or dropped
	wheelRun    int       // notches moved in the current gesture
	wheelDir    int       // which way that gesture is going: -1 up, 1 down
	wheelBumped bool      // the current swipe has lit an end already
	swipe       swipeTracker

	// filter is what the reader has typed, and filtering says the box is open
	// and taking their keys. Open with nothing typed still shows everything,
	// so the two are not one field.
	filter    string
	filtering bool

	// all is the whole list while a filter narrows items to part of it, and
	// nil otherwise. Read it through everything(), never directly.
	all []T
}

// headingPart is one run of a heading, drawn in its own style.
type headingPart struct {
	text   string
	style  lipgloss.Style
	shrink bool // clipped to fit before it is dropped, rather than dropped whole
}

// navigate moves the cursor for the keys every list shares, and reports false
// for any other key. Mostly the keys less and vi established; page up is F
// rather than b, so f and F pair as the lists' other keys do.
func (l listView[T]) navigate(key string) (listView[T], bool) {
	switch key {
	case "up", "k":
		return l.moveBy(-1), true
	case "down", "j":
		return l.moveBy(1), true
	case "pgup", "F":
		return l.moveBy(-l.page()), true
	case "pgdown", "f", "space":
		return l.moveBy(l.page()), true
	case "ctrl+u":
		return l.moveBy(-l.halfPage()), true
	case "ctrl+d":
		return l.moveBy(l.halfPage()), true
	case "home", "g":
		return l.moveToEnd(bumpTop), true
	case "end", "G":
		return l.moveToEnd(bumpBottom), true
	}
	return l, false
}

// moveBy moves the cursor by delta rows, stopping at either end. It lights an
// edge only where the cursor could not move at all: a page down that would run
// past the last row lands on it instead, which is no refusal.
//
// Every relative key goes through it, and every absolute one through
// moveToEnd, so what a move means is decided in two functions rather than in
// each of eight bindings.
func (l listView[T]) moveBy(delta int) listView[T] {
	target := max(min(l.cursor+delta, l.lastRow()), 0)
	if target == l.cursor {
		if delta < 0 {
			return l.bump(bumpTop)
		}
		return l.bump(bumpBottom)
	}
	l.cursor = target
	return l.follow()
}

// moveToEnd puts the cursor on the first or the last row. The edge is passed in
// rather than derived from the row it lands on, because a list of one item has
// the same row at both ends and the two keys would then light the same border.
func (l listView[T]) moveToEnd(edge bumpEdge) listView[T] {
	target := 0
	if edge == bumpBottom {
		target = l.lastRow()
	}
	if target == l.cursor {
		return l.bump(edge)
	}
	l.cursor = target
	return l.follow()
}

// wheelStep reads one wheel notch and says how many rows it moves, signed,
// or zero for a notch dropped: no oftener than listWheelStep, and further per
// notch the longer the wheel keeps going.
func (l listView[T]) wheelStep(msg tea.MouseWheelMsg) (listView[T], int) {
	dir := map[tea.MouseButton]int{tea.MouseWheelUp: -1, tea.MouseWheelDown: 1}[msg.Button]
	if dir == 0 {
		return l, 0
	}

	now := time.Now()
	var fresh bool
	if l.swipe, fresh = l.swipe.notch(dir, now); fresh {
		l.wheelBumped = false
	}
	if dir != l.wheelDir || now.Sub(l.wheelSeenAt) > listWheelRunGap {
		l.wheelRun = 0
	}
	l.wheelSeenAt, l.wheelDir = now, dir
	if now.Sub(l.wheeledAt) < listWheelStep {
		return l, 0
	}

	rows := min(1+l.wheelRun/listWheelRampNotches, listWheelMaxRows)
	l.wheelRun, l.wheeledAt = l.wheelRun+1, now
	return l, dir * rows
}

// wheelMove moves the cursor by a step wheelStep allowed. An end lights once
// per swipe, and never on top of a pulse still running out.
func (l listView[T]) wheelMove(delta int) listView[T] {
	edge, bumpedAt, running := l.edge, l.bumpedAt, l.bumping()
	l = l.moveBy(delta)
	if bumped := !l.bumpedAt.Equal(bumpedAt); bumped {
		if l.wheelBumped || (running && l.edge == edge) {
			l.edge, l.bumpedAt = edge, bumpedAt
		}
		l.wheelBumped = true
	}
	return l
}

// follow scrolls the window the least it can to keep the cursor on screen. The
// window is what moves; the cursor is where the reader is.
func (l listView[T]) follow() listView[T] {
	switch {
	case l.cursor < l.offset:
		l.offset = l.cursor
	case l.cursor >= l.offset+l.visibleRows():
		l.offset = l.cursor - l.visibleRows() + 1
	}
	l.offset = max(min(l.offset, l.maxOffset()), 0)
	return l
}

// land scrolls the cursor row to landingContext rows below the top of the
// window, fewer in a short window. At the end of the list the window stops at
// the last row instead, so no row stays empty.
func (l listView[T]) land() listView[T] {
	above := min(landingContext, (l.visibleRows()-1)/2)
	l.offset = max(min(l.cursor-above, l.maxOffset()), 0)
	return l
}

// lastRow is the index of the final item, or zero for a list with none. The
// cursor always sits somewhere, even on a list with nowhere to sit.
func (l listView[T]) lastRow() int { return max(len(l.items)-1, 0) }

// page is how far a page move travels: one whole window, as a pager does. The
// reader keeps their place by the heading, which names the rows on screen.
func (l listView[T]) page() int { return l.visibleRows() }

// halfPage is the move ctrl+u and ctrl+d make, which keeps half the window in
// view and so needs no heading to stay oriented.
func (l listView[T]) halfPage() int { return max(l.visibleRows()/2, 1) }

// bump lights the end the reader pressed against. Every press restarts it, so
// a held key holds the edge lit and it cools once the key is released.
func (l listView[T]) bump(edge bumpEdge) listView[T] {
	l.edge, l.bumpedAt = edge, time.Now()
	return l
}

// bumping reports whether an edge is still lit.
func (l listView[T]) bumping() bool { return l.heat() > 0 }

// heat is how lit the bumped edge is, from 1 at the press to 0 once cooled.
func (l listView[T]) heat() float32 {
	if l.bumpedAt.IsZero() {
		return 0
	}
	left := bumpDuration - time.Since(l.bumpedAt)
	if left <= 0 {
		return 0
	}
	return float32(left) / float32(bumpDuration)
}

// window is the slice of items on screen. It keeps both marker lines in place
// while the list overflows, so the layout does not jump as the reader scrolls.
func (l listView[T]) window() (first, last int) {
	first = min(l.offset, l.maxOffset())
	return first, min(first+l.visibleRows(), len(l.items))
}

// visibleRows is how many items fit. Every row below the heading holds an item,
// so the count does not change with where the reader has scrolled to.
func (l listView[T]) visibleRows() int {
	return max(l.rows-listHeaderRows, 1)
}

// overflows reports whether the list is longer than the window.
func (l listView[T]) overflows() bool {
	return len(l.items) > l.rows-listHeaderRows
}

func (l listView[T]) maxOffset() int {
	return max(len(l.items)-l.visibleRows(), 0)
}

// rangeLabel says which part of the list is on screen.
func (l listView[T]) rangeLabel() string {
	first, last := l.window()
	return fmt.Sprintf("%d-%d of %d", first+1, last, len(l.items))
}

// arrow is the mark at the right edge of one visible row. It sits on the first
// and the last row of the window, and only where the list really goes on past
// that edge. Rows carry it rather than lines of their own, so the list keeps
// one height from the top of a list to the bottom.
func (l listView[T]) arrow(first, last, row int) string {
	switch {
	case row == 0 && first > 0:
		return "↑"
	case row == last-first-1 && last < len(l.items):
		return "↓"
	default:
		return " "
	}
}

// columns is how wide the two columns of a row should be. Neither is
// truncated while both fit. They are measured across the WHOLE list rather
// than the visible window, so the columns do not shift as the reader scrolls.
func (l listView[T]) columns(firstOf, secondOf func(T) string) (first, second int) {
	for _, item := range l.items {
		first = max(first, lipgloss.Width(firstOf(item)))
		second = max(second, lipgloss.Width(secondOf(item)))
	}

	room := l.width - listMarkWidth - listGap
	if l.overflows() {
		room -= listGap + listArrowWidth
	}
	if room <= 0 {
		return first, second
	}
	if first+second <= room {
		// The box is a fixed width, so the row fills it rather than stopping
		// short of the rule above. The first column takes the slack, being the
		// field that most often wants it.
		return room - second, second
	}

	// The second column is the shorter field and the one a reader recognises
	// from less of it, so it keeps its natural width while that fits in half
	// the row. The first takes what is left, which is the larger share.
	second = min(second, max(room/2, listSecondMin))
	return max(room-second, listFirstMin), second
}

// render draws the heading, the rule and either the note or the rows in the
// window. row draws one item, given its index and its scroll arrow.
func (l listView[T]) render(heading []headingPart, note string, row func(index int, item T, arrow string) string) string {
	var b strings.Builder
	for _, part := range heading {
		b.WriteString(part.style.Render(part.text))
	}
	b.WriteString("\n" + listRuleStyle().Render(strings.Repeat("─", l.width)) + "\n")

	if note != "" {
		b.WriteString(listNoteStyle().Render(note))
		return b.String()
	}

	first, last := l.window()
	for i, item := range l.items[first:last] {
		b.WriteString(row(first+i, item, l.arrow(first, last, i)) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// drawRow lays out one row: the cursor mark, two columns at the given widths,
// and the scroll arrow. style colours the row; the arrow carries its own.
func (l listView[T]) drawRow(index int, first, second string, widths [2]int, style lipgloss.Style, arrow string, hideCursor bool) string {
	mark := " "
	if index == l.cursor && !hideCursor {
		mark = listCursorGlyph
	}
	body := fmt.Sprintf("%s %s  %s", mark, padRight(clip(first, widths[0]), widths[0]), clip(second, widths[1]))

	// The arrow column exists only where the list overflows, so a short list
	// draws no trailing space and the modal still shrinks to it.
	if !l.overflows() {
		return style.Render(body)
	}
	full := listMarkWidth + widths[0] + listGap + widths[1]
	return style.Render(padRight(body, full)) + strings.Repeat(" ", listGap) + listArrowStyle(l.openedAt).Render(arrow)
}

// borderLight is the pulse that answers a press against an end of the list. It
// leaves the centre of that border and runs out to both corners, which is the
// language the notice entrance already speaks. Nil when nothing is lit, so the
// modal draws its own resting colour.
func (l listView[T]) borderLight() panes.BorderLight {
	heat := l.heat()
	if heat <= 0 {
		return nil
	}
	progress := 1 - heat

	return func(edge panes.ModalEdge, fromCentre float32) panes.BorderCell {
		if !l.lights(edge) {
			return panes.BorderCell{}
		}

		lit := flashsweep.Spread(fromCentre, progress)
		if lit < bumpVisibleAt {
			return panes.BorderCell{} // the modal keeps its own border here
		}
		return panes.BorderCell{Color: bumpColour(lit), Heavy: lit >= bumpHeavyAt}
	}
}

// lights reports whether a border carries the pulse. The edge the reader
// pressed against is the one that answers.
func (l listView[T]) lights(edge panes.ModalEdge) bool {
	return (l.edge == bumpTop && edge == panes.ModalTop) ||
		(l.edge == bumpBottom && edge == panes.ModalBottom) ||
		(l.edge == bumpLeft && edge == panes.ModalLeft) ||
		(l.edge == bumpRight && edge == panes.ModalRight)
}

// fitHeading drops what a narrow box cannot hold, last part first. The parts
// that say where the reader is come first, so the rest is what goes rather
// than anything being cut in half. A last part marked shrink is clipped first.
func fitHeading(parts []headingPart, width int) []headingPart {
	total := func(parts []headingPart) int {
		var sum int
		for _, part := range parts {
			sum += lipgloss.Width(part.text)
		}
		return sum
	}

	if last := len(parts) - 1; width > 0 && parts[last].shrink && total(parts) > width {
		room := width - total(parts[:last]) - lipgloss.Width(headingSeparator)
		if room > headingShortest { // the ellipsis takes one of the columns
			name := strings.TrimPrefix(parts[last].text, headingSeparator)
			parts[last].text = headingSeparator + clip(name, room)
		}
	}
	for width > 0 && len(parts) > 1 && total(parts) > width {
		parts = parts[:len(parts)-1]
	}
	if width > 0 && total(parts) > width {
		parts[0].text = clip(parts[0].text, width)
	}
	return parts
}

// clip shortens to a display width, marking the cut. It walks runes rather
// than bytes: track titles carry accents, and half a rune is not a character.
func clip(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	kept, total := []rune(nil), 0
	for _, r := range s {
		w := lipgloss.Width(string(r))
		if total+w > width-1 {
			break
		}
		kept, total = append(kept, r), total+w
	}
	return string(kept) + "…"
}

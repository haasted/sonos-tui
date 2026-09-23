package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

const (
	queueHeaderRows = 2 // the room line and the rule below it
	queueMarkWidth  = 2 // the cursor and the space after it
	queueArrowWidth = 1 // the scroll arrow at the right edge of a row
	queueGap        = 2 // between one column and the next

	// The floors the two columns keep when the modal cannot hold them both.
	// Below these a column stops carrying information and only carries an
	// ellipsis.
	queueTitleMin  = 20
	queueArtistMin = 12

	// queuePulseInterval is how often the arrows repaint. A breath takes five
	// seconds, so twenty frames a second is more than the eye asks for.
	queuePulseInterval = 50 * time.Millisecond

	// bumpDuration is how long the edge stays lit after the reader presses
	// against it. Short: it answers a keypress and then gets out of the way.
	bumpDuration = 320 * time.Millisecond
)

// queueCursorGlyph marks the row the reader is on. It used to mark the playing
// track, which now shows as colour alone: the cursor moves and so needs the
// stronger signal of the two.
const queueCursorGlyph = "▶"

// bumpEdge is which end of the list the reader pressed against.
type bumpEdge int

const (
	bumpNone bumpEdge = iota
	bumpTop
	bumpBottom
)

// queueContent lists a speaker's queue inside a modal. It holds what it was
// given and where the reader has scrolled to; it fetches nothing itself.
type queueContent struct {
	room    string
	items   []sonos.QueueItem
	playing int
	err     error
	loading bool

	cursor   int       // the row the reader is on, as an index into items
	offset   int       // first visible item
	rows     int       // how many rows the modal will show, its frame excluded
	width    int       // how many columns it will show, likewise
	openedAt time.Time // when the list appeared, which is when the breath began

	edge     bumpEdge  // which end the reader last pressed against
	bumpedAt time.Time // when they did; zero when nothing is lit

	// moveFrom is where the moving track sat when the window opened, 1-based.
	// It is what the speaker is told, since the rows the reader now sees exist
	// only here. Zero when no move is pending.
	moveFrom int
	moveGen  int // id of the open commit window, matched against arriving ticks

	// settling is the track a committed move is waiting to see land: the queue
	// the speaker sends back is what says where it really went. Its URI rather
	// than its position, because the position is precisely what changed.
	settling   string
	settlingAt time.Time
}

var _ tea.Model = queueContent{}

func (queueContent) Init() tea.Cmd { return nil }

// Update scrolls the list. The modal forwards every key except esc, so these
// keys mean the queue here and the player everywhere else: j is the previous
// track outside this list and one row down inside it.
//
// The keys are the ones charm.land/bubbles binds by default, which are the
// ones less and vi established.
func (q queueContent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return q, nil
	}

	switch key.String() {
	case "up", "k":
		q = q.moveBy(-1)
	case "down", "j":
		q = q.moveBy(1)
	case "pgup", "b":
		q = q.moveBy(-q.page())
	case "pgdown", "f", "space":
		q = q.moveBy(q.page())
	case "ctrl+u":
		q = q.moveBy(-q.halfPage())
	case "ctrl+d":
		q = q.moveBy(q.halfPage())
	case "home", "g":
		q = q.moveToEnd(bumpTop)
	case "end", "G":
		q = q.moveToEnd(bumpBottom)
	case "c":
		q = q.jumpToPlaying()
	case "x", "delete":
		return q.remove()
	case "n":
		return q.playNext()
	case "K", "shift+up":
		return q.shift(-1)
	case "J", "shift+down":
		return q.shift(1)
	}
	return q, nil
}

// shift moves the track under the cursor one place and keeps the cursor on it.
//
// THIS IS THE ONE PLACE THE LIST EDITS ITSELF, and it is deliberate. Everywhere
// else the speaker is told and the ContentDirectory event brings the new queue
// back. A move cannot wait for that: the track has to follow the key on every
// press or the key feels broken, so for as long as the window below is open the
// rows on screen are the reader's and not the speaker's.
func (q queueContent) shift(delta int) (queueContent, tea.Cmd) {
	// A move has been sent and the queue it produced has not arrived. Every
	// position held here still describes the queue from before it, so a second
	// move built on them would name the wrong track. Wait: the list is about to
	// be replaced, and moving rows that are about to be thrown away is what
	// makes tracks appear to jump.
	if q.settlingMove() {
		return q, nil
	}

	target := q.cursor + delta
	if q.cursor >= len(q.items) || target < 0 || target >= len(q.items) {
		if delta < 0 {
			return q.bump(bumpTop), nil
		}
		return q.bump(bumpBottom), nil
	}

	// Where the track started, remembered once. A run of presses is one move
	// from there, and the rows it passed over never existed for the speaker.
	if q.moveFrom == 0 {
		q.moveFrom = q.items[q.cursor].Position
	}

	q.items = movedItems(q.items, q.cursor, target)
	q.cursor = target
	q = q.follow()

	if q.moveGen != 0 {
		return q, nil // a tick is already pending; this press joins that window
	}
	q.moveGen = nextQueueGen()
	return q, queueMoveTick(q.moveGen)
}

// movedItems is the list with one item lifted out and put back at another
// index. It copies rather than reordering in place: the slice it is given is
// shared with the Device that read it, and editing that would change what
// every other reader of the queue sees.
func movedItems(items []sonos.QueueItem, from, to int) []sonos.QueueItem {
	moved := make([]sonos.QueueItem, 0, len(items))
	moved = append(moved, items[:from]...)
	moved = append(moved, items[from+1:]...)

	rest := make([]sonos.QueueItem, 0, len(items))
	rest = append(rest, moved[:to]...)
	rest = append(rest, items[from])
	return append(rest, moved[to:]...)
}

// pendingMove is the move waiting to be sent, and whether there is one. to is
// the position the track now appears at, which is what the speaker is asked
// for.
func (q queueContent) pendingMove() (from, to int, pending bool) {
	if q.moveFrom == 0 || q.cursor >= len(q.items) {
		return 0, 0, false
	}
	return q.moveFrom, q.cursor + 1, true
}

// clearMove closes the window, whether it was committed or abandoned.
func (q queueContent) clearMove() queueContent {
	q.moveFrom, q.moveGen = 0, 0
	return q
}

// settleMove records that a move has gone to the speaker and names the track
// to look for in the queue that comes back.
func (q queueContent) settleMove(uri string) queueContent {
	q.settling, q.settlingAt = uri, time.Now()
	return q
}

// settlingMove reports a move still waiting for its queue. It expires, because
// the event that would end it can be dropped -- Device says so when its channel
// fills -- and a reader must not be left unable to move anything.
func (q queueContent) settlingMove() bool {
	return q.settling != "" && time.Since(q.settlingAt) < queueMoveSettle
}

// followSettled puts the cursor back on the track a move sent away, now that
// the speaker has said where it went. The reader was watching that track, not
// that row.
func (q queueContent) followSettled() queueContent {
	uri := q.settling
	q.settling, q.settlingAt = "", time.Time{}
	if uri == "" {
		return q
	}

	for i, item := range q.items {
		if item.URI == uri {
			q.cursor = i
			return q.follow()
		}
	}
	return q
}

// remove asks for the track under the cursor to go. The list does not do it
// itself and does not know the speaker: it says which track, and whoever holds
// the device acts on it.
//
// The cursor stays where it is. Once the shorter queue arrives the row below
// has moved up into it, which is what a reader clearing several in a row
// wants; at the end of the list the cursor is clamped back by one.
//
// A move still waiting to be sent is dropped first. Both commands address
// tracks by position, and a removal shifts every position below it -- so a move
// committed afterwards would name a track the reader never chose.
func (q queueContent) remove() (queueContent, tea.Cmd) {
	if q.cursor >= len(q.items) {
		return q, nil // an empty queue, or one still being read
	}

	position := q.items[q.cursor].Position

	var dropped tea.Cmd
	if _, _, pending := q.pendingMove(); pending {
		q = q.clearMove()
		dropped = notify(panes.NoticeWarn, queueMoveAbandoned)
	}

	ask := func() tea.Msg { return queueRemoveMsg{position: position} }
	return q, tea.Batch(ask, dropped)
}

// playNext asks for the track under the cursor to move directly after the one
// playing, so it is the next thing heard. Unlike shift's debounced window this
// is decided and sent in one step: there is no run of presses to gather, so
// the list waits for the ContentDirectory event like remove does.
//
// The cursor stays where it is, exactly as remove leaves it: once the
// reordered queue arrives, the track that was beneath the one that moved has
// slid up into this row. At the last row, which has nothing beneath it, the
// same clamp remove relies on lands it on the track that was above instead.
func (q queueContent) playNext() (queueContent, tea.Cmd) {
	if q.cursor >= len(q.items) {
		return q, nil // an empty queue, or one still being read
	}

	// A station plays past the queue without touching it. DeviceState.Track
	// then counts the stream, so playing is left at zero: there is no current
	// track to be "next" of, even though the queue behind it is right there.
	if q.playing <= 0 {
		return q, notify(panes.NoticeInfo, queuePlayNextNoTrack)
	}

	track := q.items[q.cursor]
	switch track.Position {
	case q.playing:
		return q, notify(panes.NoticeInfo, queueAlreadyPlaying)
	case q.playing + 1:
		return q, notify(panes.NoticeInfo, queueAlreadyNext)
	}

	// to is where the track should end up once it is lifted out, in the
	// numbering that leaves it in -- see MoveQueueTrack. Lifting a track from
	// above the playing one shifts the playing one up by one place first.
	to := q.playing + 1
	if track.Position < q.playing {
		to = q.playing
	}

	var dropped tea.Cmd
	if _, _, pending := q.pendingMove(); pending {
		q = q.clearMove()
		dropped = notify(panes.NoticeWarn, queueMoveAbandoned)
	}

	from := track.Position
	ask := func() tea.Msg { return queuePlayNextMsg{from: from, to: to} }
	return q, tea.Batch(ask, dropped)
}

// moveBy moves the cursor by delta rows, stopping at either end. It lights an
// edge only where the cursor could not move at all: a page down that would run
// past the last row lands on it instead, which is no refusal.
//
// Every relative key goes through it, and every absolute one through
// moveToEnd, so what a move means is decided in two functions rather than in
// each of eight bindings.
func (q queueContent) moveBy(delta int) queueContent {
	target := max(min(q.cursor+delta, q.lastRow()), 0)
	if target == q.cursor {
		if delta < 0 {
			return q.bump(bumpTop)
		}
		return q.bump(bumpBottom)
	}
	q.cursor = target
	return q.follow()
}

// moveToEnd puts the cursor on the first or the last row. The edge is passed in
// rather than derived from the row it lands on, because a queue of one item has
// the same row at both ends and the two keys would then light the same border.
func (q queueContent) moveToEnd(edge bumpEdge) queueContent {
	target := 0
	if edge == bumpBottom {
		target = q.lastRow()
	}
	if target == q.cursor {
		return q.bump(edge)
	}
	q.cursor = target
	return q.follow()
}

// jumpToPlaying moves the cursor to the track now playing. Pausing or muting
// the speaker does not change which track that is, only stopping the queue
// itself does, so this answers "playing" the same way in every one of those
// states.
func (q queueContent) jumpToPlaying() queueContent {
	if q.playing <= 0 {
		return q
	}
	for i, item := range q.items {
		if item.Position == q.playing {
			q.cursor = i
			return q.follow()
		}
	}
	return q
}

// follow scrolls the window the least it can to keep the cursor on screen. The
// window is what moves; the cursor is where the reader is.
func (q queueContent) follow() queueContent {
	switch {
	case q.cursor < q.offset:
		q.offset = q.cursor
	case q.cursor >= q.offset+q.visibleRows():
		q.offset = q.cursor - q.visibleRows() + 1
	}
	q.offset = max(min(q.offset, q.maxOffset()), 0)
	return q
}

// lastRow is the index of the final item, or zero for a queue with none. The
// cursor always sits somewhere, even on a list with nowhere to sit.
func (q queueContent) lastRow() int { return max(len(q.items)-1, 0) }

// page is how far a page move travels: one whole window, as a pager does. The
// reader keeps their place by the heading, which names the rows on screen.
func (q queueContent) page() int { return q.visibleRows() }

// halfPage is the move ctrl+u and ctrl+d make, which keeps half the window in
// view and so needs no heading to stay oriented.
func (q queueContent) halfPage() int { return max(q.visibleRows()/2, 1) }

// bump lights the end the reader pressed against. Every press restarts it, so
// a held key holds the edge lit and it cools once the key is released.
func (q queueContent) bump(edge bumpEdge) queueContent {
	q.edge, q.bumpedAt = edge, time.Now()
	return q
}

// bumping reports whether an edge is still lit.
func (q queueContent) bumping() bool { return q.heat() > 0 }

// heat is how lit the bumped edge is, from 1 at the press to 0 once cooled.
func (q queueContent) heat() float32 {
	if q.bumpedAt.IsZero() {
		return 0
	}
	left := bumpDuration - time.Since(q.bumpedAt)
	if left <= 0 {
		return 0
	}
	return float32(left) / float32(bumpDuration)
}

func (q queueContent) View() tea.View {
	title, artist := q.columns()

	var b strings.Builder
	b.WriteString(queueRoomStyle().Render(q.heading()) + "\n")
	b.WriteString(queueRuleStyle().Render(strings.Repeat("─", q.ruleWidth(title, artist))) + "\n")

	if note := q.note(); note != "" {
		b.WriteString(queueNoteStyle().Render(note))
		return tea.NewView(b.String())
	}

	first, last := q.window()
	for i, item := range q.items[first:last] {
		b.WriteString(q.row(item, first+i, title, artist, q.arrow(first, last, i)) + "\n")
	}

	return tea.NewView(strings.TrimRight(b.String(), "\n"))
}

// columns is how wide the title and the artist should be. Neither is truncated
// while both fit. They are measured across the WHOLE queue rather than the
// visible window, so the columns do not shift as the reader scrolls.
func (q queueContent) columns() (title, artist int) {
	for _, item := range q.items {
		title = max(title, lipgloss.Width(item.Title))
		artist = max(artist, lipgloss.Width(item.Artist))
	}

	room := q.width - queueMarkWidth - queueGap
	if q.overflows() {
		room -= queueGap + queueArrowWidth
	}
	if room <= 0 || title+artist <= room {
		return title, artist
	}

	// The artist is the shorter field and the one a reader recognises from
	// less of it, so it keeps its natural width while that fits in half the
	// row. The title takes what is left, which is the larger share.
	artist = min(artist, max(room/2, queueArtistMin))
	return max(room-artist, queueTitleMin), artist
}

// ruleWidth spans the widest thing under it, so the rule marks the list out
// rather than stopping short of it or running past.
func (q queueContent) ruleWidth(title, artist int) int {
	widest := max(q.widestHeading(), lipgloss.Width(q.note()))
	if len(q.items) > 0 {
		row := queueMarkWidth + title + queueGap + artist
		if q.overflows() {
			row += queueGap + queueArrowWidth
		}
		widest = max(widest, row)
	}
	return widest
}

// heading names the room and the size of its queue, so a reader knows what
// they are looking at and how much of it they can see.
func (q queueContent) heading() string {
	switch {
	case q.loading, len(q.items) == 0:
		return q.room
	case q.overflows():
		first, last := q.window()
		return q.room + " · " + q.rangeLabel(first, last)
	case len(q.items) == 1:
		return q.room + " · 1 track"
	default:
		return fmt.Sprintf("%s · %d tracks", q.room, len(q.items))
	}
}

// rangeLabel says which part of the queue is on screen.
func (q queueContent) rangeLabel(first, last int) string {
	return fmt.Sprintf("%d-%d of %d", first+1, last, len(q.items))
}

// widestHeading is how wide the heading can ever get for this queue. Both
// numbers in the range only grow, so the last window is the longest one. The
// rule measures against this rather than the heading on screen, which holds
// the box at one width while leaving the heading itself unpadded.
func (q queueContent) widestHeading() int {
	if !q.overflows() {
		return lipgloss.Width(q.heading())
	}
	return lipgloss.Width(q.room + " · " + q.rangeLabel(q.maxOffset(), len(q.items)))
}

// note is the whole body when there are no rows to draw, and empty otherwise.
func (q queueContent) note() string {
	switch {
	case q.loading:
		return "Reading the queue…"
	case q.err != nil:
		return "Could not read the queue: " + q.err.Error()
	case len(q.items) == 0:
		return "The queue is empty."
	default:
		return ""
	}
}

// window is the slice of items on screen. It keeps both marker lines in place
// while the queue overflows, so the layout does not jump as the reader scrolls.
func (q queueContent) window() (first, last int) {
	first = min(q.offset, q.maxOffset())
	return first, min(first+q.visibleRows(), len(q.items))
}

// visibleRows is how many items fit. Every row below the heading holds an item,
// so the count does not change with where the reader has scrolled to.
func (q queueContent) visibleRows() int {
	return max(q.rows-queueHeaderRows, 1)
}

// overflows reports whether the queue is longer than the window.
func (q queueContent) overflows() bool {
	return len(q.items) > q.rows-queueHeaderRows
}

func (q queueContent) maxOffset() int {
	return max(len(q.items)-q.visibleRows(), 0)
}

// arrow is the mark at the right edge of one visible row. It sits on the first
// and the last row of the window, and only where the queue really goes on past
// that edge. Rows carry it rather than lines of their own, so the list keeps
// one height from the top of a queue to the bottom.
func (q queueContent) arrow(first, last, row int) string {
	switch {
	case row == 0 && first > 0:
		return "↑"
	case row == last-first-1 && last < len(q.items):
		return "↓"
	default:
		return " "
	}
}

// row draws one queue item: the title and the artist, and a marker on the one
// that is playing. The speaker's own position is held on the item but not
// drawn — it is what the highlight matches on, and what a later "jump to
// track" will need.
func (q queueContent) row(item sonos.QueueItem, index, title, artist int, arrow string) string {
	mark := " "
	if index == q.cursor {
		mark = queueCursorGlyph
	}

	body := fmt.Sprintf("%s %s  %s", mark,
		padRight(clip(item.Title, title), title),
		clip(item.Artist, artist))

	style := queueRowStyle()
	if q.isPlaying(item) {
		style = queuePlayingStyle()
	}

	// The arrow column exists only where the queue overflows, so a short queue
	// draws no trailing space and the modal still shrinks to it. The arrow
	// carries its own colour, which breathes, so it is styled apart from the row.
	if !q.overflows() {
		return style.Render(body)
	}

	full := queueMarkWidth + title + queueGap + artist
	return style.Render(padRight(body, full)) +
		strings.Repeat(" ", queueGap) +
		queueArrowStyle(q.openedAt).Render(arrow)
}

// isPlaying guards the zero as well as matching the position. A queue counts
// from one, so zero means nothing is playing from it — and an item that
// somehow carries no position must not answer to that.
func (q queueContent) isPlaying(item sonos.QueueItem) bool {
	return q.playing > 0 && item.Position == q.playing
}

// borderLight is the pulse that answers a press against an end of the list. It
// leaves the centre of that border and runs out to both corners, which is the
// language the notice entrance already speaks. Nil when nothing is lit, so the
// modal draws its own resting colour.
func (q queueContent) borderLight() panes.BorderLight {
	heat := q.heat()
	if heat <= 0 {
		return nil
	}
	progress := 1 - heat

	return func(edge panes.ModalEdge, fromCentre float32) panes.BorderCell {
		if !q.lights(edge) {
			return panes.BorderCell{}
		}

		lit := flashsweep.Spread(fromCentre, progress)
		if lit < bumpVisibleAt {
			return panes.BorderCell{} // the modal keeps its own border here
		}
		return panes.BorderCell{Color: bumpColour(lit), Heavy: lit >= bumpHeavyAt}
	}
}

// lights reports whether a border carries the pulse. The end the reader pressed
// against is the one that answers.
func (q queueContent) lights(edge panes.ModalEdge) bool {
	return (q.edge == bumpTop && edge == panes.ModalTop) ||
		(q.edge == bumpBottom && edge == panes.ModalBottom)
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

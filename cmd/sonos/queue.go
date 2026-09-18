package main

import (
	"fmt"
	"image/color"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

const (
	queueHeaderRows = 2 // the room line and the rule below it
	queueMarkWidth  = 2 // the playing marker and the space after it
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

	// bumpVisibleAt is the point below which a rune is left in the border's own
	// colour, rather than painted a shade indistinguishable from it.
	bumpVisibleAt = 0.08

	// bumpHeavyAt is where the line thickens. Colour alone spreads the pulse
	// into a smear of close shades; the weight gives it an edge the eye finds
	// without looking for it.
	bumpHeavyAt = 0.35
)

// bumpEdge is which end of the list the reader pressed against.
type bumpEdge int

const (
	bumpNone bumpEdge = iota
	bumpTop
	bumpBottom
)

// queueTickMsg is one frame of the arrows' breath. The generation dies with the
// modal, so a chain cannot outlive the list that began it.
type queueTickMsg struct{ gen int }

var queueGenerations atomic.Int64

// nextQueueGen returns a generation unique across the process.
func nextQueueGen() int { return int(queueGenerations.Add(1)) }

func queueTick(gen int) tea.Cmd {
	return tea.Tick(queuePulseInterval, func(time.Time) tea.Msg {
		return queueTickMsg{gen: gen}
	})
}

// queueLoadedMsg carries a speaker's queue back to the modal that asked for it.
type queueLoadedMsg struct {
	device  *sonos.Device
	room    string
	items   []sonos.QueueItem
	playing int // 1-based position now playing, or 0 when the queue is not the source
	err     error
}

// queueContent lists a speaker's queue inside a modal. It holds what it was
// given and where the reader has scrolled to; it fetches nothing itself.
type queueContent struct {
	room    string
	items   []sonos.QueueItem
	playing int
	err     error
	loading bool

	offset   int       // first visible item
	rows     int       // how many rows the modal will show, its frame excluded
	width    int       // how many columns it will show, likewise
	openedAt time.Time // when the list appeared, which is when the breath began

	edge     bumpEdge  // which end the reader last pressed against
	bumpedAt time.Time // when they did; zero when nothing is lit
}

var _ tea.Model = queueContent{}

func (queueContent) Init() tea.Cmd { return nil }

// Update scrolls the list. The modal forwards every key except esc, so up and
// down mean the queue here and the volume everywhere else.
func (q queueContent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return q, nil
	}

	switch key.String() {
	case "up":
		if q.offset == 0 {
			return q.bump(bumpTop), nil
		}
		q.offset--
	case "down":
		if q.offset >= q.maxOffset() {
			return q.bump(bumpBottom), nil
		}
		q.offset++
	}
	return q, nil
}

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
		b.WriteString(q.row(item, title, artist, q.arrow(first, last, i)) + "\n")
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
func (q queueContent) row(item sonos.QueueItem, title, artist int, arrow string) string {
	mark := " "
	if q.isPlaying(item) {
		mark = "▶"
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

// bumpColour is one rune of the pulse: the scheme's own base at the faint end
// of the light, and white at its front.
//
// The blend never passes through the border's resting colour. That colour is
// amber, and a gradient from amber to blue crosses grey — the pulse would read
// as the border going muddy rather than as a light on it. Below bumpVisibleAt
// the rune is left alone instead, so the handover is a step and not a smear.
func bumpColour(lit float32) color.Color {
	base := flashsweep.FromColor(themedBumpBase())
	peak := flashsweep.FromColor(themedBumpPeak())

	mixed := base.Lerp(peak, smoothstep01(bumpVisibleAt, 1, lit))
	return lipgloss.Color(mixed.ToRgb().String())
}

// smoothstep01 eases between two edges, so the front of the pulse rounds into
// white rather than arriving at it in a straight line.
func smoothstep01(edge0, edge1, x float32) float32 {
	t := min(max((x-edge0)/(edge1-edge0), 0), 1)
	return t * t * (3 - 2*t)
}

// openQueue asks the selected speaker for its queue. Every open fetches again:
// the queue changes underneath us, and a stale list is worse than a short wait.
func (m appModel) openQueue() (tea.Model, tea.Cmd) {
	speaker, ok := m.activeSpeaker()
	if !ok {
		return m, nil
	}

	m.queue = m.queue.Open()
	m.queue.Content = queueContent{
		room:     speaker.device.RoomName(),
		loading:  true,
		rows:     m.queue.ContentRows(),
		width:    m.queue.ContentColumns(),
		openedAt: time.Now(),
	}
	return m, readQueue(speaker.device)
}

func readQueue(device *sonos.Device) tea.Cmd {
	return func() tea.Msg {
		items, err := device.Queue()
		if err != nil {
			slog.Warn("reading queue for the modal", "speaker", device.RoomName(), "err", err)
			return queueLoadedMsg{device: device, room: device.RoomName(), err: err}
		}

		return queueLoadedMsg{
			device:  device,
			room:    device.RoomName(),
			items:   items,
			playing: playingPosition(device.State()),
		}
	}
}

// playingPosition is the queue position now playing, or zero when nothing in
// the queue is. A station plays straight past the queue without touching it,
// and DeviceState.Track then counts the stream — so highlighting item 1 would
// point confidently at the wrong track.
func playingPosition(state sonos.DeviceState) int {
	if state.Source.Kind() != sonos.SourceTrack {
		return 0
	}
	return state.Track
}

// applyQueueLoaded fills the open modal in. A result for a speaker other than
// the one on screen is dropped rather than shown under the wrong room name.
func (m appModel) applyQueueLoaded(msg queueLoadedMsg) (tea.Model, tea.Cmd) {
	content, ok := m.queue.Content.(queueContent)
	if !ok || !m.queue.IsOpen() {
		return m, nil
	}
	if speaker, found := m.activeSpeaker(); !found || speaker.device != msg.device {
		return m, nil
	}

	content.room = msg.room
	content.items = msg.items
	content.playing = msg.playing
	content.err = msg.err
	content.loading = false
	content.rows = m.queue.ContentRows()
	content.width = m.queue.ContentColumns()

	m.queue.Content = content

	// The breath starts only once there is an arrow to breathe. A queue that
	// fits draws none, and a chain for it would repaint a still picture.
	if !content.overflows() {
		return m, nil
	}
	m.queueGen = nextQueueGen()
	return m, queueTick(m.queueGen)
}

// applyQueueTick repaints the arrows. It re-arms only while the same modal is
// open and still has an arrow, so the chain cannot outlive either.
func (m appModel) applyQueueTick(msg queueTickMsg) (tea.Model, tea.Cmd) {
	content, ok := m.queue.Content.(queueContent)
	if msg.gen != m.queueGen || !m.queue.IsOpen() || !ok {
		return m, nil
	}

	// The arrows breathe while the queue overflows, and a bumped edge cools
	// whether it does or not. Neither is true: nothing on screen is moving.
	if !content.overflows() && !content.bumping() {
		return m, nil
	}
	return m, queueTick(msg.gen)
}

// restartQueueChain begins a fresh repaint chain and abandons any running one.
// A bump on a queue that fits has no chain to ride, and taking a generation
// each time is what keeps exactly one of them alive.
func (m appModel) restartQueueChain() (appModel, tea.Cmd) {
	m.queueGen = nextQueueGen()
	return m, queueTick(m.queueGen)
}

// activeSpeaker is the speaker whose tab is on screen.
func (m appModel) activeSpeaker() (speakerModel, bool) {
	if m.tabs.Active >= len(m.tabs.Items) {
		return speakerModel{}, false
	}
	speaker, ok := m.tabs.Items[m.tabs.Active].Content.(speakerModel)
	return speaker, ok
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

func queueRoomStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).
		Foreground(theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24")))
}

func queueRuleStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.LightDark(lipgloss.Color("#D0D0D0"), lipgloss.Color("#4A4A4A")))
}

func queueRowStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.LightDark(lipgloss.Color("#1F2937"), lipgloss.Color("#E5E7EB")))
}

func queuePlayingStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).
		Foreground(theme.LightDark(lipgloss.Color("#1D4ED8"), lipgloss.Color("#93C5FD")))
}

// queueArrowStyle breathes the arrow from near the colour of the rule to the
// far end of the palette. A wide swing on purpose: the arrow is one character,
// so a narrow one is only found by a reader who already knows to look.
func queueArrowStyle(since time.Time) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(flashsweep.BreathAt(time.Since(since), themedArrowRest(), themedArrowLit()))
}

func themedArrowRest() color.Color {
	return theme.LightDark(lipgloss.Color("#C4C4C4"), lipgloss.Color("#5A5A5A"))
}

// themedArrowLit brightens on a dark ground and deepens on a light one. The
// same pair the notice entrance uses, for the same reason: a light that flashes
// toward white on white erases itself.
func themedArrowLit() color.Color {
	return theme.LightDark(lipgloss.Color("#0F172A"), lipgloss.Color("#FFFFFF"))
}

func themedBorder() color.Color {
	return theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24"))
}

// themedBumpBase and themedBumpPeak are the pulse's two colours. Sky into white
// on a dark ground, and the same pair inverted on a light one, where a white
// front would vanish into the page.
func themedBumpBase() color.Color {
	return theme.LightDark(lipgloss.Color("#0369A1"), lipgloss.Color("#38BDF8"))
}

func themedBumpPeak() color.Color {
	return theme.LightDark(lipgloss.Color("#0F172A"), lipgloss.Color("#FFFFFF"))
}

func queueNoteStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))
}

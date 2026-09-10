package main

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

const (
	queueHeaderRows = 2 // the room line and the rule below it
	queueMarkerRows = 2 // the "more above" and "more below" lines
	queueMarkWidth  = 2 // the playing marker and the space after it
	queueGap        = 2 // between the title and the artist

	// The floors the two columns keep when the modal cannot hold them both.
	// Below these a column stops carrying information and only carries an
	// ellipsis.
	queueTitleMin  = 20
	queueArtistMin = 12
)

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

	offset int // first visible item
	rows   int // how many rows the modal will show, its frame excluded
	width  int // how many columns it will show, likewise
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
		q.offset = max(q.offset-1, 0)
	case "down":
		q.offset = min(q.offset+1, q.maxOffset())
	}
	return q, nil
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
	if q.overflows() {
		b.WriteString(q.marker(first, "↑") + "\n")
	}
	for _, item := range q.items[first:last] {
		b.WriteString(q.row(item, title, artist) + "\n")
	}
	if q.overflows() {
		b.WriteString(q.marker(len(q.items)-last, "↓"))
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
	widest := max(lipgloss.Width(q.heading()), lipgloss.Width(q.note()))
	if len(q.items) > 0 {
		widest = max(widest, queueMarkWidth+title+queueGap+artist)
	}
	return widest
}

// heading names the room and the size of its queue, so a reader knows what
// they are looking at and how much of it they can see.
func (q queueContent) heading() string {
	switch {
	case q.loading:
		return q.room
	case len(q.items) == 1:
		return q.room + " · 1 track"
	default:
		return fmt.Sprintf("%s · %d tracks", q.room, len(q.items))
	}
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

// visibleRows is how many items fit. A queue that overflows spends two of its
// rows on the markers; one that fits keeps them, so a short list has no gap
// above and below it.
func (q queueContent) visibleRows() int {
	room := q.rows - queueHeaderRows
	if len(q.items) > room {
		room -= queueMarkerRows
	}
	return max(room, 1)
}

// overflows reports whether the queue is longer than the window.
func (q queueContent) overflows() bool {
	return len(q.items) > q.rows-queueHeaderRows
}

func (q queueContent) maxOffset() int {
	return max(len(q.items)-q.visibleRows(), 0)
}

// marker reports how many items lie past the edge of the window. It keeps its
// line at zero, so the rows below it do not shift as the reader scrolls to an
// edge and away again.
func (q queueContent) marker(hidden int, arrow string) string {
	if hidden <= 0 {
		return ""
	}
	return queueNoteStyle().Render(fmt.Sprintf("  %s %d more", arrow, hidden))
}

// row draws one queue item: the title and the artist, and a marker on the one
// that is playing. The speaker's own position is held on the item but not
// drawn — it is what the highlight matches on, and what a later "jump to
// track" will need.
func (q queueContent) row(item sonos.QueueItem, title, artist int) string {
	mark := " "
	if q.isPlaying(item) {
		mark = "▶"
	}

	line := fmt.Sprintf("%s %s  %s", mark,
		padRight(clip(item.Title, title), title),
		clip(item.Artist, artist))

	if q.isPlaying(item) {
		return queuePlayingStyle().Render(line)
	}
	return queueRowStyle().Render(line)
}

// isPlaying guards the zero as well as matching the position. A queue counts
// from one, so zero means nothing is playing from it — and an item that
// somehow carries no position must not answer to that.
func (q queueContent) isPlaying(item sonos.QueueItem) bool {
	return q.playing > 0 && item.Position == q.playing
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
		room:    speaker.device.DisplayName(),
		loading: true,
		rows:    m.queue.ContentRows(),
		width:   m.queue.ContentColumns(),
	}
	return m, readQueue(speaker.device)
}

func readQueue(device *sonos.Device) tea.Cmd {
	return func() tea.Msg {
		items, err := device.Queue()
		if err != nil {
			slog.Warn("reading queue for the modal", "speaker", device.DisplayName(), "err", err)
			return queueLoadedMsg{device: device, room: device.DisplayName(), err: err}
		}

		return queueLoadedMsg{
			device:  device,
			room:    device.DisplayName(),
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
	return m, nil
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

func queueNoteStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))
}

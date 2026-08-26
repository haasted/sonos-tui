package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
	"github.com/haasted/sonos-sandbox/pkg/ui/spinners"
)

// mockTrackProgress is interface1's mockTrack plus a playback position —
// durations for Fortunate Son/Wind of Change/Lift Me Up are the real
// values seen in earlier -capture sessions; Frankie Sinatra's is a guess,
// since that one was never captured with its own duration.
type mockTrackProgress struct {
	room     string
	track    string
	artist   string
	album    string
	elapsed  time.Duration
	duration time.Duration
}

var mockProgressTracks = []mockTrackProgress{
	{room: "Kitchen", track: "Fortunate Son", artist: "Creedence Clearwater Revival", album: "Willy and the Poor Boys",
		elapsed: 58 * time.Second, duration: 2*time.Minute + 21*time.Second},
	{room: "Bedroom", track: "Wind of Change", artist: "Scorpions", album: "Crazy World",
		elapsed: 3*time.Minute + 40*time.Second, duration: 5*time.Minute + 12*time.Second},
	{room: "Living Room", track: "Lift Me Up", artist: "Moby", album: "Hotel",
		elapsed: 22 * time.Second, duration: 3*time.Minute + 19*time.Second},
	{room: "Office", track: "Frankie Sinatra", artist: "The Avalanches", album: "Wildflower",
		elapsed: 2*time.Minute + 10*time.Second, duration: 3*time.Minute + 44*time.Second},
	// Deliberately long title — exercises the marquee-scroll path below
	// rather than the static one every other entry takes.
	{room: "Guest Room", track: "There Is a Kingdom (2011 Remaster)", artist: "Nick Cave & The Bad Seeds", album: "The Boatman's Call",
		elapsed: 1*time.Minute + 45*time.Second, duration: 6*time.Minute + 32*time.Second},
	// Deliberately long artist/album, with a short title — exercises the
	// artist/album marquee path independently of the title's, and shows
	// them scrolling within their wider window (see metaWindowWidth).
	{room: "Studio", track: "Anthem", artist: "The Melbourne Symphony Orchestra & Chorus (Live Recording)", album: "Live at the Sydney Opera House (Deluxe Edition, Remastered)",
		elapsed: 1*time.Minute + 12*time.Second, duration: 4*time.Minute + 5*time.Second},
}

var progressTimeStyle = lipgloss.NewStyle().
	Faint(true).
	Foreground(lipgloss.AdaptiveColor{Light: "#4A4A4A", Dark: "#9B9B9B"})

// titleRowWidth is the fixed width the position text right-aligns
// against, so it stays pinned to a constant column regardless of how long
// a given track's title is — a real terminal-width-reactive version would
// need Tab content to learn the width from tea.WindowSizeMsg, which
// panes.Tabs/simpleTabsModel don't currently thread down to Content, so
// this mockup uses a fixed constant instead. A title long enough to
// collide with it would need truncation, not handled here.
const titleRowWidth = 50

// longTitleWindowWidth is both the threshold ("titles longer than this
// scroll instead of rendering statically") and the marquee window's fixed
// width for the title line specifically. Fixed on purpose, same reasoning
// as titleRowWidth: a title that scrolls within a constant-width window
// contributes a constant width to the titleRowWidth gap math below, so the
// position stays put even while the title animates — only static-vs-
// scrolling width would need to change dynamically to break that, and it
// never does per-tab. 35 leaves a couple of columns of gap before the
// position for every mock duration (all single-digit minutes); a title row
// with double-digit minutes on both sides of the position would eat into
// that and fall back to the 1-column minimum gap, not a collision.
const longTitleWindowWidth = 35

const (
	scrollInterval = 150 * time.Millisecond
	scrollDelay    = 3 * time.Second // how long a long line sits still before it starts scrolling
)

// scrollDelayTicks is scrollDelay expressed as a tick count, so the wait
// can be a simple counter rather than tracking wall-clock time separately
// from the tick loop that's already running.
const scrollDelayTicks = int(scrollDelay / scrollInterval)

// scrollTickMsg carries the generation it was scheduled under (see
// progressContent.scrollGen) so a stale chain from an earlier tab
// activation can recognize itself as stale and stop rescheduling, instead
// of running forever alongside whatever chain the current activation
// started.
type scrollTickMsg struct {
	gen int
}

func scrollTick(gen int) tea.Cmd {
	return tea.Tick(scrollInterval, func(time.Time) tea.Msg {
		return scrollTickMsg{gen: gen}
	})
}

// marqueeLine is one line of text that scrolls via the doubled-runes
// sliding-window technique (see effect-marquee.go) once it's too wide to
// fit statically within windowWidth. windowWidth is per-line rather than
// one shared constant, since not every line has the same amount of room:
// the title shares its row with the position indicator, artist/album don't
// and so get a wider window (see newProgressContent).
type marqueeLine struct {
	text        string
	windowWidth int
	runes       []rune // text + trailing gap, doubled at render time
	offset      int
}

func newMarqueeLine(text string, windowWidth int) marqueeLine {
	return marqueeLine{text: text, windowWidth: windowWidth, runes: []rune(text + "   ")}
}

func (l marqueeLine) needsScroll() bool {
	return lipgloss.Width(l.text) > l.windowWidth
}

// advance steps the scroll window one position forward, wrapping via modulo.
func (l marqueeLine) advance() marqueeLine {
	l.offset = (l.offset + 1) % len(l.runes)
	return l
}

// render returns the styled text — the full string if it fits, or a
// windowWidth-wide scrolling slice of it (doubled first, so the window
// sliding past the end wraps into the start for free) if not.
func (l marqueeLine) render(style lipgloss.Style) string {
	if !l.needsScroll() {
		return style.Render(l.text)
	}
	doubled := append(append([]rune{}, l.runes...), l.runes...)
	visible := string(doubled[l.offset : l.offset+l.windowWidth])
	return style.Render(visible)
}

// progressContent is interface1's nowPlayingContent plus an "elapsed /
// total" position, right-aligned on the title row — the plain-text
// alternative to a full-width progress bar, which read as disproportionate
// to the single number it was conveying. Any line too long to fit — title,
// artist, or album — scrolls independently via marqueeLine.
type progressContent struct {
	track   mockTrackProgress
	spinner spinner.Model

	title  marqueeLine
	artist marqueeLine
	album  marqueeLine

	// ticksWaited is a single shared delay gate for all three lines: it
	// counts up to scrollDelayTicks before any line starts advancing, so a
	// tab that needs scrolling settles for a beat before anything moves,
	// rather than each line pausing independently.
	ticksWaited int

	// scrollGen is bumped every Init() (i.e. every time this tab regains
	// focus) and stamped onto the tea.Tick chain started at that moment.
	// It's a pointer, shared across every copy of this progressContent,
	// because Init() only returns a tea.Cmd — it can't hand panes.Tabs an
	// updated model to store, so the only way for it to leave a mark that
	// Update() can later see is through shared state, not a value receiver
	// mutation. Without this, panes.Tabs re-running Init() on every tab
	// switch (by design, so a fresh animation restarts cleanly) leaves the
	// *previous* activation's tick chain running forever alongside the new
	// one, since Bubbletea has no way to cancel an in-flight tea.Cmd — each
	// extra chain adds another scrollTickMsg every interval, so the scroll
	// speed compounds with every visit to the tab.
	scrollGen *int
}

func newProgressContent(t mockTrackProgress) progressContent {
	s := spinner.New()
	s.Spinner = spinners.Equalizer

	// Artist/album don't share their row with the position indicator the
	// way the title does, so they get a wider scroll window: the full
	// title-row budget minus just the spinner + separating space, instead
	// of minus the position too.
	metaWindowWidth := titleRowWidth - lipgloss.Width(s.View()) - 1

	return progressContent{
		track:     t,
		spinner:   s,
		title:     newMarqueeLine(t.track, longTitleWindowWidth),
		artist:    newMarqueeLine(t.artist, metaWindowWidth),
		album:     newMarqueeLine(t.album, metaWindowWidth),
		scrollGen: new(int),
	}
}

// needsScroll reports whether any line needs the scroll-tick chain running at all.
func (m progressContent) needsScroll() bool {
	return m.title.needsScroll() || m.artist.needsScroll() || m.album.needsScroll()
}

func (m progressContent) Init() tea.Cmd {
	if m.needsScroll() {
		*m.scrollGen++
		return tea.Batch(m.spinner.Tick, scrollTick(*m.scrollGen))
	}
	return m.spinner.Tick
}

func (m progressContent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case scrollTickMsg:
		if msg.gen != *m.scrollGen {
			// Left over from an earlier activation of this tab — don't
			// reschedule, so the chain dies here instead of running forever.
			return m, nil
		}
		if m.ticksWaited < scrollDelayTicks {
			m.ticksWaited++
			return m, scrollTick(msg.gen)
		}
		if m.title.needsScroll() {
			m.title = m.title.advance()
		}
		if m.artist.needsScroll() {
			m.artist = m.artist.advance()
		}
		if m.album.needsScroll() {
			m.album = m.album.advance()
		}
		return m, scrollTick(msg.gen)
	}
	return m, nil
}

func (m progressContent) View() string {
	spinnerGlyph := m.spinner.View()
	left := lipgloss.JoinHorizontal(lipgloss.Top, spinnerGlyph, " ", m.title.render(trackStyle))
	position := formatPosition(m.track.elapsed, m.track.duration)

	// Pad out to titleRowWidth so position lands on a fixed column instead
	// of trailing directly after however long this track's title is. A
	// title long enough to eat the whole budget just falls back to a
	// single-space gap rather than a negative repeat count.
	gap := titleRowWidth - lipgloss.Width(left) - lipgloss.Width(position)
	if gap < 1 {
		gap = 1
	}
	titleLine := left + strings.Repeat(" ", gap) + position

	indent := strings.Repeat(" ", lipgloss.Width(spinnerGlyph)+1)
	return lipgloss.JoinVertical(lipgloss.Left,
		titleLine,
		indent+m.artist.render(artistStyle),
		indent+m.album.render(albumStyle),
	)
}

// formatPosition renders "<elapsed> / <total>". Static here (this is mock
// UI, nothing is actually playing) — hooking it up to a live position
// later is just feeding it a changing elapsed value each tick.
func formatPosition(elapsed, duration time.Duration) string {
	return progressTimeStyle.Render(formatDuration(elapsed) + " / " + formatDuration(duration))
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d/time.Minute), int((d%time.Minute)/time.Second))
}

// layoutInterface3 is interface1 with an "elapsed / total" position,
// right-aligned on the title row. Pure mock UI — no real functionality.
func layoutInterface3() {
	tabs := make([]panes.Tab, len(mockProgressTracks))
	for i, t := range mockProgressTracks {
		tabs[i] = panes.Tab{Title: t.room, Content: newProgressContent(t)}
	}

	tb := panes.NewTabs(tabs)
	tb.Position = panes.Bottom
	tb.HideBorder = true
	runSimpleTabs(tb)
}

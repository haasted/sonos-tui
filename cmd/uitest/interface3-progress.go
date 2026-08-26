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
	{room: "Guest Room", track: "There Is a Kingdom (2011 Remaster)", artist: "Nick Cave & The Bad Seeds", album: "The Boatman's Call",
		elapsed: 1*time.Minute + 45*time.Second, duration: 6*time.Minute + 32*time.Second},
	{room: "Studio", track: "Anthem", artist: "The Melbourne Symphony Orchestra & Chorus (Live Recording)", album: "Live at the Sydney Opera House (Deluxe Edition, Remastered)",
		elapsed: 1*time.Minute + 12*time.Second, duration: 4*time.Minute + 5*time.Second},
}

var progressTimeStyle = lipgloss.NewStyle().
	Faint(true).
	Foreground(lipgloss.AdaptiveColor{Light: "#4A4A4A", Dark: "#9B9B9B"})

const titleRowWidth = 50

const longTitleWindowWidth = 35

const (
	scrollInterval = 150 * time.Millisecond
	scrollDelay    = 3 * time.Second
)

const scrollDelayTicks = int(scrollDelay / scrollInterval)

type scrollTickMsg struct {
	gen int
}

func scrollTick(gen int) tea.Cmd {
	return tea.Tick(scrollInterval, func(time.Time) tea.Msg {
		return scrollTickMsg{gen: gen}
	})
}

type marqueeLine struct {
	text        string
	windowWidth int
	runes       []rune
	offset      int
}

func newMarqueeLine(text string, windowWidth int) marqueeLine {
	return marqueeLine{text: text, windowWidth: windowWidth, runes: []rune(text + "   ")}
}

func (l marqueeLine) needsScroll() bool {
	return lipgloss.Width(l.text) > l.windowWidth
}

func (l marqueeLine) advance() marqueeLine {
	l.offset = (l.offset + 1) % len(l.runes)
	return l
}

func (l marqueeLine) render(style lipgloss.Style) string {
	if !l.needsScroll() {
		return style.Render(l.text)
	}
	doubled := append(append([]rune{}, l.runes...), l.runes...)
	visible := string(doubled[l.offset : l.offset+l.windowWidth])
	return style.Render(visible)
}

type progressContent struct {
	track   mockTrackProgress
	spinner spinner.Model

	title  marqueeLine
	artist marqueeLine
	album  marqueeLine

	ticksWaited int

	scrollGen *int
}

func newProgressContent(t mockTrackProgress) progressContent {
	s := spinner.New()
	s.Spinner = spinners.Equalizer

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

func formatPosition(elapsed, duration time.Duration) string {
	return progressTimeStyle.Render(formatDuration(elapsed) + " / " + formatDuration(duration))
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d/time.Minute), int((d%time.Minute)/time.Second))
}

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

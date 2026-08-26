package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
	"github.com/haasted/sonos-sandbox/pkg/ui/spinners"
)

// mockTrack is fake Artist/Track/Album data for the "interface" UI mockups —
// no real Device/Discover wiring, just enough to see a layout. Reuses real
// song titles from earlier -capture sessions for a bit of realism.
type mockTrack struct {
	room   string
	track  string
	artist string
	album  string
}

var mockNowPlaying = []mockTrack{
	{room: "Kitchen", track: "Fortunate Son", artist: "Creedence Clearwater Revival", album: "Willy and the Poor Boys"},
	{room: "Bedroom", track: "Wind of Change", artist: "Scorpions", album: "Crazy World"},
	{room: "Living Room", track: "Lift Me Up", artist: "Moby", album: "Hotel"},
	{room: "Office", track: "Frankie Sinatra", artist: "The Avalanches", album: "Wildflower"},
}

var (
	trackStyle = lipgloss.NewStyle().Bold(true)

	artistStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#4A4A4A", Dark: "#9B9B9B"})

	albumStyle = lipgloss.NewStyle().
			Faint(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#4A4A4A", Dark: "#9B9B9B"})
)

// nowPlayingContent is one speaker's mock now-playing tab content: an
// animated spinner (standing in for "this speaker is actively playing")
// to the left of the track title, artist/album below.
type nowPlayingContent struct {
	track   mockTrack
	spinner spinner.Model
}

func newNowPlayingContent(t mockTrack) nowPlayingContent {
	s := spinner.New()
	// s.Spinner = spinners.Bars
	s.Spinner = spinners.Equalizer
	return nowPlayingContent{track: t, spinner: s}
}

func (m nowPlayingContent) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m nowPlayingContent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m nowPlayingContent) View() string {
	spinnerGlyph := m.spinner.View()
	titleLine := lipgloss.JoinHorizontal(lipgloss.Top, spinnerGlyph, " ", trackStyle.Render(m.track.track))

	// Indent artist/album by the spinner's rendered width + the separating
	// space, so their text lines up under the title rather than the spinner.
	indent := strings.Repeat(" ", lipgloss.Width(spinnerGlyph)+1)
	return lipgloss.JoinVertical(lipgloss.Left,
		titleLine,
		indent+artistStyle.Render(m.track.artist),
		indent+albumStyle.Render(m.track.album),
	)
}

// layoutInterface1 is the first "interface" mockup: artist/track/album with
// a playing-indicator spinner, speakers as tabs along the bottom. Pure mock
// UI — no real functionality.
func layoutInterface1() {
	tabs := make([]panes.Tab, len(mockNowPlaying))
	for i, t := range mockNowPlaying {
		tabs[i] = panes.Tab{Title: t.room, Content: newNowPlayingContent(t)}
	}

	tb := panes.NewTabs(tabs)
	tb.Position = panes.Bottom
	tb.HideBorder = true
	runSimpleTabs(tb)
}

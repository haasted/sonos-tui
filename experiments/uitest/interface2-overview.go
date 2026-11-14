package main

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/ui/theme"

	"github.com/haasted/sonos-tui/pkg/ui/spinners"
)

// playbackState is a mock speaker's playback state for the "Overview"
// dashboard mockup (interface2) — unlike interface1, which always shows a
// spinner, this mockup deliberately shows mixed states.
type playbackState int

const (
	statePlaying playbackState = iota
	statePaused
	stateIdle
)

type overviewSpeaker struct {
	room   string
	state  playbackState
	track  string
	artist string
}

var mockOverview = []overviewSpeaker{
	{room: "Kitchen", state: statePlaying, track: "Fortunate Son", artist: "Creedence Clearwater Revival"},
	{room: "Bedroom", state: statePlaying, track: "Wind of Change", artist: "Scorpions"},
	{room: "Living Room", state: statePaused, track: "Lift Me Up", artist: "Moby"},
	{room: "Office", state: stateIdle},
}

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.LightDark(lipgloss.Color("#6B46C1"), lipgloss.Color("#A855F7")))

	roomStyle = lipgloss.NewStyle().Bold(true)

	idleStyle = lipgloss.NewStyle().
			Faint(true).
			Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))
)

// overviewRootModel is the "Overview" mockup: every speaker shown at once,
// no tabs — the opposite interaction model from interface1's one-at-a-time
// tabbed view. Each statePlaying speaker gets its own independently
// ticking spinner.Model; paused/idle rows are static.
type overviewRootModel struct {
	speakers []overviewSpeaker
	spinners []spinner.Model // parallel to speakers; only populated/ticked for statePlaying entries
	quitting bool
}

func newOverviewRootModel() overviewRootModel {
	spins := make([]spinner.Model, len(mockOverview))
	for i, s := range mockOverview {
		if s.state != statePlaying {
			continue
		}
		sp := spinner.New()
		sp.Spinner = spinners.Equalizer // same style interface1 uses, for a consistent household "look"
		spins[i] = sp
	}
	return overviewRootModel{speakers: mockOverview, spinners: spins}
}

func (m overviewRootModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	for i, s := range m.speakers {
		if s.state == statePlaying {
			cmds = append(cmds, m.spinners[i].Tick)
		}
	}
	return tea.Batch(cmds...)
}

func (m overviewRootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}

	if _, ok := msg.(spinner.TickMsg); ok {
		// spinner.Model.Update no-ops (nil cmd) for a tick that isn't its
		// own — safe to offer every spinner the same incoming message.
		var cmds []tea.Cmd
		for i, s := range m.speakers {
			if s.state != statePlaying {
				continue
			}
			var cmd tea.Cmd
			m.spinners[i], cmd = m.spinners[i].Update(msg)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

func (m overviewRootModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	roomWidth := 0
	for _, s := range m.speakers {
		if w := lipgloss.Width(s.room); w > roomWidth {
			roomWidth = w
		}
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render(fmt.Sprintf("SONOS — %d speakers", len(m.speakers))))
	b.WriteString("\n\n")

	for i, s := range m.speakers {
		icon := overviewIcon(s, m.spinners[i])
		room := roomStyle.Render(padRight(s.room, roomWidth))
		indent := strings.Repeat(" ", lipgloss.Width(icon)+1+roomWidth+2)

		if s.state == stateIdle {
			fmt.Fprintf(&b, "%s %s  %s\n\n", icon, room, idleStyle.Render("— nothing playing —"))
			continue
		}

		fmt.Fprintf(&b, "%s %s  %s\n", icon, room, trackStyle.Render(s.track))
		b.WriteString(indent + artistStyle.Render(s.artist) + "\n\n")
	}

	b.WriteString("q to quit\n")
	return tea.NewView(b.String())
}

// overviewIcon is the per-row state indicator: an animated spinner while
// playing, a static glyph for paused/idle.
func overviewIcon(s overviewSpeaker, sp spinner.Model) string {
	switch s.state {
	case statePlaying:
		return sp.View()
	case statePaused:
		return "⏸"
	default:
		return "·"
	}
}

// padRight right-pads s with spaces to width, measured by rendered width
// (not byte length) so it stays correct for multi-byte room names.
func padRight(s string, width int) string {
	if w := lipgloss.Width(s); w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

// layoutInterface2 is the "Overview" mockup. Pure mock UI — no real functionality.
func layoutInterface2() {
	if _, err := tea.NewProgram(newOverviewRootModel()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

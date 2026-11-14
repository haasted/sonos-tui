package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/termenv"

	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// effectsRootModel is the outermost Bubbletea model for the "effects"
// layout — parallel to tabsRootModel, but each tab is a self-contained
// color/animation effect demo instead of a room. New effects get added as
// new tabs here as we come up with them.
type effectsRootModel struct {
	tabs     panes.Tabs
	quitting bool
}

func newEffectsRootModel(bg colorful.Color) effectsRootModel {
	t := panes.NewTabs([]panes.Tab{
		{Title: "Pulse", Content: newPulseEffect()},
		{Title: "Fade Out", Content: newFadeEffect(bg)},
		{Title: "Blink", Content: newBlinkEffect()},
		{Title: "Blink Text", Content: newTextBlinkEffect()},
		{Title: "Marquee", Content: newMarqueeEffect()},
		{Title: "Typewriter", Content: newTypewriterEffect()},
		{Title: "Wave", Content: newWaveEffect()},
		{Title: "Shake", Content: newShakeEffect()},
		{Title: "Progress", Content: newProgressEffect()},
		{Title: "Countdown", Content: newCountdownEffect()},
	})
	return effectsRootModel{tabs: t}
}

func (m effectsRootModel) Init() tea.Cmd {
	return m.tabs.Init()
}

func (m effectsRootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.tabs.Width = sizeMsg.Width
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}

	tabs, cmd := m.tabs.Update(msg)
	m.tabs = tabs
	return m, cmd
}

func (m effectsRootModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	return tea.NewView(m.tabs.View() + "\n\ntab/shift+tab or ←/→ to switch tabs · q to quit\n")
}

func layoutEffects() {
	// Query the terminal's real background color (OSC 11, via termenv —
	// the same mechanism Lipgloss's light/dark detection uses) before starting
	// the Bubbletea program, not inside Init(). Once Run() starts,
	// Bubbletea owns raw-mode reads from stdin for its own key-event loop;
	// querying afterward would race it for the OSC 11 reply and likely
	// just stall until termenv's query times out.
	//
	// Note: on a terminal that never answers OSC 11 at all, this call
	// blocks for up to termenv.OSCTimeout (5s as of muesli/termenv v0.16.0)
	// before falling back to a default — a one-time cost paid here, before
	// the program draws its first frame, not per-effect. Terminals that do
	// answer (most modern ones) resolve in low milliseconds.
	bg := termenv.ConvertToRGB(termenv.BackgroundColor())

	// No WithAltScreen, quitting->"" View: same inline, self-clearing
	// rendering as runTabs.
	if _, err := tea.NewProgram(newEffectsRootModel(bg)).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

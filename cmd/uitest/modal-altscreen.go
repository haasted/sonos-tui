package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

// modalDemo is throwaway content for exercising panes.Modal: it reports the
// sizes the component chose, so both modes can be judged live. Numbered filler
// rows make anything surviving a close read as debris rather than blank space.
type modalDemo struct {
	mode     panes.ModalMode
	terminal [2]int
	box      [2]int
	resizes  int
}

var _ tea.Model = modalDemo{}

func (m modalDemo) Init() tea.Cmd { return nil }

func (m modalDemo) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }

func (m modalDemo) View() tea.View {
	mode := "hover (no alt screen)"
	if m.mode == panes.ModalFullScreen {
		mode = "full screen (alt screen)"
	}

	lines := []string{
		demoTitleStyle.Render("Modal prototype (son-911)"),
		"",
		fmt.Sprintf("terminal      %d×%d", m.terminal[0], m.terminal[1]),
		fmt.Sprintf("modal         %d×%d", m.box[0], m.box[1]),
		fmt.Sprintf("mode          %s", mode),
		fmt.Sprintf("resizes open  %d", m.resizes),
		"",
		demoHintStyle.Render("m switches hover/full screen · ? or esc closes"),
		"",
	}
	for i := len(lines); i < m.box[1]; i++ {
		lines = append(lines, demoFillStyle.Render(fmt.Sprintf("%3d %s", i, strings.Repeat("·", max(m.box[0]-10, 4)))))
	}

	return tea.NewView(strings.Join(lines, "\n"))
}

var (
	demoTitleStyle = lipgloss.NewStyle().Bold(true).
			Foreground(theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24")))

	demoHintStyle = lipgloss.NewStyle().
			Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))

	demoFillStyle = lipgloss.NewStyle().Faint(true).
			Foreground(theme.LightDark(lipgloss.Color("#8A8A8A"), lipgloss.Color("#5A5A5A")))
)

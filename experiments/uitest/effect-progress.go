package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/ui/theme"
)

const (
	progressWidth        = 24 // bar width in cells
	progressTickInterval = 80 * time.Millisecond
	progressPauseTicks   = 8 // brief pause at 100% before resetting
)

// progressEffect is a hand-rolled fill bar (block characters, no external
// widget) that loops: fill left to right, pause briefly at 100%, reset.
type progressEffect struct {
	step    int // 0..progressWidth cells filled
	pausing bool
	pauseAt int
}

func newProgressEffect() progressEffect { return progressEffect{} }

type progressTickMsg time.Time

func progressTick() tea.Cmd {
	return tea.Tick(progressTickInterval, func(t time.Time) tea.Msg { return progressTickMsg(t) })
}

func (m progressEffect) Init() tea.Cmd { return progressTick() }

func (m progressEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(progressTickMsg); !ok {
		return m, nil
	}

	if m.pausing {
		m.pauseAt--
		if m.pauseAt <= 0 {
			m.pausing = false
			m.step = 0
		}
		return m, progressTick()
	}

	m.step++
	if m.step >= progressWidth {
		m.step = progressWidth
		m.pausing = true
		m.pauseAt = progressPauseTicks
	}
	return m, progressTick()
}

func (m progressEffect) View() tea.View {
	filled := progressFilledStyle.Render(strings.Repeat("█", m.step))
	empty := progressEmptyStyle.Render(strings.Repeat("░", progressWidth-m.step))
	pct := m.step * 100 / progressWidth
	return tea.NewView(progressBoxStyle.Render(fmt.Sprintf("%s%s %3d%%", filled, empty, pct)))
}

var (
	progressFilledStyle = lipgloss.NewStyle().Foreground(theme.LightDark(lipgloss.Color("#0B6E4F"), lipgloss.Color("#3DDC97")))
	progressEmptyStyle  = lipgloss.NewStyle().Foreground(theme.LightDark(lipgloss.Color("#D0D0D0"), lipgloss.Color("#4A4A4A")))
	progressBoxStyle    = lipgloss.NewStyle().Bold(true).Padding(1, 3)
)

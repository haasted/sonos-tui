package main

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	countdownStart        = 5
	countdownTickInterval = 700 * time.Millisecond
	countdownPauseTicks   = 2 // brief pause on "Go!" before resetting
)

// countdownEffect ticks a number down to zero, shows "Go!" briefly, then
// resets — a numeric/component effect rather than a text or color one.
type countdownEffect struct {
	remaining int
	done      bool
	pauseAt   int
}

func newCountdownEffect() countdownEffect { return countdownEffect{remaining: countdownStart} }

type countdownTickMsg time.Time

func countdownTick() tea.Cmd {
	return tea.Tick(countdownTickInterval, func(t time.Time) tea.Msg { return countdownTickMsg(t) })
}

func (m countdownEffect) Init() tea.Cmd { return countdownTick() }

func (m countdownEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(countdownTickMsg); !ok {
		return m, nil
	}

	if m.done {
		m.pauseAt--
		if m.pauseAt <= 0 {
			m.done = false
			m.remaining = countdownStart
		}
		return m, countdownTick()
	}

	m.remaining--
	if m.remaining <= 0 {
		m.done = true
		m.pauseAt = countdownPauseTicks
	}
	return m, countdownTick()
}

func (m countdownEffect) View() string {
	if m.done {
		return countdownGoStyle.Render("Go!")
	}
	return countdownStyle.Render(fmt.Sprintf("%d", m.remaining))
}

var (
	countdownStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(1, 4).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.AdaptiveColor{Light: "#6B46C1", Dark: "#A855F7"})

	countdownGoStyle = lipgloss.NewStyle().
				Bold(true).
				Padding(1, 4).
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#0B6E4F"))
)

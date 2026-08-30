package nowplaying

import (
	"time"

	"github.com/charmbracelet/lipgloss"
)

const (
	titleRowWidth        = 50
	longTitleWindowWidth = 35
)

const pausedGlyph = "⣿"

const (
	scrollInterval = 150 * time.Millisecond
	scrollDelay    = 3 * time.Second
)

const scrollDelayTicks = int(scrollDelay / scrollInterval)

var (
	trackStyle = lipgloss.NewStyle().Bold(true)

	artistStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#4A4A4A", Dark: "#9B9B9B"})

	albumStyle = lipgloss.NewStyle().
			Faint(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#4A4A4A", Dark: "#9B9B9B"})

	timeStyle = lipgloss.NewStyle().
			Faint(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#4A4A4A", Dark: "#9B9B9B"})
)

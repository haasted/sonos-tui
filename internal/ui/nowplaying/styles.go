package nowplaying

import (
	"time"

	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/theme"
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
			Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))

	albumStyle = lipgloss.NewStyle().
			Faint(true).
			Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))

	timeStyle = lipgloss.NewStyle().
			Faint(true).
			Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))

	errorStyle = lipgloss.NewStyle().
			Foreground(theme.LightDark(lipgloss.Color("#B00020"), lipgloss.Color("#FF6B6B")))
)

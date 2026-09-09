package nowplaying

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const ScrollDelayTicks = scrollDelayTicks

// SweepLabel starts the stream label's glint immediately, so a test does not
// have to wait out the idle gap before one begins.
func SweepLabel(m Model) Model {
	m.labelSweep = time.Now()
	return m
}

// ScrollGen is the generation the marquee's live tick chain carries.
func ScrollGen(m Model) int { return *m.scrollGen }

// NewScrollTick builds a marquee tick for a given generation, so a test can
// deliver one from a chain that should already be dead.
func NewScrollTick(gen int) tea.Msg { return scrollTickMsg{gen: gen} }

// VisibleTitle is the marquee's current window on its own, isolated from the
// rest of the row — the clock and the volume readout share that line.
func VisibleTitle(m Model) string { return m.title.Render(lipgloss.NewStyle()) }

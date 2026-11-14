// The part of the client's list modal glue the mock-up needs: the interface
// a list holds to, its repaint tick, and two helpers from elsewhere in the
// client.
package main

import (
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// listContent is what a list modal holds.
type listContent interface {
	tea.Model
	animating() bool // something on screen moves, so it needs frames
	bumping() bool   // an end is lit, which a fresh chain must carry
	borderLight() panes.BorderLight
	hintReady() bool // there are rows for the hinted keys to act on
	resized(rows, width int) tea.Model
}

// listTickMsg is one frame of a list's repaint chain.
type listTickMsg struct{ gen int }

var generations atomic.Int64

func listTick(gen int) tea.Cmd {
	return tea.Tick(listPulseInterval, func(time.Time) tea.Msg { return listTickMsg{gen: gen} })
}

// nextGen returns a generation unique across the process, for any chain.
func nextGen() int { return int(generations.Add(1)) }

// padRight pads s with spaces to width cells.
func padRight(s string, width int) string {
	if pad := width - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

package main

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

const (
	// hintDelay is how long the program waits before offering help. Long enough
	// that anyone who knows the keys has already pressed one, short enough that
	// someone who does not is still looking at the screen.
	hintDelay = 4 * time.Second

	hintText = "press ? for keyboard shortcuts"
)

// hintTickMsg fires once, hintDelay after startup. It carries no generation:
// the timer is never restarted, so there is no second chain it could be
// confused with — hintDone alone decides whether it still applies.
type hintTickMsg struct{}

func hintTimer() tea.Cmd {
	return tea.Tick(hintDelay, func(time.Time) tea.Msg { return hintTickMsg{} })
}

// applyHintTick offers the help hint, unless the moment for it has passed. It
// is an invitation to a first-time user, not a recurring nag, so anything that
// shows the user has found the keyboard — or anything more important arriving
// on the same rule — retires it for the rest of the run.
func (m appModel) applyHintTick() (tea.Model, tea.Cmd) {
	if m.hintDone {
		return m, nil
	}
	m.hintDone = true
	return m, notifyEntrance(panes.NoticeInfo, hintText, panes.NoticeBloom)
}

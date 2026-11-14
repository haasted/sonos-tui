package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/ui/theme"
)

// textBlinkEffect is a variant of blinkEffect where the text itself
// disappears and reappears (rendered vs. blank) instead of a background
// box flashing around static text. Shares blinkState/blinkTick with
// blinkEffect (see effect-blink.go) — only what an on/off tick looks like
// differs.
//
// Lipgloss styles do have a Blink(true) method (the ANSI "slow blink" SGR
// attribute), which would let the terminal itself animate this instead.
// Not used here: terminal support is inconsistent — several terminals,
// iTerm2 included, disable blink rendering by default for
// accessibility/annoyance reasons — so hand-rolling it via tea.Tick, same
// as blinkEffect, behaves the same everywhere.
type textBlinkEffect struct {
	blinkState
}

func newTextBlinkEffect() textBlinkEffect {
	return textBlinkEffect{blinkState: newBlinkState()}
}

func (b textBlinkEffect) Init() tea.Cmd {
	return blinkTick()
}

func (b textBlinkEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(blinkTickMsg); !ok {
		return b, nil
	}
	b.blinkState = b.blinkState.advance()
	return b, blinkTick()
}

func (b textBlinkEffect) View() tea.View {
	style := textBlinkRestStyle
	if !b.resting {
		style = textBlinkOnStyle
	}

	label := fmt.Sprintf("⚠  Attention needed  (%s)", b.status())
	content := label
	if !b.resting && b.ticksInBurst%2 == 1 {
		// "off" tick mid-burst: blank space the same width as the label,
		// so the text disappearing doesn't shift the layout.
		content = strings.Repeat(" ", lipgloss.Width(label))
	}
	return tea.NewView(style.Render(content))
}

var (
	textBlinkOnStyle = lipgloss.NewStyle().
				Bold(true).
				Padding(1, 3).
				Foreground(lipgloss.Color("#FF3860"))

	textBlinkRestStyle = lipgloss.NewStyle().
				Padding(1, 3).
				Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))
)

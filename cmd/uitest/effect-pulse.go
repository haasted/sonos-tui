package main

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// pulseEffect is a tea.Model that continuously cycles its own background hue
// via go-colorful — our first "festive" color effect, and a live check that
// a self-ticking model works correctly as panes.Tab content, including
// restarting cleanly after its tab loses and regains focus.
type pulseEffect struct {
	hue float64 // degrees, 0-360
}

func newPulseEffect() pulseEffect {
	return pulseEffect{}
}

type pulseTickMsg time.Time

func pulseTick() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(t time.Time) tea.Msg {
		return pulseTickMsg(t)
	})
}

func (p pulseEffect) Init() tea.Cmd {
	return pulseTick()
}

func (p pulseEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(pulseTickMsg); ok {
		p.hue += 3
		if p.hue >= 360 {
			p.hue -= 360
		}
		return p, pulseTick()
	}
	return p, nil
}

func (p pulseEffect) View() string {
	bg := colorful.Hsv(p.hue, 0.85, 0.85)

	// Flip text color by background luminance so it stays readable across
	// the whole hue wheel (white-on-yellow is unreadable otherwise).
	lum := 0.2126*bg.R + 0.7152*bg.G + 0.0722*bg.B
	fg := lipgloss.Color("#111111")
	if lum < 0.5 {
		fg = lipgloss.Color("#FFFFFF")
	}

	style := lipgloss.NewStyle().
		Bold(true).
		Padding(1, 3).
		Background(lipgloss.Color(bg.Hex())).
		Foreground(fg)

	return style.Render(fmt.Sprintf("Pulse  hue=%.0f°  %s", p.hue, bg.Hex()))
}

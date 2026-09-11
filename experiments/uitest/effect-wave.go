package main

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"
)

const (
	waveText         = "S O N O S   E F F E C T S"
	waveTickInterval = 80 * time.Millisecond
	waveHueStep      = 8.0 // degrees of hue difference between adjacent runes — the ripple
	waveSpeed        = 6.0 // degrees the whole wave advances per tick
)

// waveEffect is a rainbow color ripple across static text — unlike Pulse
// (one solid color animating over a whole box), each rune gets its own hue
// offset from its neighbors, so the color itself appears to travel through
// the text left to right.
type waveEffect struct {
	offset float64 // degrees, advances each tick
}

func newWaveEffect() waveEffect { return waveEffect{} }

type waveTickMsg time.Time

func waveTick() tea.Cmd {
	return tea.Tick(waveTickInterval, func(t time.Time) tea.Msg { return waveTickMsg(t) })
}

func (m waveEffect) Init() tea.Cmd { return waveTick() }

func (m waveEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(waveTickMsg); !ok {
		return m, nil
	}
	m.offset += waveSpeed
	if m.offset >= 360 {
		m.offset -= 360
	}
	return m, waveTick()
}

func (m waveEffect) View() tea.View {
	var out string
	for i, r := range []rune(waveText) {
		hue := m.offset + float64(i)*waveHueStep
		for hue >= 360 {
			hue -= 360
		}
		color := colorful.Hsv(hue, 0.85, 0.9)
		out += lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color.Hex())).Render(string(r))
	}
	return tea.NewView(waveBoxStyle.Render(out))
}

var waveBoxStyle = lipgloss.NewStyle().Padding(1, 3)

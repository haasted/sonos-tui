package main

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	shakeText         = "⚡ Look out! ⚡"
	shakeTickInterval = 60 * time.Millisecond
)

// shakeOffsets is a hand-picked, looping sequence of leading-space counts,
// so the text jitters horizontally in place rather than drifting off in
// one direction — a "shake to get attention" effect distinct from Blink
// (color/visibility) and Marquee (steady one-way scroll).
var shakeOffsets = []int{0, 2, 0, 1, 0, 2, 1, 0}

type shakeEffect struct {
	frame int
}

func newShakeEffect() shakeEffect { return shakeEffect{} }

type shakeTickMsg time.Time

func shakeTick() tea.Cmd {
	return tea.Tick(shakeTickInterval, func(t time.Time) tea.Msg { return shakeTickMsg(t) })
}

func (m shakeEffect) Init() tea.Cmd { return shakeTick() }

func (m shakeEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(shakeTickMsg); !ok {
		return m, nil
	}
	m.frame = (m.frame + 1) % len(shakeOffsets)
	return m, shakeTick()
}

func (m shakeEffect) View() string {
	pad := strings.Repeat(" ", shakeOffsets[m.frame])
	return shakeStyle.Render(pad + shakeText)
}

var shakeStyle = lipgloss.NewStyle().
	Bold(true).
	Padding(1, 3).
	Foreground(lipgloss.Color("#FF3860"))

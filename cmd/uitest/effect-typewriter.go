package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	typewriterText         = "The quick brown fox jumps over the lazy dog."
	typewriterTickInterval = 60 * time.Millisecond
	typewriterPauseTicks   = 15 // ~1.5s fully-typed pause before resetting
)

// typewriterEffect reveals text one rune at a time with a blinking-cursor
// feel, pauses once fully typed, then resets and retypes.
type typewriterEffect struct {
	shown   int // runes revealed so far
	pausing bool
	pauseAt int
}

func newTypewriterEffect() typewriterEffect { return typewriterEffect{} }

type typewriterTickMsg time.Time

func typewriterTick() tea.Cmd {
	return tea.Tick(typewriterTickInterval, func(t time.Time) tea.Msg { return typewriterTickMsg(t) })
}

func (m typewriterEffect) Init() tea.Cmd { return typewriterTick() }

func (m typewriterEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(typewriterTickMsg); !ok {
		return m, nil
	}

	if m.pausing {
		m.pauseAt--
		if m.pauseAt <= 0 {
			m.pausing = false
			m.shown = 0
		}
		return m, typewriterTick()
	}

	m.shown++
	if m.shown >= len([]rune(typewriterText)) {
		m.shown = len([]rune(typewriterText))
		m.pausing = true
		m.pauseAt = typewriterPauseTicks
	}
	return m, typewriterTick()
}

func (m typewriterEffect) View() string {
	visible := string([]rune(typewriterText)[:m.shown])
	cursor := "▌"
	if m.pausing {
		cursor = "" // hide the cursor once fully typed, reads as "done"
	}
	return typewriterStyle.Render(visible + cursor)
}

var typewriterStyle = lipgloss.NewStyle().
	Bold(true).
	Padding(1, 3).
	Foreground(lipgloss.AdaptiveColor{Light: "#1A1A1A", Dark: "#E5E5E5"})

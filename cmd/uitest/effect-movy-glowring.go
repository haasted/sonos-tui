package main

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/movy"
)

const (
	glowRingW   = 40
	glowRingH   = 20
	glowRingFPS = 30
)

type glowRingEffect struct {
	frame *movy.Frame
	t     float64
}

func newGlowRingEffect() glowRingEffect {
	f := movy.NewFrame(glowRingW, glowRingH)
	f.Rect(0, 0, glowRingW, glowRingH, movy.V3Of(0.02, 0.02, 0.05))
	return glowRingEffect{frame: f}
}

type glowRingTickMsg time.Time

func glowRingTick() tea.Cmd {
	return tea.Tick(time.Second/glowRingFPS, func(t time.Time) tea.Msg { return glowRingTickMsg(t) })
}

func (m glowRingEffect) Init() tea.Cmd { return glowRingTick() }

func (m glowRingEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(glowRingTickMsg); !ok {
		return m, nil
	}
	m.t += 1.0 / glowRingFPS
	return m, glowRingTick()
}

func (m glowRingEffect) View() tea.View {
	cx := float32(glowRingW)/2 + float32(math.Cos(m.t*1.3))*float32(glowRingW)/2.6
	cy := float32(glowRingH)/2 + float32(math.Sin(m.t*2.1))*float32(glowRingH)/2.6
	ring := themedV3("#7C3AED", "#22D3EE")

	m.frame.BeginFrame()
	m.frame.GRing(cx, cy, 2.2, ring)
	m.frame.Composite()

	return tea.NewView(movyStyle.Render(m.frame.Surface.Render()))
}

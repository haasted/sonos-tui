package main

import (
	"math"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-tui/internal/ui/movy"
)

const (
	crtGridW   = 40
	crtGridH   = 20
	crtGridFPS = 30
)

var crtGridBars = []movy.Rgb{
	{R: 235, G: 64, B: 52},
	{R: 235, G: 160, B: 52},
	{R: 235, G: 220, B: 52},
	{R: 90, G: 200, B: 90},
	{R: 70, G: 140, B: 235},
	{R: 150, G: 90, B: 220},
}

type crtGridEffect struct {
	frame *movy.Frame
	t     float64
}

func newCRTGridEffect() crtGridEffect {
	f := movy.NewFrame(crtGridW, crtGridH)
	barW := crtGridW / len(crtGridBars)
	for i, c := range crtGridBars {
		f.Rect(i*barW, 0, barW, crtGridH, movy.FromRgb(c))
	}
	return crtGridEffect{frame: f}
}

type crtGridTickMsg time.Time

func crtGridTick() tea.Cmd {
	return tea.Tick(time.Second/crtGridFPS, func(t time.Time) tea.Msg { return crtGridTickMsg(t) })
}

func (m crtGridEffect) Init() tea.Cmd { return crtGridTick() }

func (m crtGridEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(crtGridTickMsg); !ok {
		return m, nil
	}
	m.t += 1.0 / crtGridFPS
	return m, crtGridTick()
}

func (m crtGridEffect) View() tea.View {
	m.frame.Warmth = float32(0.5 + 0.5*math.Sin(m.t*0.8))
	m.frame.Flash = float32(math.Max(0, math.Sin(m.t*2.3)-0.9)) * 8

	m.frame.Composite()

	return tea.NewView(movyStyle.Render(m.frame.Surface.Render()))
}

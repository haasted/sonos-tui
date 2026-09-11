package main

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/movy"
)

const (
	textRevealFPS       = 30
	textRevealSpeed     = 14
	textRevealLinePause = 400 * time.Millisecond
	textRevealReplayGap = 1200 * time.Millisecond
)

var textRevealLines = []string{"So What", "Miles Davis", "Kind of Blue"}

type textRevealEffect struct {
	start       time.Time
	base, flash movy.V3
}

func newTextRevealEffect() textRevealEffect {
	return textRevealEffect{
		start: time.Now(),
		base:  themedV3("#1D4ED8", "#66D9FF"),
		flash: movy.WhiteF,
	}
}

func (m textRevealEffect) lineStarts() []time.Duration {
	starts := make([]time.Duration, len(textRevealLines))
	var t time.Duration
	for i, line := range textRevealLines {
		starts[i] = t
		t += flashsweep.Duration(line, textRevealSpeed) + textRevealLinePause
	}
	return starts
}

func (m textRevealEffect) totalDuration() time.Duration {
	starts := m.lineStarts()
	last := starts[len(starts)-1]
	return last + flashsweep.Duration(textRevealLines[len(textRevealLines)-1], textRevealSpeed)
}

type textRevealTickMsg time.Time

func textRevealTick() tea.Cmd {
	return tea.Tick(time.Second/textRevealFPS, func(t time.Time) tea.Msg { return textRevealTickMsg(t) })
}

func (m textRevealEffect) Init() tea.Cmd { return textRevealTick() }

func (m textRevealEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(textRevealTickMsg); !ok {
		return m, nil
	}
	if time.Since(m.start) > m.totalDuration()+textRevealReplayGap {
		m.start = time.Now()
	}
	return m, textRevealTick()
}

func (m textRevealEffect) View() tea.View {
	elapsed := time.Since(m.start)
	starts := m.lineStarts()

	var b strings.Builder
	b.WriteString("\n\n")
	for i, line := range textRevealLines {
		lightPos := float32(-1000)
		if since := elapsed - starts[i]; since > 0 {
			lightPos = flashsweep.LightPos(since, textRevealSpeed)
		}
		b.WriteString("    " + flashsweep.Line(line, lightPos, m.base, m.flash) + "\n")
	}
	b.WriteString("\n")
	return tea.NewView(b.String())
}

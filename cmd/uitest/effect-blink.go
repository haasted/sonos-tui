package main

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// blinkFlashCount/blinkRestTicks/blinkTickInterval control one cycle,
// shared by every blink-style effect in this file: sit idle for
// blinkRestTicks ticks, then flash blinkFlashCount times (each flash = one
// highlighted tick + one plain tick), then rest again.
const (
	blinkFlashCount   = 4
	blinkRestTicks    = 20 // ~3s at blinkTickInterval
	blinkTickInterval = 150 * time.Millisecond
)

// blinkState is the idle-then-burst timing state machine shared by every
// blink-style effect (see blinkEffect and textBlinkEffect) — only what an
// "on" vs "off" tick actually looks like differs between them.
type blinkState struct {
	resting      bool
	restTicks    int // ticks remaining until the next flash burst
	ticksInBurst int // 0..blinkFlashCount*2-1 while flashing
}

func newBlinkState() blinkState {
	return blinkState{resting: true, restTicks: blinkRestTicks}
}

// advance moves the state machine forward by one tick.
func (s blinkState) advance() blinkState {
	if s.resting {
		s.restTicks--
		if s.restTicks <= 0 {
			s.resting = false
			s.ticksInBurst = 0
		}
		return s
	}

	s.ticksInBurst++
	if s.ticksInBurst >= blinkFlashCount*2 {
		s.resting = true
		s.restTicks = blinkRestTicks
	}
	return s
}

// status is a short human-readable phase label, shown in each variant's debug suffix.
func (s blinkState) status() string {
	if s.resting {
		return "resting"
	}
	return fmt.Sprintf("flash %d/%d", s.ticksInBurst/2+1, blinkFlashCount)
}

type blinkTickMsg time.Time

func blinkTick() tea.Cmd {
	return tea.Tick(blinkTickInterval, func(t time.Time) tea.Msg {
		return blinkTickMsg(t)
	})
}

// blinkEffect briefly flashes a highlighted box around static text to draw
// the eye to it, then goes quiet for a while before flashing again — like
// a notification badge rather than a constant animation, since something
// that never stops blinking stops reading as "look here" and starts
// reading as "broken."
type blinkEffect struct {
	blinkState
}

func newBlinkEffect() blinkEffect {
	return blinkEffect{blinkState: newBlinkState()}
}

func (b blinkEffect) Init() tea.Cmd {
	return blinkTick()
}

func (b blinkEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(blinkTickMsg); !ok {
		return b, nil
	}
	b.blinkState = b.blinkState.advance()
	return b, blinkTick()
}

func (b blinkEffect) View() string {
	on := !b.resting && b.ticksInBurst%2 == 0
	label := fmt.Sprintf("⚠  Attention needed  (%s)", b.status())

	style := blinkRestStyle
	if on {
		style = blinkFlashStyle
	}
	return style.Render(label)
}

// Same Padding on both styles so the box doesn't change size between
// on/off frames — only the color should flash, not the layout.
var (
	blinkFlashStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(1, 3).
			Background(lipgloss.Color("#FF3860")).
			Foreground(lipgloss.Color("#FFFFFF"))

	blinkRestStyle = lipgloss.NewStyle().
			Padding(1, 3).
			Foreground(lipgloss.AdaptiveColor{Light: "#4A4A4A", Dark: "#9B9B9B"})
)

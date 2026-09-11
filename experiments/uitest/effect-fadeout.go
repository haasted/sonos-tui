package main

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"
)

// fadeSteps/fadeTickInterval control how gradually the color fades;
// pauseTicks controls the "brief break" once fully faded, at the same tick rate.
const (
	fadeSteps        = 30
	fadeTickInterval = 50 * time.Millisecond
	pauseTicks       = 10 // ~0.5s
)

// fadeEffect fades a colored component down to the terminal's own
// background color (bg — queried once via termenv.BackgroundColor before
// the Bubbletea program starts, see layoutEffects), pauses briefly once
// fully faded, then resets and repeats.
type fadeEffect struct {
	from colorful.Color // the vivid color the fade starts from each cycle
	bg   colorful.Color // the real terminal background color, faded toward

	step    int // 0..fadeSteps while fading
	pausing bool
	pauseAt int // ticks remaining in the pause phase
}

func newFadeEffect(bg colorful.Color) fadeEffect {
	from, _ := colorful.Hex("#FFD700") // festive gold
	return fadeEffect{from: from, bg: bg}
}

type fadeTickMsg time.Time

func fadeTick() tea.Cmd {
	return tea.Tick(fadeTickInterval, func(t time.Time) tea.Msg {
		return fadeTickMsg(t)
	})
}

func (f fadeEffect) Init() tea.Cmd {
	return fadeTick()
}

func (f fadeEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(fadeTickMsg); !ok {
		return f, nil
	}

	if f.pausing {
		f.pauseAt--
		if f.pauseAt <= 0 {
			f.pausing = false
			f.step = 0
		}
		return f, fadeTick()
	}

	f.step++
	if f.step >= fadeSteps {
		f.step = fadeSteps
		f.pausing = true
		f.pauseAt = pauseTicks
	}
	return f, fadeTick()
}

func (f fadeEffect) View() tea.View {
	t := float64(f.step) / float64(fadeSteps)
	current := f.from.BlendLuv(f.bg, t) // CIE-Luv blend: smoother than a raw RGB lerp

	// Foreground is fixed once, from the starting color's luminance, so the
	// label stays legible even once the background has faded all the way
	// into the terminal's own background.
	fg := lipgloss.Color("#111111")
	lum := 0.2126*f.from.R + 0.7152*f.from.G + 0.0722*f.from.B
	if lum < 0.5 {
		fg = lipgloss.Color("#FFFFFF")
	}

	style := lipgloss.NewStyle().
		Bold(true).
		Padding(1, 3).
		Background(lipgloss.Color(current.Hex())).
		Foreground(fg)

	label := "Fade Out"
	if f.pausing {
		label = "Fade Out (faded)"
	}
	return tea.NewView(style.Render(fmt.Sprintf("%s  %.0f%%  %s", label, t*100, current.Hex())))
}

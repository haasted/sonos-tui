package main

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
)

const (
	breatheFPS        = 20 // a breath takes seconds, so this is ample
	breatheBarWidth   = 44
	breathePeriodMin  = time.Second
	breathePeriodMax  = 15 * time.Second
	breathePeriodStep = 500 * time.Millisecond
)

type breatheDemo struct {
	start  time.Time
	period time.Duration
}

func newBreatheDemo() breatheDemo {
	return breatheDemo{start: time.Now(), period: flashsweep.BreathPeriod}
}

type breatheTickMsg time.Time

func breatheTick() tea.Cmd {
	return tea.Tick(time.Second/breatheFPS, func(t time.Time) tea.Msg { return breatheTickMsg(t) })
}

func (m breatheDemo) Init() tea.Cmd { return breatheTick() }

func (m breatheDemo) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "+", "=":
			m.period = min(m.period+breathePeriodStep, breathePeriodMax)
		case "-", "_":
			m.period = max(m.period-breathePeriodStep, breathePeriodMin)
		}
		return m, nil
	case breatheTickMsg:
		return m, breatheTick()
	}
	return m, nil
}

func (m breatheDemo) View() tea.View {
	// Breath reads one fixed period, so the demo scales elapsed time instead of
	// the curve. A shorter period simply runs the same cycle faster.
	elapsed := time.Duration(float64(time.Since(m.start)) * float64(flashsweep.BreathPeriod) / float64(m.period))
	breath := flashsweep.Breath(elapsed)

	var b strings.Builder
	b.WriteString("\n  " + breatheTitle().Render("a light that breathes") +
		breatheNote().Render(fmt.Sprintf("   %.1fs per cycle", m.period.Seconds())) + "\n\n")

	b.WriteString(m.row("arrows", m.arrows(elapsed)))
	b.WriteString(m.row("background", m.band(elapsed)))
	b.WriteString(m.row("breath", breatheBar(breath, breatheLit())))
	b.WriteString(m.row("a sine", breatheBar(sineAt(elapsed), breatheRest())))

	b.WriteString("\n  " + breatheNote().Render(
		"the sine turns sharply at both ends and never rests; the breath pauses, and fills quicker than it empties") + "\n")
	b.WriteString("  " + breatheNote().Render("+/- period · q quit") + "\n")

	return tea.NewView(b.String())
}

// row lays one labelled line out, so every sample starts in the same column.
func (m breatheDemo) row(label, content string) string {
	return "  " + breatheNote().Render(fmt.Sprintf("%-11s", label)) + content + "\n\n"
}

// arrows are the glyphs the queue list draws at its edges, at the size they are
// really seen: one character, with nothing around them to help.
func (m breatheDemo) arrows(elapsed time.Duration) string {
	lit := lipgloss.NewStyle().Foreground(flashsweep.BreathAt(elapsed, breatheRest(), breatheLit()))
	return lit.Render("↑") + "        " + lit.Render("↓")
}

// band is a block whose background breathes, which is the same curve driving a
// larger area. A wide area carries a colour further than one glyph does, so the
// two ends are closer together here than they are for the arrows.
func (m breatheDemo) band(elapsed time.Duration) string {
	bg := flashsweep.BreathAt(elapsed, breatheBandRest(), breatheBandLit())

	return lipgloss.NewStyle().Background(bg).Foreground(breatheLit()).
		Width(breatheBarWidth).Align(lipgloss.Center).Render("Now Playing")
}

// breatheBar draws a value from 0 to 1 as a filled bar, so the shape of a curve
// is visible over time rather than only its colour.
func breatheBar(v float32, on color.Color) string {
	filled := int(v*breatheBarWidth + 0.5)
	return lipgloss.NewStyle().Foreground(on).Render(strings.Repeat("█", filled)) +
		breatheNote().Render(strings.Repeat("░", breatheBarWidth-filled)) +
		breatheNote().Render(fmt.Sprintf("  %.3f", v))
}

// sineAt is the obvious alternative, drawn beside the breath for comparison.
func sineAt(elapsed time.Duration) float32 {
	t := float64(elapsed%flashsweep.BreathPeriod) / float64(flashsweep.BreathPeriod)
	return float32((1 - math.Cos(2*math.Pi*t)) / 2)
}

func breatheTitle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(themedColor("#B45309", "#FBBF24"))
}

func breatheNote() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(themedColor("#9CA3AF", "#6B7280"))
}

func breatheRest() color.Color { return themedColor("#C4C4C4", "#5A5A5A") }

func breatheLit() color.Color { return themedColor("#0F172A", "#FFFFFF") }

func breatheBandRest() color.Color { return themedColor("#E9E9E9", "#1A1A1A") }

func breatheBandLit() color.Color { return themedColor("#93C5FD", "#1D4ED8") }

func layoutBreathe() {
	if _, err := tea.NewProgram(newBreatheDemo()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

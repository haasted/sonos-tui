package main

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/ui/theme"
)

const (
	volDemoRowWidth    = 50 // mirrors nowplaying.titleRowWidth
	volDemoTitleWidth  = 35 // mirrors nowplaying.longTitleWindowWidth
	volDemoHoldTime    = 1400 * time.Millisecond
	volDemoFadeTime    = 600 * time.Millisecond
	volDemoFrameRate   = 40 * time.Millisecond
	volDemoBrailleCell = 5  // braille cells in the bar
	volDemoBlockCells  = 10 // block cells in the bar
	volDemoMuteFade    = 1200 * time.Millisecond
	volDemoMuteHold    = 250 * time.Millisecond // full flash before the fade starts
)

// volStyle names the readout variants the demo cycles through with "v".
type volStyle int

const (
	volArrowLevel volStyle = iota
	volLabelLevel
	volFineBar
	volFineBarArrow
	volFineBarBare
	volBraille
	volStyleCount
)

func (s volStyle) String() string {
	return [...]string{
		"arrow + level", "label + level", "fine bar + level",
		"arrow + fine bar + level", "fine bar, no track", "braille bar + level",
	}[s]
}

// volumeDemo renders the real nowplaying title row with the volume readout
// standing in for the clock, so the look can be judged in context.
type volumeDemo struct {
	volume     int
	style      volStyle
	changedAt  time.Time // zero once the readout has fully faded
	muted      bool
	elapsed    time.Duration
	fadeFrames bool
	mutedAt    time.Time   // when mute was last switched on, for the highlight fade
	eased      bool        // smoothstep the fades instead of ramping them linearly
	bg         color.Color // the terminal's real background, target of the mute flash fade
	rising     bool        // direction of the last volume change, for the arrow
}

func newVolumeDemo() volumeDemo {
	return volumeDemo{volume: 46, elapsed: 73 * time.Second, fadeFrames: true, eased: true}
}

// brailleBar fills 2 columns per cell, bottom-up, per the table in pkg/ui/spinners.
func brailleBar(volume, cells int) string {
	cols := (volume*cells*2 + 50) / 100
	var b strings.Builder
	for c := range cells {
		switch filled := cols - c*2; {
		case filled >= 2:
			b.WriteString("⣿")
		case filled == 1:
			b.WriteString("⡇")
		default:
			b.WriteString("⣀")
		}
	}
	return b.String()
}

// fineBar uses eighth-width block characters, so each cell carries 8 sub-steps
// and a 2-point volume change is visible on nearly every keypress.
func fineBar(volume, cells int, track bool) string {
	eighths := (volume * cells * 8) / 100
	full, rem := eighths/8, eighths%8

	var b strings.Builder
	b.WriteString(strings.Repeat("█", full))
	used := full
	if rem > 0 {
		b.WriteString(string([]rune("▏▎▍▌▋▊▉")[rem-1]))
		used++
	}
	pad := " "
	if track {
		pad = "░"
	}
	b.WriteString(strings.Repeat(pad, max(cells-used, 0)))
	return b.String()
}

func (m volumeDemo) readout() string {
	arrow := "▼"
	if m.rising {
		arrow = "▲"
	}
	switch m.style {
	case volArrowLevel:
		return fmt.Sprintf("%s %d", arrow, m.volume)
	case volLabelLevel:
		return fmt.Sprintf("vol %d", m.volume)
	case volFineBar:
		return fmt.Sprintf("%s %d", fineBar(m.volume, 6, true), m.volume)
	case volFineBarArrow:
		return fmt.Sprintf("%s %s %d", arrow, fineBar(m.volume, 6, true), m.volume)
	case volFineBarBare:
		return fmt.Sprintf("%s %d", fineBar(m.volume, 6, false), m.volume)
	default:
		return fmt.Sprintf("%s %d", brailleBar(m.volume, volDemoBrailleCell), m.volume)
	}
}

// muteHighlight is 1 while held, then ramps to 0 across the fade.
func (m volumeDemo) muteHighlight() float64 {
	if m.mutedAt.IsZero() {
		return 0
	}
	since := time.Since(m.mutedAt)
	if since < volDemoMuteHold {
		return 1
	}
	left := volDemoMuteHold + volDemoMuteFade - since
	if left <= 0 {
		return 0
	}
	return m.shape(float64(left) / float64(volDemoMuteFade))
}

// shape eases a 0-1 ramp so it leaves and arrives gently instead of linearly.
func (m volumeDemo) shape(t float64) float64 {
	if !m.eased {
		return t
	}
	return t * t * (3 - 2*t)
}

// fadeAmount is 1 while held and ramps to 0 across the fade, or -1 once done.
func (m volumeDemo) fadeAmount() float64 {
	if m.changedAt.IsZero() {
		return -1
	}
	since := time.Since(m.changedAt)
	if since < volDemoHoldTime {
		return 1
	}
	if !m.fadeFrames {
		if since < volDemoHoldTime+volDemoFadeTime {
			return 1
		}
		return -1
	}
	left := volDemoHoldTime + volDemoFadeTime - since
	if left <= 0 {
		return -1
	}
	return m.shape(float64(left) / float64(volDemoFadeTime))
}

type volDemoTick time.Time

func volDemoTickCmd() tea.Cmd {
	return tea.Tick(volDemoFrameRate, func(t time.Time) tea.Msg { return volDemoTick(t) })
}

func (m volumeDemo) Init() tea.Cmd { return tea.Batch(volDemoTickCmd(), tea.RequestBackgroundColor) }

func (m volumeDemo) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case volDemoTick:
		return m, volDemoTickCmd()
	case tea.BackgroundColorMsg:
		m.bg = msg.Color
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up":
			m.volume = min(m.volume+2, 100)
			m.changedAt = time.Now()
			m.rising = true
			m.muted = false
		case "down":
			m.volume = max(m.volume-2, 0)
			m.changedAt = time.Now()
			m.rising = false
			m.muted = false
		case "m", "M":
			m.muted = !m.muted
			if m.muted {
				m.mutedAt = time.Now()
			}
		case "v", "V":
			m.style = (m.style + 1) % volStyleCount
			m.changedAt = time.Now()
		case "f", "F":
			m.fadeFrames = !m.fadeFrames
			m.changedAt = time.Now()
		case "e", "E":
			m.eased = !m.eased
			m.changedAt = time.Now()
			m.mutedAt = time.Now()
		}
	}
	return m, nil
}

// row builds one title line exactly as nowplaying.View does: glyph, space,
// fixed-width title, then a right-aligned slot.
func row(glyph, title, right string, rightStyle lipgloss.Style) string {
	left := glyph + " " + lipgloss.NewStyle().Bold(true).Render(padRight(title, volDemoTitleWidth))
	gap := max(volDemoRowWidth-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return left + strings.Repeat(" ", gap) + rightStyle.Render(right)
}

func (m volumeDemo) View() tea.View {
	glyph := "⣶"
	if m.muted {
		glyph = m.muteGlyphStyle(m.muteHighlight()).Render("X")
	}

	right, style := formatPositionDemo(m.elapsed), dimStyle()
	if fade := m.fadeAmount(); fade >= 0 {
		right, style = m.readout(), fadedAccent(fade)
	}

	var b strings.Builder
	b.WriteString("\n  " + row(glyph, "You Belong to Me", right, style) + "\n")
	b.WriteString("    " + dimStyle().Render("Bob Dylan") + "\n")
	b.WriteString("    " + dimStyle().Render("Natural Born Killers") + "\n")
	b.WriteString("  " + dimStyle().Render(strings.Repeat("─", volDemoRowWidth)) + "\n")

	b.WriteString("\n  " + dimStyle().Render("ladder — every level in this style:") + "\n")
	for _, v := range []int{0, 10, 25, 40, 46, 50, 60, 73, 88, 95, 100} {
		probe := volumeDemo{volume: v, style: m.style, rising: m.rising}
		marker := "  "
		if v == m.volume {
			marker = "▸ "
		}
		b.WriteString(fmt.Sprintf("  %s%3d  %s\n", marker, v, accentStyle().Render(probe.readout())))
	}

	fadeLabel := "on"
	if !m.fadeFrames {
		fadeLabel = "off (abrupt)"
	}
	easeLabel := "smoothstep"
	if !m.eased {
		easeLabel = "linear"
	}
	b.WriteString("\n  " + dimStyle().Render(fmt.Sprintf(
		"style: %s   fade: %s   curve: %s   bg: %s\n  ↑/↓ volume · m mute · v style · f fade · e ease · q quit",
		m.style, fadeLabel, easeLabel, bgLabel(m.bg))) + "\n")

	return tea.NewView(b.String())
}

// bgLabel reports whether the terminal answered the background-colour query.
func bgLabel(c color.Color) string {
	if c == nil {
		return "not reported (using themed guess)"
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}

func formatPositionDemo(d time.Duration) string {
	return fmt.Sprintf("%d:%02d / 3:09", int(d/time.Minute), int((d%time.Minute)/time.Second))
}

func dimStyle() lipgloss.Style {
	return lipgloss.NewStyle().Faint(true).
		Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))
}

func accentStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24")))
}

// lerpHex blends from -> to as amount goes 0 -> 1, both as {R,G,B}.
func lerpHex(from, to [3]float64, amount float64) color.Color {
	r := from[0] + (to[0]-from[0])*amount
	g := from[1] + (to[1]-from[1])*amount
	b := from[2] + (to[2]-from[2])*amount
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", int(r), int(g), int(b)))
}

// fadedAccent interpolates the accent toward the dim colour as amount goes 1 -> 0.
func fadedAccent(amount float64) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(
		lerpHex([3]float64{0x9B, 0x9B, 0x9B}, [3]float64{0xFB, 0xBF, 0x24}, amount))
}

// muteGlyphStyle keeps the glyph white and flashes its BACKGROUND red on
// activation, fading to the terminal's own background so the block disappears.
func (m volumeDemo) muteGlyphStyle(highlight float64) lipgloss.Style {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	if highlight <= 0 {
		return lipgloss.NewStyle() // fully settled: no styling at all
	}
	flash := [3]float64{0xC0, 0x1C, 0x28}
	return style.Background(lerpHex(m.restBackground(), flash, highlight))
}

// restBackground is the terminal's reported background, or a themed guess.
func (m volumeDemo) restBackground() [3]float64 {
	if m.bg != nil {
		r, g, b, _ := m.bg.RGBA()
		return [3]float64{float64(r >> 8), float64(g >> 8), float64(b >> 8)}
	}
	r, g, b, _ := theme.LightDark(lipgloss.Color("#FFFFFF"), lipgloss.Color("#000000")).RGBA()
	return [3]float64{float64(r >> 8), float64(g >> 8), float64(b >> 8)}
}

func layoutVolume() {
	if _, err := tea.NewProgram(newVolumeDemo()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

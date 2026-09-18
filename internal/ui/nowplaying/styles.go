package nowplaying

import (
	"fmt"
	"image/color"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

const (
	titleRowWidth        = 50
	longTitleWindowWidth = 35
)

// pausedGlyph is the pause mark every player uses, which a reader knows
// without being taught. It leaves the spinner's braille family for that
// reason. One cell only, as mutedGlyph explains.
const pausedGlyph = "‖"

// mutedGlyph replaces the playing/paused glyph while muted. It leaves the
// braille family on purpose: a muted speaker is not the equalizer at rest, it
// is sound refused. One cell only: View derives its indent from this width.
const mutedGlyph = "⊘"

// breathInterval is how often a resting glyph repaints. A breath takes five
// seconds, so this is already more than the eye asks for.
const breathInterval = 50 * time.Millisecond

const (
	volumeHold = 1400 * time.Millisecond // readout stays at full strength this long
	volumeFade = 600 * time.Millisecond  // then dims away, restoring the clock
)

const (
	volumeUpGlyph   = "▲"
	volumeDownGlyph = "▼"
)

// volumeSlotWidth is the room the readout needs: the widest form ("▼ 100")
// plus a separating space. Only claimed from the album while showing, so a
// long album title keeps the full width the rest of the time.
const volumeSlotWidth = 6

const (
	scrollInterval = 150 * time.Millisecond
	scrollDelay    = 3 * time.Second
)

const scrollDelayTicks = int(scrollDelay / scrollInterval)

var (
	trackStyle = lipgloss.NewStyle().Bold(true)

	artistStyle = lipgloss.NewStyle().
			Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))

	albumStyle = lipgloss.NewStyle().
			Faint(true).
			Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))

	timeStyle = lipgloss.NewStyle().
			Faint(true).
			Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))

	volumeAccent = theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24"))
)

// volumeStyle fades the readout from the accent into the terminal background
// as strength goes 1 -> 0, so it is already invisible when it stops rendering.
func volumeStyle(strength float64) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(blend(terminalBackground(), volumeAccent, strength))
}

// restingGlyphStyle breathes the glyph of a speaker that waits, from a tone
// near the dim rows below it up to lit. A wide swing on purpose, for the
// reason the queue arrows need one: the glyph is a single character.
func restingGlyphStyle(elapsed time.Duration, lit color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(flashsweep.BreathAt(elapsed, restingBreathBase(), lit))
}

func restingBreathBase() color.Color {
	return theme.LightDark(lipgloss.Color("#C4C4C4"), lipgloss.Color("#5A5A5A"))
}

// pausedBreathLit deepens on a light ground and brightens on a dark one, the
// pair every other light in this program breathes toward.
func pausedBreathLit() color.Color {
	return theme.LightDark(lipgloss.Color("#0F172A"), lipgloss.Color("#FFFFFF"))
}

// mutedBreathLit keeps the red the removed flash used. Silence is the one
// resting state worth a warning colour.
func mutedBreathLit() color.Color {
	return theme.LightDark(lipgloss.Color("#C01C28"), lipgloss.Color("#F87171"))
}

// terminalBackground is the terminal's own background, or a themed stand-in
// when it did not report one. Both transients fade toward it so they are
// already invisible by the time they stop being drawn.
func terminalBackground() color.Color {
	if bg := theme.Background(); bg != nil {
		return bg
	}
	return theme.LightDark(lipgloss.Color("#FFFFFF"), lipgloss.Color("#000000"))
}

// blend mixes from -> to as amount goes 0 -> 1.
func blend(from, to color.Color, amount float64) color.Color {
	fr, fg, fb, _ := from.RGBA()
	tr, tg, tb, _ := to.RGBA()
	mix := func(a, b uint32) int {
		return int((float64(a>>8) + (float64(b>>8)-float64(a>>8))*amount))
	}
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X",
		mix(fr, tr), mix(fg, tg), mix(fb, tb)))
}

// streamLabelStyle is the resting look of the "live"/"radio"/"stream" label:
// the same dim tone the clock it replaces uses, so the row does not shift in
// weight when playback changes kind.
func streamLabelStyle() lipgloss.Style { return timeStyle }

// streamLabelRamp is the glint that travels the label. It brightens from the
// label's own resting tone toward the accent already used for volume and the
// active tab, so the pulse reads as this app rather than as an alarm.
func streamLabelRamp() flashsweep.Ramp {
	return flashsweep.Ramp{
		Base:  flashsweep.FromColor(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B"))),
		Flash: flashsweep.FromColor(volumeAccent),
	}
}

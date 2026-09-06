package nowplaying

import (
	"fmt"
	"image/color"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

const (
	titleRowWidth        = 50
	longTitleWindowWidth = 35
)

const pausedGlyph = "⣿"

// mutedGlyph replaces the playing/paused glyph while muted. One cell only:
// View derives its indent from this width. Swap freely to taste.
const mutedGlyph = "X"

const (
	muteFlashHold = 250 * time.Millisecond  // full-strength flash before it starts fading
	muteFlashFade = 1200 * time.Millisecond // fade from flash to the terminal background
)

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

	errorStyle = lipgloss.NewStyle().
			Foreground(theme.LightDark(lipgloss.Color("#B00020"), lipgloss.Color("#FF6B6B")))

	muteFlashColor = lipgloss.Color("#C01C28") // background the mute glyph flashes on activation
	muteGlyphColor = lipgloss.Color("#FFFFFF") // glyph stays legible against the flash
)

// muteGlyphStyle keeps the glyph legible and flashes its background, fading
// that background toward the terminal's own so the block disappears.
func muteGlyphStyle(strength float64) lipgloss.Style {
	if strength <= 0 {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().
		Foreground(muteGlyphColor).
		Background(blend(muteRestColor(), muteFlashColor, strength))
}

// muteRestColor is the terminal's background, or a themed stand-in when it
// did not report one.
func muteRestColor() color.Color {
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

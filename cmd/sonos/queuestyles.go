// The colours and marks the queue is drawn with, apart from the list itself.
// internal/ui/nowplaying keeps the same division, and for the same reason: a
// reader changing how something looks should not have to read how it works.
package main

import (
	"image/color"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

const (
	// bumpVisibleAt is the point below which a rune is left in the border's own
	// colour, rather than painted a shade indistinguishable from it.
	bumpVisibleAt = 0.08

	// bumpHeavyAt is where the line thickens. Colour alone spreads the pulse
	// into a smear of close shades; the weight gives it an edge the eye finds
	// without looking for it.
	bumpHeavyAt = 0.35
)

func queueRoomStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).
		Foreground(theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24")))
}

func queueRuleStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.LightDark(lipgloss.Color("#D0D0D0"), lipgloss.Color("#4A4A4A")))
}

func queueRowStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.LightDark(lipgloss.Color("#1F2937"), lipgloss.Color("#E5E7EB")))
}

func queuePlayingStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).
		Foreground(theme.LightDark(lipgloss.Color("#1D4ED8"), lipgloss.Color("#93C5FD")))
}

// queueArrowStyle breathes the arrow from near the colour of the rule to the
// far end of the palette. A wide swing on purpose: the arrow is one character,
// so a narrow one is only found by a reader who already knows to look.
func queueArrowStyle(since time.Time) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(flashsweep.BreathAt(time.Since(since), themedArrowRest(), themedArrowLit()))
}

func themedArrowRest() color.Color {
	return theme.LightDark(lipgloss.Color("#C4C4C4"), lipgloss.Color("#5A5A5A"))
}

// themedArrowLit brightens on a dark ground and deepens on a light one. The
// same pair the notice entrance uses, for the same reason: a light that flashes
// toward white on white erases itself.
func themedArrowLit() color.Color {
	return theme.LightDark(lipgloss.Color("#0F172A"), lipgloss.Color("#FFFFFF"))
}

// themedBumpBase and themedBumpPeak are the pulse's two colours. Sky into white
// on a dark ground, and the same pair inverted on a light one, where a white
// front would vanish into the page.
func themedBumpBase() color.Color {
	return theme.LightDark(lipgloss.Color("#0369A1"), lipgloss.Color("#38BDF8"))
}

func themedBumpPeak() color.Color {
	return theme.LightDark(lipgloss.Color("#0F172A"), lipgloss.Color("#FFFFFF"))
}

func queueNoteStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))
}

// bumpColour is one rune of the pulse: the scheme's own base at the faint end
// of the light, and white at its front.
//
// The blend never passes through the border's resting colour. That colour is
// amber, and a gradient from amber to blue crosses grey — the pulse would read
// as the border going muddy rather than as a light on it. Below bumpVisibleAt
// the rune is left alone instead, so the handover is a step and not a smear.
func bumpColour(lit float32) color.Color {
	base := flashsweep.FromColor(themedBumpBase())
	peak := flashsweep.FromColor(themedBumpPeak())

	mixed := base.Lerp(peak, smoothstep01(bumpVisibleAt, 1, lit))
	return lipgloss.Color(mixed.ToRgb().String())
}

// smoothstep01 eases between two edges, so the front of the pulse rounds into
// white rather than arriving at it in a straight line.
func smoothstep01(edge0, edge1, x float32) float32 {
	t := min(max((x-edge0)/(edge1-edge0), 0), 1)
	return t * t * (3 - 2*t)
}

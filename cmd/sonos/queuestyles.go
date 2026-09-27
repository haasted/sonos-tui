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

func themedRowColor() color.Color {
	return theme.LightDark(lipgloss.Color("#1F2937"), lipgloss.Color("#E5E7EB"))
}

func queueRowStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(themedRowColor())
}

func themedPlayingColor() color.Color {
	return theme.LightDark(lipgloss.Color("#1D4ED8"), lipgloss.Color("#93C5FD"))
}

func queuePlayingStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(themedPlayingColor())
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

// themedSelectedBackground marks a row the reader has picked out. Background
// rather than foreground because the foreground already carries two meanings
// here -- blue for the track playing, amber for one that just landed -- and a
// selected row has to be able to be either of those at the same time.
//
// Neutral on purpose, for the same reason: a tinted grey reads as "marked"
// without arguing with the two colours that mean something.
func themedSelectedBackground() color.Color {
	return theme.LightDark(lipgloss.Color("#D4D4D8"), lipgloss.Color("#3F3F46"))
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

// themedHighlightPeak is the flash's own colour, deliberately not
// themedBumpPeak's white: a row's resting colour in dark mode (themedRowColor)
// is already close to white, so brightening toward white there is the same
// mistake themedArrowLit's own comment warns against, just missed here first
// time round. A saturated accent that neither the resting grey nor the
// playing blue is anywhere near reads as a real change from both, in either
// theme. Reuses queueRoomStyle's amber rather than inventing a third colour.
func themedHighlightPeak() color.Color {
	return theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24"))
}

// highlightColour blends a row's own colour toward the flash's peak, so a
// track that just moved lights up without losing whether it is the one
// playing.
func highlightColour(base color.Color, glow float32) color.Color {
	from := flashsweep.FromColor(base)
	peak := flashsweep.FromColor(themedHighlightPeak())

	mixed := from.Lerp(peak, smoothstep01(0, 1, glow))
	return lipgloss.Color(mixed.ToRgb().String())
}

// smoothstep01 eases between two edges, so the front of the pulse rounds into
// white rather than arriving at it in a straight line.
func smoothstep01(edge0, edge1, x float32) float32 {
	t := min(max((x-edge0)/(edge1-edge0), 0), 1)
	return t * t * (3 - 2*t)
}

// queueCountStyle draws the selection count in the heading. Quieter than the
// room name beside it, and not bold: the room says what the reader is looking
// at, where the count answers a question they ask only now and then.
//
// Deliberately the same grey as queueNoteStyle rather than a fourth colour.
// The heading already carries amber, and the list below it blue and amber
// again; a new hue here would mean something it does not.
func queueCountStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))
}

// themedGoingColor draws a track the reader has removed and the speaker has
// not yet confirmed gone. Dimmer than a resting row in both themes: it is on
// its way out, so it should read as less present than what is staying, not
// as another kind of important.
func themedGoingColor() color.Color {
	return theme.LightDark(lipgloss.Color("#9CA3AF"), lipgloss.Color("#6B7280"))
}

// queueFilterPromptStyle draws the key that opened the filter box, which
// stays on screen while it is open so a reader who has not typed yet can see
// that they may.
//
// Quieter than the room name and not bold, like the selection count: it is
// the prompt rather than anything the reader put there. Told apart from what
// they type, which keeps the room name's own colour.
func queueFilterPromptStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))
}

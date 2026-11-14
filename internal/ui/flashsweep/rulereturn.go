package flashsweep

import (
	"math/rand"
	"strings"
	"time"

	"github.com/haasted/sonos-tui/internal/ui/movy"
)

// ReturnStyle selects how a rule comes back into the gap a notice left in it.
// Every style ends on the same unbroken rule; they differ in the order the
// cells come back and in what the rule looks like on the way.
type ReturnStyle int

const (
	// ReturnFade brings the whole gap up from the backdrop at once.
	ReturnFade ReturnStyle = iota
	// ReturnCloseIn regrows the rule from both ends of the gap to the middle.
	ReturnCloseIn
	// ReturnOpenOut grows the rule from the middle out to both ends.
	ReturnOpenOut
	// ReturnSweep draws the rule from one end to the other, like a pen.
	ReturnSweep
	// ReturnDissolve fills the cells in a scattered order.
	ReturnDissolve
	// ReturnGlint fades the rule up while a light runs along it.
	ReturnGlint
	// ReturnStitch firms the rule up from dotted, through dashed, to solid.
	ReturnStitch
)

// ReturnStyles is every style in declaration order, for callers that pick one.
var ReturnStyles = []ReturnStyle{
	ReturnFade, ReturnCloseIn, ReturnOpenOut, ReturnSweep, ReturnDissolve, ReturnGlint, ReturnStitch,
}

func (s ReturnStyle) String() string {
	switch s {
	case ReturnCloseIn:
		return "close in"
	case ReturnOpenOut:
		return "open out"
	case ReturnSweep:
		return "sweep"
	case ReturnDissolve:
		return "dissolve"
	case ReturnGlint:
		return "glint"
	case ReturnStitch:
		return "stitch"
	default:
		return "fade"
	}
}

const (
	// returnCellTime is how long each cell of the gap adds to a return, so a
	// long gap takes longer to close than a short one. returnShortest keeps a
	// gap of a few cells from closing in a frame or two.
	returnCellTime = 14 * time.Millisecond
	returnShortest = 160 * time.Millisecond

	// returnSoft is how wide a growing front is, as a part of the gap: the
	// cells just behind it are still coming up from the backdrop.
	returnSoft = 0.3

	// returnGlintFade is how much of a glint return the rule takes to come up,
	// before the light has run on to the far end.
	returnGlintFade = 0.5
)

// stitchGlyphs are the rule's stages in ReturnStitch, from faintest to solid.
// The solid stage is RuleReturn.Rule itself.
var stitchGlyphs = []rune{'┈', '╌'}

// ReturnDuration is how long a gap of width cells takes to close.
func ReturnDuration(width int) time.Duration {
	return max(returnShortest, time.Duration(width)*returnCellTime)
}

// RuleReturn draws a run of rule coming back into a gap. Like Reveal, it
// renders the frame for a given progress rather than owning a timer.
type RuleReturn struct {
	Style    ReturnStyle
	Rule     rune    // the rule's glyph, such as '─'
	Color    movy.V3 // the rule's resting colour
	Backdrop movy.V3 // the ground the gap shows before the rule is back
	Flash    movy.V3 // the light ReturnGlint runs along the rule
	Seed     int64   // picks the dissolve's order and the sweep's direction
}

// Draw renders width cells of rule at a point between gone (0) and back (1).
func (r RuleReturn) Draw(width int, progress float32) string {
	if width <= 0 {
		return ""
	}
	progress = clamp(progress, 0, 1)
	order := r.order(width)

	var b strings.Builder
	for i := range width {
		switch r.Style {
		case ReturnGlint:
			r.glintCell(&b, i, width, progress)
		case ReturnStitch:
			r.stitchCell(&b, order[i], progress)
		default:
			r.growCell(&b, r.presence(order[i], progress))
		}
	}
	b.WriteString(reset)
	return b.String()
}

// order is, for each cell, how late it comes back: 0 for the first cell to
// return and 1 for the last. Fade and glint bring every cell at once.
func (r RuleReturn) order(width int) []float32 {
	order := make([]float32, width)
	if width == 1 {
		return order
	}
	half := float32(width-1) / 2
	ranks := rand.New(rand.NewSource(r.Seed)).Perm(width)
	for i := range order {
		switch r.Style {
		case ReturnCloseIn:
			order[i] = 1 - abs32(float32(i)-half)/half
		case ReturnOpenOut, ReturnStitch:
			order[i] = abs32(float32(i)-half) / half
		case ReturnSweep:
			order[i] = float32(i) / float32(width-1)
			if r.Seed%2 != 0 {
				order[i] = 1 - order[i]
			}
		case ReturnDissolve:
			order[i] = float32(ranks[i]) / float32(width-1)
		}
	}
	return order
}

// presence is how far a cell has come back, given how late it is due: a soft
// front passes over the cells in order, and fade has every cell due at once.
func (r RuleReturn) presence(late, progress float32) float32 {
	if r.Style == ReturnFade {
		return progress
	}
	return clamp((progress*(1+returnSoft)-late)/returnSoft, 0, 1)
}

// growCell draws one cell that is present by a: nothing yet, or the rule
// coming up from the backdrop.
func (r RuleReturn) growCell(b *strings.Builder, a float32) {
	if a <= 0 {
		b.WriteByte(' ')
		return
	}
	writeCell(b, r.Backdrop.Lerp(r.Color, a), r.Rule)
}

// glintCell draws the rule fading up across the whole gap, lit by a light
// running from one end to the other.
func (r RuleReturn) glintCell(b *strings.Builder, i, width int, progress float32) {
	a := clamp(progress/returnGlintFade, 0, 1)
	if a <= 0 {
		b.WriteByte(' ')
		return
	}
	span := float32(width-1) + 2*glintTail
	light := -glintTail + progress*span
	col := r.Backdrop.Lerp(r.Color, a).Lerp(r.Flash, GlintBurst(float32(i)-light))
	writeCell(b, col, r.Rule)
}

// stitchCell draws a cell passing through the stitch glyphs to the rule, the
// middle a little ahead of the ends so the line firms up from its centre.
func (r RuleReturn) stitchCell(b *strings.Builder, late, progress float32) {
	t := clamp(progress*(1+returnSoft)-late*returnSoft, 0, 1)
	if t <= 0 {
		b.WriteByte(' ')
		return
	}
	stage := int(t * float32(len(stitchGlyphs)+1))
	glyph := r.Rule
	if stage < len(stitchGlyphs) {
		glyph = stitchGlyphs[stage]
	}
	writeCell(b, r.Backdrop.Lerp(r.Color, 0.4+0.6*t), glyph)
}

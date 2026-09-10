package flashsweep

import (
	"strings"
	"time"

	"github.com/haasted/sonos-sandbox/internal/ui/movy"
)

// RevealStyle selects how a Reveal opens. Every style starts at the centre of
// the text and travels outward to both ends; they differ in what they do with
// colour, and one of them moves the runes themselves.
type RevealStyle int

const (
	// RevealMirror is Line folded in half — the same reveal band and hot core,
	// with two wavefronts leaving the centre rather than one crossing the line.
	RevealMirror RevealStyle = iota
	// RevealPrism keeps mirror's geometry and gives the light a hue that travels
	// with it, so the wavefronts trail colour instead of white.
	RevealPrism
	// RevealBloom opens at full brightness and then cools into the resting
	// colour — a flash first and a reveal second.
	RevealBloom
	// RevealSplit starts every rune stacked on the centre column and slides them
	// out to their real positions, so the text physically opens.
	RevealSplit
)

// RevealStyles is every style in declaration order, for callers offering a choice.
var RevealStyles = []RevealStyle{RevealMirror, RevealPrism, RevealBloom, RevealSplit}

func (s RevealStyle) String() string {
	switch s {
	case RevealPrism:
		return "prism"
	case RevealBloom:
		return "bloom"
	case RevealSplit:
		return "split"
	default:
		return "mirror"
	}
}

const (
	// DefaultRevealSpeed is how fast each wavefront travels, in cells per
	// second, when Reveal.Speed is left at zero.
	DefaultRevealSpeed = 24

	// openFlashWeight is how far the hot core pushes toward Flash. It sits
	// above Line's flashWeight because a centre-out core crosses any given rune
	// in half the time and needs the extra amplitude to register.
	openFlashWeight = 0.75

	// bloomSplit is where RevealBloom stops opening and starts cooling.
	bloomSplit = 0.55

	// prismSpread is how much of the colour wheel the text spans, and prismDrift
	// how far the whole band rotates over one opening.
	prismSpread = 0.75
	prismDrift  = 0.45

	// splitEmerge is how much of the opening RevealSplit spends fading its
	// stack up from nothing. Without it the stack stands at full brightness at
	// progress 0, stranding a lone rune where the text has not opened yet.
	splitEmerge = 0.15
)

// Reveal is a one-shot entrance for a short label: the text opens from its
// centre outward in the chosen style. It renders a frame for a given progress
// rather than owning a timer, so it composes into an animation loop the caller
// already runs — the same contract as Glint.
type Reveal struct {
	Style RevealStyle
	Ramp  Ramp
	Speed float32 // cells per second each wavefront travels; zero means DefaultRevealSpeed

	// Fill is drawn in the cells the text has not claimed yet, so an entrance
	// can part something rather than appear on a gap: a divider passes its rule
	// rune here and the text opens the rule and closes it again. Zero leaves
	// those cells blank. FillColor is required whenever Fill is set — the zero
	// value is black, which on most grounds reads as a hole.
	Fill      rune
	FillColor movy.V3
}

// Open renders text at a point between hidden (0) and fully open (1).
func (r Reveal) Open(text string, progress float32) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}
	progress = clamp(progress, 0, 1)

	switch r.Style {
	case RevealPrism:
		return r.prism(runes, progress)
	case RevealBloom:
		return r.bloom(runes, progress)
	case RevealSplit:
		return r.split(runes, progress)
	default:
		return r.mirror(runes, progress)
	}
}

// Duration is how long text takes to open at this Reveal's speed. It scales
// with half the text, since both wavefronts travel at once.
func (r Reveal) Duration(text string) time.Duration {
	runes := []rune(text)
	if len(runes) == 0 {
		return 0
	}
	return time.Duration(float64(openSpan(len(runes))/r.speed()) * float64(time.Second))
}

// Close is the entrance run backwards: at progress 0 the text stands fully
// open, at 1 it has collapsed back to its centre. With Fill set the rule closes
// over the text as it retreats, so the divider is whole again at the end rather
// than snapping back to it.
func (r Reveal) Close(text string, progress float32) string {
	return r.Open(text, 1-clamp(progress, 0, 1))
}

func (r Reveal) speed() float32 {
	if r.Speed <= 0 {
		return DefaultRevealSpeed
	}
	return r.Speed
}

func (r Reveal) mirror(runes []rune, progress float32) string {
	light := openLight(len(runes), progress)

	var b strings.Builder
	for i, ch := range runes {
		d := aheadOfLight(i, len(runes), light)

		reveal := 1 - smoothstep(-revealWidth, revealWidth, d)
		if reveal < visibleAt {
			r.writeFill(&b)
			continue
		}

		col := r.Ramp.Base.Scale(dimFloor).
			Lerp(r.Ramp.Base, reveal).
			Lerp(r.Ramp.Flash, gauss(d, sigma)*openFlashWeight)
		writeCell(&b, col, ch)
	}
	b.WriteString(reset)
	return b.String()
}

func (r Reveal) prism(runes []rune, progress float32) string {
	light := openLight(len(runes), progress)

	var b strings.Builder
	for i, ch := range runes {
		d := aheadOfLight(i, len(runes), light)

		reveal := 1 - smoothstep(-revealWidth, revealWidth, d)
		if reveal < visibleAt {
			r.writeFill(&b)
			continue
		}

		hue := wrap01(float32(i)/float32(len(runes))*prismSpread + progress*prismDrift)
		tint := r.Ramp.Base.Lerp(hsv(hue, 0.8, 1), 0.5*reveal)

		col := tint.Scale(dimFloor).Lerp(tint, reveal).Lerp(hsv(hue, 0.45, 1), gauss(d, sigma))
		writeCell(&b, col, ch)
	}
	b.WriteString(reset)
	return b.String()
}

func (r Reveal) bloom(runes []rune, progress float32) string {
	var (
		open  = clamp(progress/bloomSplit, 0, 1)
		cool  = smoothstep(bloomSplit, 1, progress)
		light = openLight(len(runes), open)

		b strings.Builder
	)

	for i, ch := range runes {
		d := aheadOfLight(i, len(runes), light)

		reveal := 1 - smoothstep(-revealWidth, revealWidth, d)
		if reveal < visibleAt {
			r.writeFill(&b)
			continue
		}

		col := r.Ramp.Flash.Lerp(r.Ramp.Base, cool).Scale(0.35 + 0.65*reveal)
		writeCell(&b, col, ch)
	}
	b.WriteString(reset)
	return b.String()
}

func (r Reveal) split(runes []rune, progress float32) string {
	var (
		centre = float32(len(runes)-1) / 2
		t      = easeOutCubic(progress)

		cells = make([]rune, len(runes)) // zero marks a column no rune has claimed
		cols  = make([]movy.V3, len(runes))
	)

	emerge := smoothstep(0, splitEmerge, progress)
	if emerge < visibleAt {
		return r.unclaimed(len(runes))
	}

	for i, ch := range runes {
		at := round(centre + (float32(i)-centre)*t)
		if at < 0 || at >= len(cells) {
			continue
		}
		if ch == ' ' && cells[at] != 0 {
			continue // a space must not stamp out a rune already stacked there
		}
		cells[at] = ch
		cols[at] = r.Ramp.Flash.Lerp(r.Ramp.Base, t).Scale(dimFloor + (1-dimFloor)*emerge)
	}

	var b strings.Builder
	for i, ch := range cells {
		if ch == 0 {
			r.writeFill(&b)
			continue
		}
		writeCell(&b, cols[i], ch)
	}
	b.WriteString(reset)
	return b.String()
}

// openSpan is how far each wavefront travels: half the text, plus a band either
// end so the centre fades up rather than popping in and the last rune finishes.
func openSpan(n int) float32 {
	return float32(n-1)/2 + 2*revealWidth
}

func openLight(n int, progress float32) float32 {
	return -revealWidth + progress*openSpan(n)
}

// aheadOfLight is how far rune i still sits in front of the wavefront, measured
// from the centre outward. Negative once the light has passed it.
func aheadOfLight(i, n int, light float32) float32 {
	centre := float32(n-1) / 2
	return abs32(float32(i)-centre) - light
}

// writeFill draws a cell the text has not claimed — the caller's Fill rune when
// it set one, and a plain space otherwise.
func (r Reveal) writeFill(b *strings.Builder) {
	if r.Fill == 0 {
		b.WriteByte(' ')
		return
	}
	writeCell(b, r.FillColor, r.Fill)
}

// unclaimed is a whole row the text has no presence in — the frame an entrance
// starts from and a collapse ends on.
func (r Reveal) unclaimed(n int) string {
	var b strings.Builder
	for range n {
		r.writeFill(&b)
	}
	b.WriteString(reset)
	return b.String()
}

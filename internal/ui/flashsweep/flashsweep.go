package flashsweep

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/haasted/sonos-sandbox/internal/ui/movy"
)

const (
	revealWidth float32 = 2.5
	sigma       float32 = 1.0
	flashWeight float32 = 0.6
	dimFloor    float32 = 0.15
	visibleAt   float32 = 0.06
	glintSigma  float32 = 2.2
	glintTail   float32 = 2.5 * glintSigma
	glintWeight float32 = 1.0
)

func clamp(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func smoothstep(edge0, edge1, x float32) float32 {
	t := clamp((x-edge0)/(edge1-edge0), 0, 1)
	return t * t * (3 - 2*t)
}

func LightPos(elapsed time.Duration, speed float32) float32 {
	return float32(elapsed.Seconds()) * speed
}

func Duration(text string, speed float32) time.Duration {
	if speed <= 0 {
		return 0
	}
	span := float32(len([]rune(text))) + revealWidth
	return time.Duration(float64(span/speed) * float64(time.Second))
}

func FromColor(c color.Color) movy.V3 {
	r, g, b, _ := c.RGBA()
	return movy.FromRgb(movy.Rgb{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8)})
}

type Ramp struct {
	Base  movy.V3
	Flash movy.V3
}

func (r Ramp) at(burst float32) movy.Rgb {
	return r.blend(burst).ToRgb()
}

func (r Ramp) blend(burst float32) movy.V3 {
	return r.Base.Lerp(r.Flash, burst)
}

func Glint(text string, progress float32, fg Ramp) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}

	span := float32(len(runes)-1) + 2*glintTail
	lightPos := -glintTail + clamp(progress, 0, 1)*span

	var b strings.Builder
	for i, ch := range runes {
		d := float32(i) - lightPos
		writeCell(&b, fg.blend(glintWeight*gauss(d, glintSigma)), ch)
	}
	b.WriteString(reset)
	return b.String()
}

func Line(text string, lightPos float32, base, flash movy.V3) string {
	var b strings.Builder
	for i, ch := range []rune(text) {
		d := float32(i) - lightPos

		reveal := 1 - smoothstep(-revealWidth, revealWidth, d)
		if reveal < visibleAt {
			b.WriteByte(' ')
			continue
		}

		col := base.Scale(dimFloor).Lerp(base, reveal).Lerp(flash, gauss(d, sigma)*flashWeight)
		writeCell(&b, col, ch)
	}
	b.WriteString(reset)
	return b.String()
}

// reset closes a run of per-cell colour. One trailing reset is enough: every
// cell sets its own foreground, so none inherits the previous cell's.
const reset = "\x1b[0m"

// writeCell emits one rune in a truecolor foreground, the raw-ANSI approach
// this package uses throughout — see the README for why not lipgloss.
func writeCell(b *strings.Builder, c movy.V3, ch rune) {
	rgb := c.ToRgb()
	fmt.Fprintf(b, "\x1b[38;2;%d;%d;%dm%c", rgb.R, rgb.G, rgb.B, ch)
}

// gauss is the falloff every travelling light in this package uses: 1 at the
// wavefront, decaying either side over sigma cells.
func gauss(d, sigma float32) float32 {
	return float32(math.Exp(float64(-(d * d) / (2 * sigma * sigma))))
}

func abs32(v float32) float32 { return float32(math.Abs(float64(v))) }

func wrap01(v float32) float32 { return v - float32(math.Floor(float64(v))) }

func round(v float32) int { return int(math.Round(float64(v))) }

func easeOutCubic(t float32) float32 {
	u := 1 - t
	return 1 - u*u*u
}

// hsv converts a hue/saturation/value triple to linear colour, so a style can
// travel the colour wheel without a hard-coded ramp per step.
func hsv(h, s, v float32) movy.V3 {
	i := math.Floor(float64(h * 6))
	f := h*6 - float32(i)
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)

	switch int(i) % 6 {
	case 0:
		return movy.V3{R: v, G: t, B: p}
	case 1:
		return movy.V3{R: q, G: v, B: p}
	case 2:
		return movy.V3{R: p, G: v, B: t}
	case 3:
		return movy.V3{R: p, G: q, B: v}
	case 4:
		return movy.V3{R: t, G: p, B: v}
	default:
		return movy.V3{R: v, G: p, B: q}
	}
}

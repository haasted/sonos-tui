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
	return r.Base.Lerp(r.Flash, burst).ToRgb()
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
		burst := glintWeight * float32(math.Exp(float64(-(d*d)/(2*glintSigma*glintSigma))))

		c := fg.at(burst)
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm%c", c.R, c.G, c.B, ch)
	}
	b.WriteString("\x1b[0m")
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

		burst := float32(math.Exp(float64(-(d * d) / (2 * sigma * sigma))))
		col := base.Scale(dimFloor).Lerp(base, reveal).Lerp(flash, burst*flashWeight)

		rgb := col.ToRgb()
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm%c", rgb.R, rgb.G, rgb.B, ch)
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

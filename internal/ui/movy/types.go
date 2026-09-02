package movy

import "math"

type Rgb struct {
	R, G, B uint8
}

type V3 struct {
	R, G, B float32
}

func (a V3) Add(b V3) V3 { return V3{a.R + b.R, a.G + b.G, a.B + b.B} }

func (a V3) Scale(s float32) V3 { return V3{a.R * s, a.G * s, a.B * s} }

func (a V3) Mul(b V3) V3 { return V3{a.R * b.R, a.G * b.G, a.B * b.B} }

func (a V3) Lerp(b V3, t float32) V3 {
	return V3{
		R: a.R + (b.R-a.R)*t,
		G: a.G + (b.G-a.G)*t,
		B: a.B + (b.B-a.B)*t,
	}
}

func clamp01(f float32) float32 {
	return float32(math.Min(1, math.Max(0, float64(f))))
}

func (c V3) ToRgb() Rgb {
	return Rgb{
		R: uint8(clamp01(c.R) * 255.0),
		G: uint8(clamp01(c.G) * 255.0),
		B: uint8(clamp01(c.B) * 255.0),
	}
}

func FromRgb(c Rgb) V3 {
	return V3{
		R: float32(c.R) / 255.0,
		G: float32(c.G) / 255.0,
		B: float32(c.B) / 255.0,
	}
}

func V3Of(r, g, b float32) V3 { return V3{r, g, b} }

var (
	WhiteF = V3{1, 1, 1}
	BlackF = V3{0, 0, 0}
)

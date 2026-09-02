package movy

import "math"

type Frame struct {
	W, H int
	n    int

	Solid        []V3
	Glow         []V3
	tmp          []V3
	ScanlineMask []uint8

	vigX, vigY []float32

	Surface *RenderSurface

	Warmth   float32
	Flash    float32
	FlashCol V3
	Tint     V3

	GlowDecay   float32
	GlowBlur    bool
	ScanlineMul float32
	VignetteAmt float32
}

func NewFrame(w, h int) *Frame {
	n := w * h
	f := &Frame{
		W: w, H: h, n: n,
		Solid:        make([]V3, n),
		Glow:         make([]V3, n),
		tmp:          make([]V3, n),
		ScanlineMask: make([]uint8, n),
		vigX:         make([]float32, w),
		vigY:         make([]float32, h),
		Surface:      NewRenderSurface(w, h, Rgb{}),
		FlashCol:     WhiteF,
		Tint:         WhiteF,
		GlowDecay:    0.72,
		GlowBlur:     true,
		ScanlineMul:  0.87,
		VignetteAmt:  0.22,
	}
	f.rebuildVignette()
	return f
}

func (f *Frame) SetVignette(amt float32) {
	f.VignetteAmt = amt
	f.rebuildVignette()
}

func (f *Frame) rebuildVignette() {
	for x := 0; x < f.W; x++ {
		nx := (float32(x)/float32(f.W-1))*2.0 - 1.0
		f.vigX[x] = 1.0 - f.VignetteAmt*nx*nx*nx*nx
	}
	for y := 0; y < f.H; y++ {
		ny := (float32(y)/float32(f.H-1))*2.0 - 1.0
		f.vigY[y] = 1.0 - f.VignetteAmt*ny*ny*ny*ny
	}
}

func (f *Frame) idx(x, y int) int { return y*f.W + x }

func (f *Frame) inBounds(x, y int) bool {
	return x >= 0 && x < f.W && y >= 0 && y < f.H
}

func (f *Frame) BeginFrame() {
	for i := range f.ScanlineMask {
		f.ScanlineMask[i] = 0
	}

	if f.GlowBlur {

		for y := 0; y < f.H; y++ {
			row := y * f.W
			for x := 0; x < f.W; x++ {
				i := row + x
				l, r := BlackF, BlackF
				if x > 0 {
					l = f.Glow[i-1]
				}
				if x+1 < f.W {
					r = f.Glow[i+1]
				}
				g := f.Glow[i]
				f.tmp[i] = V3{
					R: l.R*0.25 + g.R*0.5 + r.R*0.25,
					G: l.G*0.25 + g.G*0.5 + r.G*0.25,
					B: l.B*0.25 + g.B*0.5 + r.B*0.25,
				}
			}
		}

		d := f.GlowDecay
		for y := 0; y < f.H; y++ {
			row := y * f.W
			for x := 0; x < f.W; x++ {
				i := row + x
				u, dn := BlackF, BlackF
				if y > 0 {
					u = f.tmp[i-f.W]
				}
				if y+1 < f.H {
					dn = f.tmp[i+f.W]
				}
				t := f.tmp[i]
				f.Glow[i] = V3{
					R: (u.R*0.25 + t.R*0.5 + dn.R*0.25) * d,
					G: (u.G*0.25 + t.G*0.5 + dn.G*0.25) * d,
					B: (u.B*0.25 + t.B*0.5 + dn.B*0.25) * d,
				}
			}
		}
	} else {
		d := f.GlowDecay
		for i := range f.Glow {
			f.Glow[i].R *= d
			f.Glow[i].G *= d
			f.Glow[i].B *= d
		}
	}
}

func (f *Frame) Composite() {
	for y := 0; y < f.H; y++ {
		row := y * f.W
		odd := y&1 == 1
		scan := float32(1.0)
		if odd {
			scan = f.ScanlineMul
		}
		vy := f.vigY[y] * scan
		vyFree := f.vigY[y]

		for x := 0; x < f.W; x++ {
			i := row + x
			s := f.Solid[i]
			g := f.Glow[i]
			v := f.vigX[x]
			if odd && f.ScanlineMask[i] != 0 {
				v *= vyFree
			} else {
				v *= vy
			}

			r := clamp01(s.R+g.R) * v
			gg := clamp01(s.G+g.G) * v
			b := clamp01(s.B+g.B) * v

			if f.Warmth > 0.001 {
				w := f.Warmth
				wr := r + (b-r)*w
				wb := b + (r-b)*w
				r, b = wr, wb
			}

			if f.Flash > 0.005 {
				r += (f.FlashCol.R - r) * f.Flash
				gg += (f.FlashCol.G - gg) * f.Flash
				b += (f.FlashCol.B - b) * f.Flash
			}

			r *= f.Tint.R
			gg *= f.Tint.G
			b *= f.Tint.B

			f.Surface.ColorMap[i] = Rgb{
				R: uint8(clamp01(r) * 255.0),
				G: uint8(clamp01(gg) * 255.0),
				B: uint8(clamp01(b) * 255.0),
			}
			f.Surface.ShadowMap[i] = 255
		}
	}
}

func (f *Frame) Px(x, y int, c V3) {
	if !f.inBounds(x, y) {
		return
	}
	f.Solid[f.idx(x, y)] = c
}

func (f *Frame) Rect(x, y, w, h int, c V3) {
	x0, y0 := max(x, 0), max(y, 0)
	x1, y1 := min(x+w, f.W), min(y+h, f.H)
	if x0 >= x1 || y0 >= y1 {
		return
	}
	for yy := y0; yy < y1; yy++ {
		for xx := x0; xx < x1; xx++ {
			f.Solid[f.idx(xx, yy)] = c
		}
	}
}

func (f *Frame) RectOutline(x, y, w, h int, c V3) {
	f.HLine(x, y, w, c)
	f.HLine(x, y+h-1, w, c)
	f.VLine(x, y, h, c)
	f.VLine(x+w-1, y, h, c)
}

func (f *Frame) HLine(x, y, w int, c V3) {
	if y < 0 || y >= f.H {
		return
	}
	x0, x1 := max(x, 0), min(x+w, f.W)
	for xx := x0; xx < x1; xx++ {
		f.Solid[f.idx(xx, y)] = c
	}
}

func (f *Frame) VLine(x, y, h int, c V3) {
	if x < 0 || x >= f.W {
		return
	}
	y0, y1 := max(y, 0), min(y+h, f.H)
	for yy := y0; yy < y1; yy++ {
		f.Solid[f.idx(x, yy)] = c
	}
}

func (f *Frame) SlPx(x, y int) {
	if !f.inBounds(x, y) {
		return
	}
	f.ScanlineMask[f.idx(x, y)] = 1
}

func (f *Frame) SlRect(x, y, w, h int) {
	x0, y0 := max(x, 0), max(y, 0)
	x1, y1 := min(x+w, f.W), min(y+h, f.H)
	if x0 >= x1 || y0 >= y1 {
		return
	}
	for yy := y0; yy < y1; yy++ {
		for xx := x0; xx < x1; xx++ {
			f.ScanlineMask[f.idx(xx, yy)] = 1
		}
	}
}

func (f *Frame) ShadeRect(x, y, w, h int, m float32) {
	x0, y0 := max(x, 0), max(y, 0)
	x1, y1 := min(x+w, f.W), min(y+h, f.H)
	if x0 >= x1 || y0 >= y1 {
		return
	}
	for yy := y0; yy < y1; yy++ {
		for xx := x0; xx < x1; xx++ {
			i := f.idx(xx, yy)
			f.Solid[i] = f.Solid[i].Scale(m)
		}
	}
}

func (f *Frame) GPx(x, y int, c V3) {
	if !f.inBounds(x, y) {
		return
	}
	i := f.idx(x, y)
	f.Glow[i] = f.Glow[i].Add(c)
}

func (f *Frame) GRect(x, y, w, h int, c V3) {
	x0, y0 := max(x, 0), max(y, 0)
	x1, y1 := min(x+w, f.W), min(y+h, f.H)
	if x0 >= x1 || y0 >= y1 {
		return
	}
	for yy := y0; yy < y1; yy++ {
		for xx := x0; xx < x1; xx++ {
			i := f.idx(xx, yy)
			f.Glow[i] = f.Glow[i].Add(c)
		}
	}
}

func (f *Frame) GHLine(x, y, w int, c V3) {
	if y < 0 || y >= f.H {
		return
	}
	x0, x1 := max(x, 0), min(x+w, f.W)
	for xx := x0; xx < x1; xx++ {
		i := f.idx(xx, y)
		f.Glow[i] = f.Glow[i].Add(c)
	}
}

func (f *Frame) GVLine(x, y, h int, c V3) {
	if x < 0 || x >= f.W {
		return
	}
	y0, y1 := max(y, 0), min(y+h, f.H)
	for yy := y0; yy < y1; yy++ {
		i := f.idx(x, yy)
		f.Glow[i] = f.Glow[i].Add(c)
	}
}

func (f *Frame) GRing(cx, cy, r float32, c V3) {
	if r <= 0 {
		return
	}
	const band float32 = 1.5
	x0 := int(math.Floor(float64(cx - r - band)))
	x1 := int(math.Ceil(float64(cx + r + band)))
	y0 := int(math.Floor(float64(cy - r - band)))
	y1 := int(math.Ceil(float64(cy + r + band)))

	yy := max(y0, 0)
	ymax := min(y1, f.H-1)
	for ; yy <= ymax; yy++ {
		xx := max(x0, 0)
		xmax := min(x1, f.W-1)
		for ; xx <= xmax; xx++ {
			dx := float32(xx) - cx
			dy := float32(yy) - cy
			d := float32(math.Sqrt(float64(dx*dx + dy*dy)))
			a := 1.0 - float32(math.Abs(float64(d-r)))/band
			if a > 0 {
				i := f.idx(xx, yy)
				f.Glow[i] = f.Glow[i].Add(c.Scale(a))
			}
		}
	}
}

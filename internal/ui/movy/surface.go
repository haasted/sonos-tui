package movy

type RenderSurface struct {
	ColorMap  []Rgb
	ShadowMap []uint8

	W, H int

	buf []byte
}

func NewRenderSurface(w, h int, color Rgb) *RenderSurface {
	s := &RenderSurface{
		ColorMap:  make([]Rgb, w*h),
		ShadowMap: make([]uint8, w*h),
		W:         w,
		H:         h,
	}
	s.ClearColored(color)
	return s
}

func (s *RenderSurface) ClearColored(c Rgb) {
	for i := range s.ColorMap {
		s.ColorMap[i] = c
	}
	for i := range s.ShadowMap {
		s.ShadowMap[i] = 255
	}
}

func (s *RenderSurface) ClearTransparent() {
	for i := range s.ColorMap {
		s.ColorMap[i] = Rgb{}
	}
	for i := range s.ShadowMap {
		s.ShadowMap[i] = 0
	}
}

func (s *RenderSurface) SetAlpha(alpha uint8) {
	if alpha == 0 {
		alpha = 1
	}
	for i, sh := range s.ShadowMap {
		if sh != 0 {
			s.ShadowMap[i] = alpha
		}
	}
}

func (s *RenderSurface) Resize(w, h int) {
	s.ColorMap = make([]Rgb, w*h)
	s.ShadowMap = make([]uint8, w*h)
	s.W, s.H = w, h
	s.ClearTransparent()
}

func (s *RenderSurface) Blit(src *RenderSurface, x, y int) {
	for sy := 0; sy < src.H; sy++ {
		dy := y + sy
		if dy < 0 || dy >= s.H {
			continue
		}
		for sx := 0; sx < src.W; sx++ {
			dx := x + sx
			if dx < 0 || dx >= s.W {
				continue
			}
			si := sx + sy*src.W
			if src.ShadowMap[si] == 0 {
				continue
			}
			di := dx + dy*s.W
			s.ColorMap[di] = src.ColorMap[si]
			s.ShadowMap[di] = src.ShadowMap[si]
		}
	}
}

func appendDecimal(buf []byte, v uint8) []byte {
	switch {
	case v >= 100:
		return append(buf, '0'+v/100, '0'+(v/10)%10, '0'+v%10)
	case v >= 10:
		return append(buf, '0'+v/10, '0'+v%10)
	default:
		return append(buf, '0'+v)
	}
}

func appendInt(buf []byte, v int) []byte {
	if v == 0 {
		return append(buf, '0')
	}
	start := len(buf)
	for v > 0 {
		buf = append(buf, byte('0'+v%10))
		v /= 10
	}

	for i, j := start, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return buf
}

func appendFg(buf []byte, c Rgb) []byte {
	buf = append(buf, "\x1b[38;2;"...)
	buf = appendDecimal(buf, c.R)
	buf = append(buf, ';')
	buf = appendDecimal(buf, c.G)
	buf = append(buf, ';')
	buf = appendDecimal(buf, c.B)
	return append(buf, 'm')
}

func appendBg(buf []byte, c Rgb) []byte {
	buf = append(buf, "\x1b[48;2;"...)
	buf = appendDecimal(buf, c.R)
	buf = append(buf, ';')
	buf = appendDecimal(buf, c.G)
	buf = append(buf, ';')
	buf = appendDecimal(buf, c.B)
	return append(buf, 'm')
}

const (
	lowerHalfBlock = "▄"
	upperHalfBlock = "▀"
)

func (s *RenderSurface) appendRow(buf []byte, y int) []byte {
	for x := 0; x < s.W; x++ {
		idx := x + y*s.W
		lowIdx := x + (y+1)*s.W

		upper := s.ColorMap[idx]
		lower := s.ColorMap[lowIdx]
		upperTrans := s.ShadowMap[idx] == 0
		lowerTrans := s.ShadowMap[lowIdx] == 0

		switch {
		case upperTrans && lowerTrans:
			buf = append(buf, "\x1b[m "...)
		case upperTrans:
			buf = append(buf, "\x1b[0;"...)
			buf = appendFg(buf, lower)
			buf = append(buf, lowerHalfBlock...)
		case lowerTrans:
			buf = append(buf, "\x1b[0;"...)
			buf = appendFg(buf, upper)
			buf = append(buf, upperHalfBlock...)
		default:
			buf = appendBg(buf, upper)
			buf = appendFg(buf, lower)
			buf = append(buf, lowerHalfBlock...)
		}
	}
	return buf
}

func (s *RenderSurface) maxEvenRow() int {
	maxY := s.H
	if s.H%2 == 1 {
		maxY = s.H - 1
	}
	return maxY
}

func (s *RenderSurface) ToAnsi() []byte {
	buf := s.buf[:0]
	maxY := s.maxEvenRow()
	for y := 0; y < maxY; y += 2 {
		buf = s.appendRow(buf, y)
		buf = append(buf, "\x1b["...)
		buf = appendInt(buf, s.W)
		buf = append(buf, 'D')
		buf = append(buf, "\x1b[1B"...)
	}
	s.buf = buf
	return buf
}

func (s *RenderSurface) Render() string {
	buf := s.buf[:0]
	maxY := s.maxEvenRow()
	for y := 0; y < maxY; y += 2 {
		buf = s.appendRow(buf, y)
		if y+2 < maxY {
			buf = append(buf, '\n')
		}
	}
	buf = append(buf, "\x1b[0m"...)
	s.buf = buf
	return string(buf)
}

package movy

import (
	"strconv"
	"strings"
	"testing"
)

func TestFrameRenderPipeline(t *testing.T) {
	f := NewFrame(20, 12)
	f.SetVignette(0.35)

	f.BeginFrame()
	for i := range f.Solid {
		f.Solid[i] = BlackF
	}
	f.Px(5, 5, WhiteF)
	f.GRing(10, 6, 2.0, V3Of(1, 0.2, 0.6))
	f.Composite()

	out := f.Surface.Render()

	if out == "" {
		t.Fatal("Render() returned empty string")
	}
	if !strings.Contains(out, "\x1b[") {
		t.Error("Render() output has no ANSI escapes — expected colored half-blocks")
	}
	if !strings.Contains(out, lowerHalfBlock) && !strings.Contains(out, upperHalfBlock) {
		t.Error("Render() output has no half-block glyphs")
	}
	if !strings.HasSuffix(out, "\x1b[0m") {
		t.Error("Render() output should end with a style reset, to avoid bleeding color into the surrounding UI")
	}

	lines := strings.Split(out, "\n")
	wantLines := f.H / 2
	if len(lines) != wantLines {
		t.Errorf("got %d text rows, want %d (H=%d)", len(lines), wantLines, f.H)
	}

	f.BeginFrame()
	for i := range f.Solid {
		f.Solid[i] = BlackF
	}
	f.Composite()
	out2 := f.Surface.Render()
	if out2 == out {
		t.Error("second frame identical to first — glow decay/blur had no effect")
	}
}

func TestRenderSurfaceOddHeight(t *testing.T) {
	s := NewRenderSurface(4, 5, Red500)
	out := s.Render()
	if out == "" {
		t.Fatal("Render() returned empty string for odd-height surface")
	}
}

func TestRenderSurfaceAllTransparent(t *testing.T) {
	s := NewRenderSurface(3, 2, White)
	s.ClearTransparent()
	out := s.Render()
	if !strings.Contains(out, "\x1b[m") {
		t.Error("fully transparent surface should use the bare reset escape")
	}
}

func TestRenderSurfaceAllOpaque(t *testing.T) {
	s := NewRenderSurface(2, 2, Blue500)
	out := s.Render()
	wantFg := "\x1b[38;2;" + strconv.Itoa(int(Blue500.R))
	wantBg := "\x1b[48;2;" + strconv.Itoa(int(Blue500.R))
	if !strings.Contains(out, wantFg) || !strings.Contains(out, wantBg) {
		t.Errorf("expected both fg and bg truecolor escapes for opaque pixels, got: %q", out)
	}
}

func TestFromHTMLRoundTrip(t *testing.T) {
	c, err := FromHTML("#892f8c")
	if err != nil {
		t.Fatal(err)
	}
	if c.R != 0x89 || c.G != 0x2f || c.B != 0x8c {
		t.Errorf("got %v", c)
	}
	if _, err := FromHTML("nope"); err == nil {
		t.Error("expected error for invalid color string")
	}
}

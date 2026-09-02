package movy

import (
	"errors"
	"testing"
)

func TestSpriteBasics(t *testing.T) {
	s := NewSprite(4, 4, "test")
	if s.Name != "test" || s.W != 4 || s.H != 4 {
		t.Fatalf("unexpected sprite: %+v", s)
	}
	if len(s.frameSet.Frames) != 1 {
		t.Fatalf("expected 1 initial frame, got %d", len(s.frameSet.Frames))
	}

	first := s.AddFrames(2, 4, 4)
	if first == nil || len(s.frameSet.Frames) != 3 {
		t.Fatalf("AddFrames: got %d frames, want 3", len(s.frameSet.Frames))
	}

	if err := s.SetFrameIndex(2); err != nil {
		t.Fatalf("SetFrameIndex(2): %v", err)
	}
	if err := s.SetFrameIndex(3); !errors.Is(err, ErrInvalidFrameIndex) {
		t.Fatalf("SetFrameIndex(3): got %v, want ErrInvalidFrameIndex", err)
	}

	if _, err := s.FrameSurface(1); err != nil {
		t.Fatalf("FrameSurface(1): %v", err)
	}
	if _, err := s.FrameSurface(9); !errors.Is(err, ErrInvalidFrameIndex) {
		t.Fatalf("FrameSurface(9): got %v, want ErrInvalidFrameIndex", err)
	}
}

func TestSpriteAnimation(t *testing.T) {
	s := NewSprite(2, 2, "anim")
	s.AddFrames(3, 2, 2)

	s.AddAnimation("cycle", NewFrameAnimation(0, 3, LoopForward, 0))

	if err := s.StartAnimation("missing"); !errors.Is(err, ErrAnimationNotFound) {
		t.Fatalf("got %v, want ErrAnimationNotFound", err)
	}

	if err := s.StartAnimation("cycle"); err != nil {
		t.Fatalf("StartAnimation: %v", err)
	}
	if s.frameSet.FrameIdx != 0 {
		t.Fatalf("frame idx after start: got %d, want 0", s.frameSet.FrameIdx)
	}

	s.StepActiveAnimation()
	if s.frameSet.FrameIdx != 0 {
		t.Fatalf("frame idx after first step: got %d, want 0 (just-started no-op)", s.frameSet.FrameIdx)
	}

	want := []int{1, 2, 3, 0}
	for i, w := range want {
		s.StepActiveAnimation()
		if s.frameSet.FrameIdx != w {
			t.Fatalf("step %d: frame idx got %d, want %d", i, s.frameSet.FrameIdx, w)
		}
	}

	if s.FinishedActiveAnimation() {
		t.Error("LoopForward should never report finished")
	}

	s.ResetActiveAnimation()
	if s.frameSet.FrameIdx != 1 {
		t.Fatalf("frame idx after reset: got %d, want 1 (see ResetActiveAnimation's doc comment)", s.frameSet.FrameIdx)
	}
}

func TestSpriteAnimationOnceFinishes(t *testing.T) {
	s := NewSprite(2, 2, "once")
	s.AddFrames(2, 2, 2)
	s.AddAnimation("play", NewFrameAnimation(0, 2, Once, 0))
	_ = s.StartAnimation("play")

	s.StepActiveAnimation()
	for i := 0; i < 2; i++ {
		s.StepActiveAnimation()
	}
	if !s.FinishedActiveAnimation() {
		t.Error("expected Once animation to be finished")
	}
	if s.frameSet.FrameIdx != 2 {
		t.Errorf("frame idx: got %d, want 2 (end)", s.frameSet.FrameIdx)
	}
}

func paintRegion(surf *RenderSurface, x0, y0, w, h int, c Rgb, alpha uint8) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			i := x + y*surf.W
			surf.ColorMap[i] = c
			surf.ShadowMap[i] = alpha
		}
	}
}

func TestSpriteSplitByHeight(t *testing.T) {
	s := NewSprite(16, 16, "split_h")
	surf := s.frameSet.Frames[0].DataSurface

	colors := []Rgb{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}, {255, 255, 0}}
	for region, c := range colors {
		paintRegion(surf, 0, region*4, 16, 4, c, uint8(200+region))
	}

	if err := s.SplitByHeight(4); err != nil {
		t.Fatalf("SplitByHeight: %v", err)
	}
	if len(s.frameSet.Frames) != 5 {
		t.Fatalf("got %d frames, want 5 (1 original + 4 split)", len(s.frameSet.Frames))
	}
	if s.H != 4 {
		t.Fatalf("sprite H: got %d, want 4", s.H)
	}

	for i := 1; i <= 4; i++ {
		frame := s.frameSet.Frames[i]
		want := colors[i-1]
		wantAlpha := uint8(200 + i - 1)
		for j, c := range frame.DataSurface.ColorMap {
			if c != want {
				t.Fatalf("frame %d pixel %d: got %v, want %v", i, j, c, want)
			}
		}
		for j, a := range frame.DataSurface.ShadowMap {
			if a != wantAlpha {
				t.Fatalf("frame %d pixel %d alpha: got %d, want %d", i, j, a, wantAlpha)
			}
		}
	}
}

func TestSpriteSplitByWH(t *testing.T) {
	s := NewSprite(32, 16, "split_wh")
	surf := s.frameSet.Frames[0].DataSurface

	colors := []Rgb{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}, {255, 255, 0}}
	for row := 0; row < 2; row++ {
		for col := 0; col < 2; col++ {
			paintRegion(surf, col*16, row*8, 16, 8, colors[row*2+col], uint8(100+row*2+col))
		}
	}

	if err := s.SplitByWH(16, 8); err != nil {
		t.Fatalf("SplitByWH: %v", err)
	}
	if len(s.frameSet.Frames) != 5 {
		t.Fatalf("got %d frames, want 5", len(s.frameSet.Frames))
	}
	if s.W != 16 || s.H != 8 {
		t.Fatalf("sprite dims: got %dx%d, want 16x8", s.W, s.H)
	}

	for i := 1; i <= 4; i++ {
		frame := s.frameSet.Frames[i]
		want := colors[i-1]
		wantAlpha := uint8(100 + i - 1)
		if frame.DataSurface.ColorMap[0] != want {
			t.Errorf("frame %d top-left: got %v, want %v", i, frame.DataSurface.ColorMap[0], want)
		}
		if frame.DataSurface.ShadowMap[0] != wantAlpha {
			t.Errorf("frame %d top-left alpha: got %d, want %d", i, frame.DataSurface.ShadowMap[0], wantAlpha)
		}
	}
}

func TestSpriteSplitByWHOffset(t *testing.T) {

	s := NewSprite(36, 16, "split_offset")
	surf := s.frameSet.Frames[0].DataSurface
	paintRegion(surf, 0, 0, 36, 16, Rgb{0, 0, 0}, 50)

	colors := []Rgb{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}, {255, 255, 0}}
	for row := 0; row < 2; row++ {
		for col := 0; col < 2; col++ {
			paintRegion(surf, 4+col*16, 4+row*6, 16, 6, colors[row*2+col], uint8(200+row*2+col))
		}
	}

	if err := s.SplitByWHOffset(16, 6, 4, 4); err != nil {
		t.Fatalf("SplitByWHOffset: %v", err)
	}
	if len(s.frameSet.Frames) != 5 {
		t.Fatalf("got %d frames, want 5", len(s.frameSet.Frames))
	}
	for i := 1; i <= 4; i++ {
		frame := s.frameSet.Frames[i]
		want := colors[i-1]
		if frame.DataSurface.ColorMap[0] != want {
			t.Errorf("frame %d first pixel: got %v, want %v (border leaked in?)", i, frame.DataSurface.ColorMap[0], want)
		}
		last := 16*6 - 1
		if frame.DataSurface.ColorMap[last] != want {
			t.Errorf("frame %d last pixel: got %v, want %v", i, frame.DataSurface.ColorMap[last], want)
		}
	}
}

func TestSpriteSplitByWHOffsetValidatesDimensions(t *testing.T) {
	s := NewSprite(32, 16, "split_validate")
	if err := s.SplitByWHOffset(16, 8, 20, 10); !errors.Is(err, ErrInvalidDimensions) {
		t.Errorf("offset-too-large: got %v, want ErrInvalidDimensions", err)
	}
	if err := s.SplitByWHOffset(13, 7, 2, 0); !errors.Is(err, ErrInvalidDimensions) {
		t.Errorf("not-evenly-divisible: got %v, want ErrInvalidDimensions", err)
	}
}

func TestSpritePool(t *testing.T) {
	pool := NewSpritePool()
	for i := 0; i < 3; i++ {
		pool.Add(NewSprite(2, 2, "poolable"))
	}
	if pool.TotalCount() != 3 || pool.CountFree() != 3 {
		t.Fatalf("initial: total=%d free=%d, want 3/3", pool.TotalCount(), pool.CountFree())
	}

	a := pool.Get()
	b := pool.Get()
	if a == nil || b == nil || a == b {
		t.Fatal("expected two distinct sprites")
	}
	if pool.CountFree() != 1 {
		t.Fatalf("free after 2 gets: got %d, want 1", pool.CountFree())
	}

	c := pool.Get()
	if c == nil {
		t.Fatal("expected a third free sprite")
	}
	if pool.Get() != nil {
		t.Error("expected nil once the pool is exhausted")
	}

	pool.Release(b)
	if pool.CountFree() != 1 {
		t.Fatalf("free after release: got %d, want 1", pool.CountFree())
	}
	if got := pool.Get(); got != b {
		t.Error("expected Get to hand back the just-released sprite")
	}

	if len(pool.All()) != 3 {
		t.Errorf("All(): got %d, want 3", len(pool.All()))
	}
}

func TestRenderSurfaceBlit(t *testing.T) {
	dst := NewRenderSurface(6, 4, Black)
	src := NewRenderSurface(2, 2, Red500)

	src.ShadowMap[0] = 0

	dst.Blit(src, 1, 1)

	if dst.ColorMap[1+1*6] != Black {
		t.Error("transparent src pixel should not have overwritten dst")
	}
	if dst.ColorMap[2+1*6] != Red500 {
		t.Error("opaque src pixel should have been blitted")
	}
	if dst.ColorMap[1+2*6] != Red500 {
		t.Error("opaque src pixel (row 2) should have been blitted")
	}

	dst.Blit(src, 5, 3)
}

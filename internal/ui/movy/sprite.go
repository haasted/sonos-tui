package movy

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrAnimationNotFound = errors.New("movy: animation not found")
	ErrEmptyFrameSet     = errors.New("movy: sprite has no frames")
	ErrInvalidFrameIndex = errors.New("movy: invalid frame index")
	ErrInvalidDimensions = errors.New("movy: invalid split dimensions")
)

type SpriteFrame struct {
	W, H        int
	DataSurface *RenderSurface
}

func NewSpriteFrame(w, h int) *SpriteFrame {
	return &SpriteFrame{W: w, H: h, DataSurface: NewRenderSurface(w, h, Rgb{})}
}

type SpriteFrameSet struct {
	Frames   []*SpriteFrame
	FrameIdx int
}

func newSpriteFrameSet(count, w, h int) *SpriteFrameSet {
	fs := &SpriteFrameSet{Frames: make([]*SpriteFrame, 0, count)}
	for i := 0; i < count; i++ {
		fs.Frames = append(fs.Frames, NewSpriteFrame(w, h))
	}
	return fs
}

func (fs *SpriteFrameSet) AddFrame(w, h int) *SpriteFrame {
	f := NewSpriteFrame(w, h)
	fs.Frames = append(fs.Frames, f)
	return f
}

type FrameAnimation struct {
	Animator IndexAnimator
	Speed    int

	justStarted bool
	speedCtr    int
}

func NewFrameAnimation(start, end int, mode LoopMode, speed int) FrameAnimation {
	return FrameAnimation{
		Animator:    NewIndexAnimator(start, end, mode),
		Speed:       speed,
		justStarted: true,
		speedCtr:    speed,
	}
}

func (a *FrameAnimation) Finished() bool { return a.Animator.OnceFinished }

func (a *FrameAnimation) step(sprite *Sprite) {
	if a.speedCtr > 0 {
		a.speedCtr--
		return
	}
	a.speedCtr = a.Speed
	if a.justStarted {
		a.justStarted = false
		return
	}
	sprite.frameSet.FrameIdx = a.Animator.Step()
}

type Sprite struct {
	Name    string
	W, H    int
	X, Y, Z int32

	frameSet        *SpriteFrameSet
	animations      map[string]*FrameAnimation
	activeAnimation string
}

func NewSprite(w, h int, name string) *Sprite {
	return &Sprite{
		Name:       name,
		W:          w,
		H:          h,
		frameSet:   newSpriteFrameSet(1, w, h),
		animations: make(map[string]*FrameAnimation),
	}
}

func (s *Sprite) SetFrameIndex(idx int) error {
	if idx < 0 || idx >= len(s.frameSet.Frames) {
		return ErrInvalidFrameIndex
	}
	s.frameSet.FrameIdx = idx
	return nil
}

func (s *Sprite) CurrentFrameSurface() (*RenderSurface, error) {
	if len(s.frameSet.Frames) == 0 {
		return nil, ErrEmptyFrameSet
	}
	return s.frameSet.Frames[s.frameSet.FrameIdx].DataSurface, nil
}

func (s *Sprite) FrameSurface(index int) (*RenderSurface, error) {
	if len(s.frameSet.Frames) == 0 {
		return nil, ErrEmptyFrameSet
	}
	if index < 0 || index >= len(s.frameSet.Frames) {
		return nil, ErrInvalidFrameIndex
	}
	return s.frameSet.Frames[index].DataSurface, nil
}

func (s *Sprite) AddFrames(n, w, h int) *SpriteFrame {
	var first *SpriteFrame
	for i := 0; i < n; i++ {
		f := s.frameSet.AddFrame(w, h)
		if i == 0 {
			first = f
		}
	}
	return first
}

func (s *Sprite) AddAnimation(name string, anim FrameAnimation) {
	s.animations[name] = &anim
}

func (s *Sprite) StartAnimation(name string) error {
	anim, ok := s.animations[name]
	if !ok {
		return fmt.Errorf("%w: %q", ErrAnimationNotFound, name)
	}
	s.activeAnimation = name
	anim.Animator.Current = anim.Animator.Start
	anim.Animator.OnceFinished = false
	anim.justStarted = true
	anim.speedCtr = anim.Speed
	s.frameSet.FrameIdx = anim.Animator.Start
	return nil
}

func (s *Sprite) StepActiveAnimation() {
	if s.activeAnimation == "" {
		return
	}
	if anim, ok := s.animations[s.activeAnimation]; ok {
		anim.step(s)
	}
}

func (s *Sprite) ResetActiveAnimation() {
	if s.activeAnimation == "" {
		return
	}
	if anim, ok := s.animations[s.activeAnimation]; ok {
		anim.Animator.Current = anim.Animator.Start
		anim.step(s)
	}
}

func (s *Sprite) FinishedActiveAnimation() bool {
	if s.activeAnimation == "" {
		return false
	}
	if anim, ok := s.animations[s.activeAnimation]; ok {
		return anim.Finished()
	}
	return false
}

func (s *Sprite) SetAlphaCurrentFrame(alpha uint8) error {
	surf, err := s.CurrentFrameSurface()
	if err != nil {
		return err
	}
	surf.SetAlpha(alpha)
	return nil
}

func (s *Sprite) SetXY(x, y int32) { s.X, s.Y = x, y }

func (s *Sprite) Render() (string, error) {
	surf, err := s.CurrentFrameSurface()
	if err != nil {
		return "", err
	}
	return surf.Render(), nil
}

func (s *Sprite) splitGrid(splitWidth, splitHeight, leftOffset, topOffset int) error {
	src := s.frameSet.Frames[0].DataSurface
	w, h := src.W, src.H

	if w == 0 || h == 0 || splitWidth <= 0 || splitHeight <= 0 {
		return ErrInvalidDimensions
	}
	if leftOffset+splitWidth > w || topOffset+splitHeight > h {
		return ErrInvalidDimensions
	}
	usableW := w - leftOffset
	usableH := h - topOffset
	if usableW%splitWidth != 0 || usableH%splitHeight != 0 {
		return ErrInvalidDimensions
	}

	numCols := usableW / splitWidth
	numRows := usableH / splitHeight

	for row := 0; row < numRows; row++ {
		for col := 0; col < numCols; col++ {
			dst := s.frameSet.AddFrame(splitWidth, splitHeight).DataSurface
			for y := 0; y < splitHeight; y++ {
				srcStart := (topOffset+row*splitHeight+y)*w + (leftOffset + col*splitWidth)
				dstStart := y * splitWidth
				copy(dst.ColorMap[dstStart:dstStart+splitWidth], src.ColorMap[srcStart:srcStart+splitWidth])
				copy(dst.ShadowMap[dstStart:dstStart+splitWidth], src.ShadowMap[srcStart:srcStart+splitWidth])
			}
		}
	}

	s.frameSet.FrameIdx = 1
	s.W, s.H = splitWidth, splitHeight
	return nil
}

func (s *Sprite) SplitByWidth(splitWidth int) error {
	return s.splitGrid(splitWidth, s.frameSet.Frames[0].DataSurface.H, 0, 0)
}

func (s *Sprite) SplitByHeight(splitHeight int) error {
	return s.splitGrid(s.frameSet.Frames[0].DataSurface.W, splitHeight, 0, 0)
}

func (s *Sprite) SplitByWH(splitWidth, splitHeight int) error {
	return s.splitGrid(splitWidth, splitHeight, 0, 0)
}

func (s *Sprite) SplitByWHOffset(splitWidth, splitHeight, leftOffset, topOffset int) error {
	return s.splitGrid(splitWidth, splitHeight, leftOffset, topOffset)
}

func (s *Sprite) DebugPrint() error {
	surf, err := s.CurrentFrameSurface()
	if err != nil {
		return err
	}
	var b strings.Builder
	for y := 0; y < surf.H; y++ {
		for x := 0; x < surf.W; x++ {
			i := x + y*surf.W
			if surf.ShadowMap[i] == 0 {
				b.WriteString("\x1b[38;2;40;40;80m█")
			} else {
				c := surf.ColorMap[i]
				fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm█", c.R, c.G, c.B)
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString("\x1b[m")
	fmt.Print(b.String())
	return nil
}

package main

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-tui/internal/ui/movy"
)

const (
	spriteWalkW          = 40
	spriteWalkH          = 16
	spriteWalkFPS        = 30
	spriteWalkStepEvery  = 3
	spriteWalkFrameSpeed = 10
)

var spriteWalkShape = [4][4]bool{
	{false, true, true, false},
	{true, true, true, true},
	{true, true, true, true},
	{false, true, true, false},
}

type spriteWalkEffect struct {
	sprite  *movy.Sprite
	posAnim movy.IndexAnimator
	ticks   int
}

func newSpriteWalkEffect() spriteWalkEffect {
	sprite := movy.NewSprite(4, 4, "blob")
	paintSpriteWalkFrame(sprite, 0, movy.Rgb{R: 255, G: 200, B: 60})
	sprite.AddFrames(1, 4, 4)
	paintSpriteWalkFrame(sprite, 1, movy.Rgb{R: 255, G: 140, B: 40})

	sprite.AddAnimation("bob", movy.NewFrameAnimation(0, 1, movy.LoopBounce, spriteWalkFrameSpeed))
	if err := sprite.StartAnimation("bob"); err != nil {
		panic(err)
	}
	sprite.Y = (spriteWalkH - 4) / 2

	return spriteWalkEffect{
		sprite:  sprite,
		posAnim: movy.NewIndexAnimator(0, spriteWalkW-4, movy.LoopBounce),
	}
}

func paintSpriteWalkFrame(s *movy.Sprite, index int, c movy.Rgb) {
	surf, err := s.FrameSurface(index)
	if err != nil {
		return
	}
	for y, row := range spriteWalkShape {
		for x, on := range row {
			if !on {
				continue
			}
			i := x + y*4
			surf.ColorMap[i] = c
			surf.ShadowMap[i] = 255
		}
	}
}

type spriteWalkTickMsg time.Time

func spriteWalkTick() tea.Cmd {
	return tea.Tick(time.Second/spriteWalkFPS, func(t time.Time) tea.Msg { return spriteWalkTickMsg(t) })
}

func (m spriteWalkEffect) Init() tea.Cmd { return spriteWalkTick() }

func (m spriteWalkEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(spriteWalkTickMsg); !ok {
		return m, nil
	}
	m.sprite.StepActiveAnimation()
	m.ticks++
	if m.ticks >= spriteWalkStepEvery {
		m.ticks = 0
		m.sprite.X = int32(m.posAnim.Step())
	}
	return m, spriteWalkTick()
}

func (m spriteWalkEffect) View() tea.View {
	canvas := movy.NewRenderSurface(spriteWalkW, spriteWalkH, movy.Rgb{R: 8, G: 10, B: 20})

	if surf, err := m.sprite.CurrentFrameSurface(); err == nil {
		canvas.Blit(surf, int(m.sprite.X), int(m.sprite.Y))
	}

	return tea.NewView(movyStyle.Render(canvas.Render()))
}

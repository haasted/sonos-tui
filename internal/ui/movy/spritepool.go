package movy

type spriteEntry struct {
	sprite *Sprite
	inUse  bool
}

type SpritePool struct {
	entries []spriteEntry
}

func NewSpritePool() *SpritePool { return &SpritePool{} }

func (p *SpritePool) Add(s *Sprite) {
	p.entries = append(p.entries, spriteEntry{sprite: s})
}

func (p *SpritePool) Get() *Sprite {
	for i := range p.entries {
		if !p.entries[i].inUse {
			p.entries[i].inUse = true
			return p.entries[i].sprite
		}
	}
	return nil
}

func (p *SpritePool) Release(sprite *Sprite) {
	for i := range p.entries {
		if p.entries[i].sprite == sprite {
			p.entries[i].inUse = false
			return
		}
	}
}

func (p *SpritePool) CountFree() int {
	n := 0
	for _, e := range p.entries {
		if !e.inUse {
			n++
		}
	}
	return n
}

func (p *SpritePool) TotalCount() int { return len(p.entries) }

func (p *SpritePool) All() []*Sprite {
	all := make([]*Sprite, len(p.entries))
	for i, e := range p.entries {
		all[i] = e.sprite
	}
	return all
}

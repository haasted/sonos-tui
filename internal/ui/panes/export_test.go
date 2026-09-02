package panes

import "time"

const SweepDuration = tabSweepDuration

type SweepTickMsg = tabSweepTickMsg

func NewSweepTick(gen int) SweepTickMsg { return tabSweepTickMsg{gen: gen} }

func (t Tabs) SweepGen() int { return t.sweepGen }

func (t Tabs) SweepElapsed() time.Duration {
	if t.sweepStart.IsZero() {
		return 0
	}
	return time.Since(t.sweepStart)
}

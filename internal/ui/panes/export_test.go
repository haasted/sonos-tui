package panes

import (
	"image/color"
	"time"
)

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

const DividerSweepDuration = dividerSweepDuration

// ForceDividerSweep starts a sweep immediately, bypassing the idle timer.
func ForceDividerSweep(t Tabs, reverse bool) Tabs {
	t.beginDividerSweep()
	t.dividerReverse = reverse
	return t
}

// DividerColor reports the colour rolled for the current sweep.
func DividerColor(t Tabs) color.Color { return t.dividerColor }

// SetDividerElapsed rewinds the sweep's start so it reads as `elapsed` old.
func SetDividerElapsed(t Tabs, elapsed time.Duration) Tabs {
	t.dividerStart = time.Now().Add(-elapsed)
	return t
}

func DividerGen(t Tabs) int { return t.dividerGen }

func NewDividerSchedule(gen int) dividerScheduleMsg { return dividerScheduleMsg{gen: gen} }

func DividerReverse(t Tabs) bool { return t.dividerReverse }

// DividerActive reports the current sweep's progress, if one is running.
func DividerActive(t Tabs) (float32, bool) { return t.dividerProgress() }

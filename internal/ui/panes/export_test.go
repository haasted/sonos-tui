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

// NoticeHoldFor is how long a notice built from this message stays fully lit,
// so a test can compare two kinds without waiting for either.
func NoticeHoldFor(msg NoticeMsg) time.Duration {
	return notice{confirmation: msg.Confirmation}.hold()
}

const ModalSweepDuration = modalSweepDuration

// SetModalSweptAt rewinds the opening sweep so it reads as `elapsed` old.
func SetModalSweptAt(m Modal, elapsed time.Duration) Modal {
	m.sweptAt = time.Now().Add(-elapsed)
	return m
}

// SetModalSweep fixes the corner and direction a sweep rolled at random.
func SetModalSweep(m Modal, corner int, clockwise bool) Modal {
	m.sweep = sweepCorner{at: corner, clockwise: clockwise}
	return m
}

// ModalSweepCorner reports the corner and direction a sweep rolled.
func ModalSweepCorner(m Modal) (int, bool) { return m.sweep.at, m.sweep.clockwise }

// ModalBox renders just the box, without compositing it over anything.
func ModalBox(m Modal) string { return m.box() }

const (
	NoticeFade   = noticeFade
	NoticeReturn = noticeReturn
)

// SetNoticeElapsed rewinds the current notice so it reads as `elapsed` old.
func SetNoticeElapsed(t Tabs, elapsed time.Duration) Tabs {
	t.notice.shownAt = time.Now().Add(-elapsed)
	return t
}

// NoticeOpenFor is how long a notice built from this message takes to open.
func NoticeOpenFor(msg NoticeMsg) time.Duration {
	return notice{text: msg.Text, entrance: msg.Entrance}.openDuration()
}

// SetModalNoticeElapsed rewinds the modal's notice so it reads as `elapsed` old.
func SetModalNoticeElapsed(m Modal, elapsed time.Duration) Modal {
	m.notice.shownAt = time.Now().Add(-elapsed)
	return m
}

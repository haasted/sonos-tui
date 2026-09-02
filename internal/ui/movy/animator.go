package movy

type LoopMode int

const (
	Once LoopMode = iota

	LoopForward

	LoopBackwards

	LoopBounce
)

type IndexAnimator struct {
	Start   int
	End     int
	Mode    LoopMode
	Current int

	Direction    int8
	OnceFinished bool
}

func NewIndexAnimator(start, end int, mode LoopMode) IndexAnimator {
	return IndexAnimator{
		Start:     start,
		End:       end,
		Mode:      mode,
		Current:   start,
		Direction: 1,
	}
}

func (a *IndexAnimator) Step() int {
	forward := a.End >= a.Start

	switch a.Mode {
	case Once:
		if a.Current == a.End {
			a.OnceFinished = true
			return a.Current
		}
		if forward {
			a.Current = min(a.Current+1, a.End)
		} else {
			a.Current = max(a.Current-1, a.End)
		}
		if a.Current == a.End {
			a.OnceFinished = true
		}

	case LoopForward:
		if forward {
			if a.Current >= a.End {
				a.Current = a.Start
			} else {
				a.Current++
			}
		} else {
			if a.Current <= a.End {
				a.Current = a.Start
			} else {
				a.Current--
			}
		}

	case LoopBackwards:
		if forward {
			if a.Current <= a.Start {
				a.Current = a.End
			} else {
				a.Current--
			}
		} else {
			if a.Current >= a.Start {
				a.Current = a.End
			} else {
				a.Current++
			}
		}

	case LoopBounce:

		if forward {
			if a.Direction > 0 {
				a.Current++
			} else {
				a.Current--
			}
			if a.Current >= a.End {
				a.Current = a.End
				a.Direction = -1
			} else if a.Current <= a.Start {
				a.Current = a.Start
				a.Direction = 1
			}
		} else {
			if a.Direction > 0 {
				a.Current--
			} else {
				a.Current++
			}
			if a.Current <= a.End {
				a.Current = a.End
				a.Direction = -1
			} else if a.Current >= a.Start {
				a.Current = a.Start
				a.Direction = 1
			}
		}
	}

	return a.Current
}

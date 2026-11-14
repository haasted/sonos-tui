package main

import "time"

// A swipe is told from the next by how fast its notches come. Measured on a
// real trackpad: one opens at 20-40 a tenth of a second and coasts down to one,
// and coasting never speeds back up, so a rate climbing again is a new swipe.
const (
	swipeWindow    = 100 * time.Millisecond // how far back the rate is counted
	swipeBurst     = 10                     // notches in the window that mean a swipe is under way
	swipeCoasting  = 3                      // notches in the window once it has died down
	swipeQuietTime = 500 * time.Millisecond // a pause this long ends a swipe whatever came before
)

// swipeTracker follows the notches of one swipe after another. A value, like
// the content holding it: the ring is an array, so a copy shares nothing.
type swipeTracker struct {
	recent   [2 * swipeBurst]time.Time // the latest notches, oldest overwritten
	next     int                       // where the next notch goes in recent
	lastAt   time.Time                 // when the last notch arrived
	dir      int                       // which way the swipe is going
	peaked   bool                      // the swipe has reached swipeBurst
	coasting bool                      // and has since died down to swipeCoasting
}

// notch records one wheel notch and reports whether it starts a new swipe: a
// change of direction, a quiet pause, or a burst after the last one coasted.
func (s swipeTracker) notch(dir int, at time.Time) (swipeTracker, bool) {
	s.recent[s.next%len(s.recent)] = at
	s.next++
	rate := s.rate(at)

	fresh := dir != s.dir || at.Sub(s.lastAt) > swipeQuietTime || (s.coasting && rate >= swipeBurst)
	if fresh {
		s.peaked, s.coasting = false, false
	}
	switch {
	case rate >= swipeBurst:
		s.peaked, s.coasting = true, false
	case s.peaked && rate <= swipeCoasting:
		s.coasting = true
	}

	s.dir, s.lastAt = dir, at
	return s, fresh
}

// rate is how many notches arrived within swipeWindow of at.
func (s swipeTracker) rate(at time.Time) int {
	count := 0
	for _, t := range s.recent {
		if !t.IsZero() && at.Sub(t) < swipeWindow {
			count++
		}
	}
	return count
}

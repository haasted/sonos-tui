package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Case: a real trackpad session, replayed. Seven separate swipes at the end
// of the list, two of them begun while the one before was still coasting, and
// a lone notch straggling in 265ms after a tail had died. Every swipe must be
// found, the two that overlap included, and the straggler must not count.
func TestSwipe_FindsEverySwipeInARealSession(t *testing.T) {
	var tracker swipeTracker
	var found []time.Duration
	for _, n := range recordedNotches(t) {
		var fresh bool
		if tracker, fresh = tracker.notch(n.dir, start.Add(n.at)); fresh {
			found = append(found, n.at)
		}
	}

	// Where each swipe begins in the recording, to the tenth of a second:
	// the scroll down to the end, then the seven at the end.
	want := []float64{0.0, 5.2, 6.7, 7.8, 10.9, 12.7, 13.8, 15.1}
	if len(found) != len(want) {
		t.Fatalf("found %d swipes, want %d: %v", len(found), len(want), found)
	}
	for i, at := range found {
		if diff := at.Seconds() - want[i]; diff < -0.15 || diff > 0.15 {
			t.Errorf("swipe %d begins at %.2fs, want about %.1fs", i, at.Seconds(), want[i])
		}
	}
}

// Case: a mouse wheel turned a notch at a time. It never bursts, so a new
// swipe is only ever a pause or a change of direction.
func TestSwipe_ASlowWheelIsOneSwipeUntilItPauses(t *testing.T) {
	var tracker swipeTracker
	var swipes int
	for _, ms := range []int{0, 200, 400, 600, 1200, 1400} { // one pause, between 600 and 1200
		var fresh bool
		if tracker, fresh = tracker.notch(1, start.Add(time.Duration(ms)*time.Millisecond)); fresh {
			swipes++
		}
	}
	if swipes != 2 {
		t.Fatalf("a slow wheel with one long pause made %d swipes, want 2", swipes)
	}
}

// Case: scrolling back the other way, mid swipe. That is a new swipe however
// close behind the last notch it comes.
func TestSwipe_ReversingStartsANewSwipe(t *testing.T) {
	tracker, _ := swipeTracker{}.notch(1, start)

	if _, fresh := tracker.notch(-1, start.Add(5*time.Millisecond)); !fresh {
		t.Fatal("reversing direction did not start a new swipe")
	}
}

var start = time.Date(2026, 10, 25, 9, 32, 26, 0, time.UTC)

type recordedNotch struct {
	at  time.Duration
	dir int
}

// recordedNotches reads the session in testdata/wheel_swipes.txt.
func recordedNotches(t *testing.T) []recordedNotch {
	t.Helper()

	f, err := os.Open("testdata/wheel_swipes.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var notches []recordedNotch
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		ms, _ := strconv.Atoi(fields[0])
		dir, _ := strconv.Atoi(fields[1])
		notches = append(notches, recordedNotch{at: time.Duration(ms) * time.Millisecond, dir: dir})
	}
	return notches
}

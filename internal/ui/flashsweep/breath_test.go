package flashsweep

import (
	"testing"
	"time"
)

// Case: the two ends of a cycle, which must meet. A curve that ends above where
// it began would jump on every wrap, once every five seconds, for ever.
func TestBreath_WrapsWithoutAJump(t *testing.T) {
	start, end := Breath(0), Breath(BreathPeriod)
	if start != 0 || end != 0 {
		t.Fatalf("a cycle runs from %.4f to %.4f, want 0 to 0", start, end)
	}

	justBefore := Breath(BreathPeriod - time.Millisecond)
	if justBefore > 0.01 {
		t.Fatalf("the cycle ends at %.4f, which is a visible step back to 0", justBefore)
	}
}

// Case: the shape. A breath climbs to full and falls back, and it does so once
// per cycle rather than twice.
func TestBreath_RisesOnceAndFallsBack(t *testing.T) {
	var peak float32
	rises, falls := 0, 0
	prev := Breath(0)
	for i := 1; i <= 500; i++ {
		v := Breath(sample(i, 500))
		switch {
		case v > prev:
			rises++
		case v < prev:
			falls++
		}
		peak = max(peak, v)
		prev = v
	}

	if peak < 0.99 {
		t.Fatalf("the breath reaches only %.3f; it should fill", peak)
	}
	if rises+falls != 500 {
		t.Fatalf("the curve is flat somewhere: %d rises and %d falls over 500 steps", rises, falls)
	}
	if rises == 0 || falls == 0 {
		t.Fatal("a breath must both rise and fall")
	}
}

// Case: the asymmetry, which is the whole reason this is not a sine. Breathing
// in is quicker than breathing out, so the peak sits before the middle.
func TestBreath_RisesQuickerThanItFalls(t *testing.T) {
	peakAt := 0
	var peak float32
	for i := 0; i <= 1000; i++ {
		if v := Breath(sample(i, 1000)); v > peak {
			peak, peakAt = v, i
		}
	}

	if peakAt >= 500 {
		t.Fatalf("the breath peaks at %.1f%% of the cycle; it must fill quicker than it empties",
			float64(peakAt)/10)
	}
	if peakAt < 300 {
		t.Fatalf("the breath peaks at %.1f%% of the cycle, which is a flash rather than a breath",
			float64(peakAt)/10)
	}

	// And the fall is wider, not merely later. At the same distance either side
	// of the peak the light is still brighter on the way down.
	peakTime := sample(peakAt, 1000)
	for _, step := range []time.Duration{200 * time.Millisecond, 500 * time.Millisecond, time.Second} {
		up, down := Breath(peakTime-step), Breath(peakTime+step)
		if down <= up {
			t.Errorf("%s either side of the peak reads %.3f rising and %.3f falling; the fall must be slower",
				step, up, down)
		}
	}
}

// Case: the pause at the bottom, which a sine does not have. The light should
// rest between breaths rather than turn straight around.
func TestBreath_RestsBetweenBreaths(t *testing.T) {
	const quiet = 0.05

	rested := 0
	for i := 0; i <= 500; i++ {
		if Breath(sample(i, 500)) < quiet {
			rested++
		}
	}

	if share := float64(rested) / 500; share < 0.15 {
		t.Fatalf("the light rests for %.0f%% of a cycle; a breath pauses longer than that", share*100)
	}
}

// Case: a caller that has been running longer than one cycle, which is every
// caller after five seconds.
func TestBreath_RepeatsEveryPeriod(t *testing.T) {
	for _, at := range []time.Duration{0, BreathPeriod / 4, BreathPeriod / 2, 3 * BreathPeriod / 4} {
		first, later := Breath(at), Breath(at+3*BreathPeriod)
		if diff := first - later; diff > 0.001 || diff < -0.001 {
			t.Errorf("at %s the breath reads %.4f, and %.4f three cycles later", at, first, later)
		}
	}
}

func sample(i, of int) time.Duration {
	return time.Duration(float64(i) / float64(of) * float64(BreathPeriod))
}

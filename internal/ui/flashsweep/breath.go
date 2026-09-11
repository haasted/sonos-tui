package flashsweep

import (
	"image/color"
	"time"

	"charm.land/lipgloss/v2"
)

const (
	// BreathPeriod is one full breath. Apple's sleep light runs at about twelve
	// breaths a minute, which is a person at rest rather than a person waiting.
	BreathPeriod = 5 * time.Second

	// breathPeak is where in the cycle the light is fullest. Before the middle,
	// because breathing in is quicker than breathing out.
	breathPeak = 0.42

	// breathRise is the width of the climb. Narrow, so the light spends a while
	// near dark at both ends of the cycle and the breaths read as separate.
	breathRise = 0.11
)

// breathFall is the width of the fall. Derived rather than chosen, so the curve
// reaches the same value at the end of a cycle as at the start and the wrap is
// invisible.
const breathFall = breathRise * (1 - breathPeak) / breathPeak

// Breath is how lit a breathing light is, from 0 at rest to 1 at the top of a
// breath. It is a pair of gaussians rather than a sine: a sine turns too
// sharply at both ends, where a breath pauses.
//
// Modelled on the sleep light of an older Mac, which follows US patent
// 6,658,577 at a slower rate than the patent states.
func Breath(elapsed time.Duration) float32 {
	t := wrap01(float32(elapsed) / float32(BreathPeriod))

	sigma := float32(breathRise)
	if t > breathPeak {
		sigma = breathFall
	}

	// Lifted clear of its own tail, so a cycle begins and ends at exactly 0
	// rather than at whatever the gaussian happens to leave behind.
	floor := gauss(breathPeak, breathRise)
	return clamp((gauss(t-breathPeak, sigma)-floor)/(1-floor), 0, 1)
}

// BreathAt mixes two colours by the breath at a point in its cycle, for a
// caller that wants a colour rather than a number.
func BreathAt(elapsed time.Duration, rest, lit color.Color) color.Color {
	ramp := Ramp{Base: FromColor(rest), Flash: FromColor(lit)}
	return lipgloss.Color(ramp.at(Breath(elapsed)).String())
}

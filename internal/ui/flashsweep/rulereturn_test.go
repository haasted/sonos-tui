package flashsweep

import (
	"strings"
	"testing"
	"time"

	"github.com/haasted/sonos-tui/internal/ui/movy"
)

// Case: every style at its two ends. At 0 the gap is empty; at 1 every cell
// is the rule's own glyph, so the divider is whole again whichever played.
func TestRuleReturn_EveryStyleStartsEmptyAndEndsWhole(t *testing.T) {
	for _, style := range ReturnStyles {
		r := testReturn(style)

		if got := plain(r.Draw(12, 0)); strings.TrimSpace(got) != "" {
			t.Errorf("%s at 0 drew %q, want an empty gap", style, got)
		}
		if got := plain(r.Draw(12, 1)); got != strings.Repeat("─", 12) {
			t.Errorf("%s at 1 drew %q, want the whole rule", style, got)
		}
	}
}

// Case: the growing styles halfway through. Close in has its ends back and
// its middle empty; open out the other way round.
func TestRuleReturn_CloseInAndOpenOutGrowFromOppositeEnds(t *testing.T) {
	closeIn := []rune(plain(testReturn(ReturnCloseIn).Draw(21, 0.45)))
	if closeIn[0] == ' ' || closeIn[10] != ' ' {
		t.Errorf("close in at half drew %q, want ends back and the middle empty", string(closeIn))
	}

	openOut := []rune(plain(testReturn(ReturnOpenOut).Draw(21, 0.45)))
	if openOut[10] == ' ' || openOut[0] != ' ' {
		t.Errorf("open out at half drew %q, want the middle back and the ends empty", string(openOut))
	}
}

// Case: the sweep. Its direction comes from the seed, and either way the rule
// is back on one side and not the other.
func TestRuleReturn_SweepGoesOneWayOrTheOther(t *testing.T) {
	right := testReturn(ReturnSweep)
	left := right
	left.Seed = 1

	r := []rune(plain(right.Draw(20, 0.4)))
	l := []rune(plain(left.Draw(20, 0.4)))
	if r[0] == ' ' || r[19] != ' ' {
		t.Errorf("an even seed drew %q, want the left end back first", string(r))
	}
	if l[19] == ' ' || l[0] != ' ' {
		t.Errorf("an odd seed drew %q, want the right end back first", string(l))
	}
}

// Case: the dissolve. The same seed always gives the same order, so a frame
// does not flicker from one paint to the next, and another seed differs.
func TestRuleReturn_DissolveKeepsItsOrderPerSeed(t *testing.T) {
	a := testReturn(ReturnDissolve)
	b := a
	b.Seed = 99

	if a.Draw(30, 0.5) != a.Draw(30, 0.5) {
		t.Error("one seed drew two different frames")
	}
	if plain(a.Draw(30, 0.5)) == plain(b.Draw(30, 0.5)) {
		t.Error("two seeds drew the same scattered frame")
	}
}

// Case: the stitch. Early on the line is dotted or dashed, not yet solid.
func TestRuleReturn_StitchFirmsUpThroughItsGlyphs(t *testing.T) {
	early := plain(testReturn(ReturnStitch).Draw(10, 0.2))

	if strings.ContainsRune(early, '─') {
		t.Errorf("stitch was solid early: %q", early)
	}
	if !strings.ContainsAny(early, "┈╌") {
		t.Errorf("stitch showed no dotted or dashed stage: %q", early)
	}
}

// Case: the duration. A longer gap takes longer to close, and a tiny one
// still takes long enough to be seen.
func TestReturnDuration_GrowsWithTheGap(t *testing.T) {
	if ReturnDuration(40) <= ReturnDuration(20) {
		t.Error("a longer gap did not take longer")
	}
	if ReturnDuration(1) < 100*time.Millisecond {
		t.Errorf("a one-cell gap closes in %v, too quick to see", ReturnDuration(1))
	}
}

func testReturn(style ReturnStyle) RuleReturn {
	return RuleReturn{
		Style: style, Rule: '─',
		Color:    movy.V3{R: 0.3, G: 0.3, B: 0.3},
		Backdrop: movy.V3{R: 0.1, G: 0.1, B: 0.1},
		Flash:    movy.V3{R: 1, G: 1, B: 1},
	}
}

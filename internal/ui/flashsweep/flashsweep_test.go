package flashsweep

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/haasted/sonos-tui/internal/ui/movy"
)

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plain(s string) string { return ansi.ReplaceAllString(s, "") }

func revealed(s string) int {
	return len(strings.TrimRight(plain(s), " "))
}

var (
	base  = movy.V3Of(0.2, 0.6, 1)
	flash = movy.WhiteF

	fgRamp = Ramp{Base: movy.V3Of(0.98, 0.75, 0.14), Flash: movy.V3Of(1, 1, 1)}
)

func TestHiddenBeforeLightArrives(t *testing.T) {
	got := Line("now playing", -1000, base, flash)
	if strings.TrimSpace(plain(got)) != "" {
		t.Fatalf("expected nothing visible, got %q", plain(got))
	}
}

func TestFullyRevealedAfterDuration(t *testing.T) {
	const text = "now playing"
	const speed float32 = 20

	got := Line(text, LightPos(Duration(text, speed), speed), base, flash)
	if plain(got) != text {
		t.Fatalf("got %q, want %q", plain(got), text)
	}
}

func TestRevealOnlyEverGrows(t *testing.T) {
	const text = "Kind of Blue"

	prev := 0
	for pos := float32(-5); pos <= 20; pos += 0.25 {
		n := revealed(Line(text, pos, base, flash))
		if n < prev {
			t.Fatalf("at lightPos %v reveal went backwards: %d then %d", pos, prev, n)
		}
		prev = n
	}
	if prev != len(text) {
		t.Fatalf("never fully revealed: got %d of %d", prev, len(text))
	}
}

func TestPreservesRunesNotBytes(t *testing.T) {
	const text = "Sigur Rós — Ágætis"

	got := plain(Line(text, 1000, base, flash))
	if got != text {
		t.Fatalf("got %q, want %q", got, text)
	}
}

func TestFlashIsBrighterThanSettledColor(t *testing.T) {
	const text = "aaaaaaaaaa"

	atCrossing := Line(text, 5, base, flash)
	settled := Line(text, 1000, base, flash)

	if brightestOf(atCrossing) <= brightestOf(settled) {
		t.Fatal("expected the crossing point to flash brighter than the settled colour")
	}
}

func brightestOf(s string) int {
	best := 0
	for _, m := range regexp.MustCompile(`38;2;(\d+);(\d+);(\d+)`).FindAllStringSubmatch(s, -1) {
		sum := 0
		for _, c := range m[1:] {
			n := 0
			for _, d := range c {
				n = n*10 + int(d-'0')
			}
			sum += n
		}
		if sum > best {
			best = sum
		}
	}
	return best
}

func TestDurationScalesWithSpeed(t *testing.T) {
	const text = "now playing"

	fast, slow := Duration(text, 40), Duration(text, 10)
	if fast >= slow {
		t.Fatalf("faster speed should finish sooner: %v vs %v", fast, slow)
	}
	if Duration(text, 0) != time.Duration(0) {
		t.Fatal("zero speed should yield zero duration rather than dividing by zero")
	}
}

func TestGlintNeverHidesCharacters(t *testing.T) {
	const text = "Living Room"

	for p := float32(0); p <= 1; p += 0.05 {
		if got := plain(Glint(text, p, fgRamp)); got != text {
			t.Fatalf("at progress %.2f got %q, want the full label %q", p, got, text)
		}
	}
}

func TestGlintPeakTravelsLeftToRight(t *testing.T) {
	const text = "Living Room"

	prev := -1
	for p := float32(0.15); p <= 0.85; p += 0.05 {
		at, _ := peakDeviation(Glint(text, p, fgRamp), fgRamp.Base.ToRgb())
		if at < prev {
			t.Fatalf("at progress %.2f the glint moved backwards: index %d then %d", p, prev, at)
		}
		prev = at
	}
	if prev <= 0 {
		t.Fatalf("glint never travelled: final peak index %d", prev)
	}
}

func TestGlintClampsProgressOutsideRange(t *testing.T) {
	const text = "Bedroom"

	if got := plain(Glint(text, -5, fgRamp)); got != text {
		t.Fatalf("negative progress got %q, want %q", got, text)
	}
	if got := plain(Glint(text, 42, fgRamp)); got != text {
		t.Fatalf("overshooting progress got %q, want %q", got, text)
	}
}

func TestGlintIsQuiescentAtBothEndsAndBrightInTheMiddle(t *testing.T) {
	const text = "Bedroom"

	settled := fgRamp.Base.ToRgb()

	for _, p := range []float32{0, 1} {
		if _, dev := peakDeviation(Glint(text, p, fgRamp), settled); dev > 20 {
			t.Fatalf("at progress %v the glint should have left the label, deviation %d", p, dev)
		}
	}

	if _, dev := peakDeviation(Glint(text, 0.5, fgRamp), settled); dev < 150 {
		t.Fatalf("mid-sweep glint too faint to see: deviation only %d", dev)
	}
}

func TestGlintLeavesAFlatRampUntouched(t *testing.T) {
	const text = "Bedroom"

	white := movy.V3Of(1, 1, 1)
	flat := Ramp{Base: white, Flash: white}

	for p := float32(0); p <= 1; p += 0.1 {
		if _, dev := peakDeviation2(Glint(text, p, flat), white.ToRgb(), `38;2;(\d+);(\d+);(\d+)`); dev != 0 {
			t.Fatalf("at progress %.1f a flat ramp still shifted by %d; it should stay constant so tab labels remain readable", p, dev)
		}
	}
}

func peakDeviation(s string, base movy.Rgb) (int, int) {
	return peakDeviation2(s, base, `38;2;(\d+);(\d+);(\d+)`)
}

func peakDeviation2(s string, base movy.Rgb, pattern string) (int, int) {
	bestIdx, best := -1, 0
	for i, m := range regexp.MustCompile(pattern).FindAllStringSubmatch(s, -1) {
		got := [3]int{}
		for j, c := range m[1:] {
			n := 0
			for _, d := range c {
				n = n*10 + int(d-'0')
			}
			got[j] = n
		}
		dev := abs(got[0]-int(base.R)) + abs(got[1]-int(base.G)) + abs(got[2]-int(base.B))
		if dev > best {
			best, bestIdx = dev, i
		}
	}
	return bestIdx, best
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func brightestIndex(s string) int {
	best, bestIdx := -1, -1
	for i, m := range regexp.MustCompile(`38;2;(\d+);(\d+);(\d+)`).FindAllStringSubmatch(s, -1) {
		sum := 0
		for _, c := range m[1:] {
			n := 0
			for _, d := range c {
				n = n*10 + int(d-'0')
			}
			sum += n
		}
		if sum > best {
			best, bestIdx = sum, i
		}
	}
	return bestIdx
}

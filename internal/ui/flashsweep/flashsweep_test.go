package flashsweep

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/haasted/sonos-sandbox/internal/ui/movy"
)

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plain(s string) string { return ansi.ReplaceAllString(s, "") }

func revealed(s string) int {
	return len(strings.TrimRight(plain(s), " "))
}

var (
	base  = movy.V3Of(0.2, 0.6, 1)
	flash = movy.WhiteF
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

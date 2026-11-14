package flashsweep

import (
	"strings"
	"testing"
	"time"

	"github.com/haasted/sonos-tui/internal/ui/movy"
)

// revealText is a short label with a leading glyph and padding, the shape a
// notice composes — the reveal has to hold up on that, not on prose.
const revealText = " ℹ press ? for help "

// Case: the entrance boundaries — nothing at all before it starts, the whole
// text once it ends. The empty frame is what makes the collapse land cleanly:
// a style that stranded a rune would leave it sitting in a closed divider.
func TestReveal_StartsHiddenAndEndsComplete(t *testing.T) {
	for _, style := range RevealStyles {
		t.Run(style.String(), func(t *testing.T) {
			r := testReveal(style)

			if cols := litColumns(r.Open(revealText, 0)); len(cols) != 0 {
				t.Fatalf("lit columns %v at progress 0; the entrance must start from nothing", cols)
			}
			if got, want := plain(r.Open(revealText, 1)), revealText; got != want {
				t.Fatalf("settled on %q, want %q", got, want)
			}
		})
	}
}

// Case: the defining property — early on, only the middle of the row is lit,
// which is what makes the entrance read as centre-out rather than as Line.
func TestReveal_OpensFromTheCentre(t *testing.T) {
	n := len([]rune(revealText))
	lo, hi := n/4, n-n/4

	for _, style := range RevealStyles {
		t.Run(style.String(), func(t *testing.T) {
			cols := litColumns(testReveal(style).Open(revealText, centreSample))
			if len(cols) == 0 {
				t.Fatalf("nothing lit at progress %.2f; the entrance is too slow to read", centreSample)
			}
			for _, c := range cols {
				if c < lo || c >= hi {
					t.Fatalf("column %d is lit early, outside the middle band [%d,%d) — this is opening from the ends",
						c, lo, hi)
				}
			}
		})
	}
}

// Case: a wavefront that overshoots would blank runes it had already brought
// up, so the lit count must only ever grow.
func TestReveal_NeverUnreveals(t *testing.T) {
	for _, style := range RevealStyles {
		t.Run(style.String(), func(t *testing.T) {
			prev := -1
			for step := range 21 {
				progress := float32(step) / 20
				lit := len(litColumns(testReveal(style).Open(revealText, progress)))
				if lit < prev {
					t.Fatalf("lit runes fell from %d to %d at progress %.2f", prev, lit, progress)
				}
				prev = lit
			}
		})
	}
}

// Case: progress outside 0-1, which a caller running slightly past the end of
// its own timer will pass without meaning to.
func TestReveal_ClampsProgressOutsideRange(t *testing.T) {
	for _, style := range RevealStyles {
		t.Run(style.String(), func(t *testing.T) {
			r := testReveal(style)
			if got, want := r.Open(revealText, 5), r.Open(revealText, 1); got != want {
				t.Fatal("progress past 1 should hold the fully open frame")
			}
			if got, want := r.Open(revealText, -5), r.Open(revealText, 0); got != want {
				t.Fatal("progress below 0 should hold the closed frame")
			}
		})
	}
}

// Case: the text is the caller's, so nothing may assume a fixed length — and
// a rune must survive as a rune, since room names carry æ, ø and å.
func TestReveal_PreservesArbitraryText(t *testing.T) {
	for _, text := range []string{"a", "ab", "Ovenpå", "ℹ", "Køkken — P6 Beat"} {
		for _, style := range RevealStyles {
			if got := plain(testReveal(style).Open(text, 1)); got != text {
				t.Fatalf("%s opened %q as %q", style, text, got)
			}
		}
	}
}

// Case: empty text, which a caller clearing a label will pass.
func TestReveal_EmptyTextIsSafe(t *testing.T) {
	for _, style := range RevealStyles {
		if got := testReveal(style).Open("", 0.5); got != "" {
			t.Fatalf("%s rendered %q for empty text", style, got)
		}
		if got := testReveal(style).Duration(""); got != 0 {
			t.Fatalf("%s gave empty text a duration of %s", style, got)
		}
	}
}

// Case: the two ways speed is configured — set explicitly, or left at zero to
// take the default. A zero that meant "instant" would divide by zero.
func TestReveal_DurationFollowsSpeed(t *testing.T) {
	fast := (Reveal{Speed: 40}).Duration(revealText)
	slow := (Reveal{Speed: 10}).Duration(revealText)
	if fast >= slow {
		t.Fatalf("a higher speed should open sooner: %s at 40, %s at 10", fast, slow)
	}

	if got, want := (Reveal{}).Duration(revealText), (Reveal{Speed: DefaultRevealSpeed}).Duration(revealText); got != want {
		t.Fatalf("zero speed gave %s, want the default %s", got, want)
	}
	if (Reveal{}).Duration(revealText) <= 0 {
		t.Fatal("the default speed must yield a usable duration")
	}
}

// Case: longer text takes longer, but only by half — both wavefronts travel at
// once, which is the whole reason this is not Line's pacing.
func TestReveal_DurationScalesWithHalfTheText(t *testing.T) {
	r := Reveal{Speed: 10}
	short, long := r.Duration("abcdefghij"), r.Duration("abcdefghijklmnopqrst")

	grew := long - short
	if want := 500 * time.Millisecond; grew < want-time.Millisecond || grew > want+time.Millisecond {
		t.Fatalf("ten more cells added %s, want %s — both ends open at once", grew, want)
	}
}

// Case: an unset Style, since RevealMirror is the zero value and a caller who
// configures only a Ramp should still get the baseline look.
func TestReveal_ZeroStyleIsMirror(t *testing.T) {
	ramp := Ramp{Base: movy.V3Of(0.2, 0.6, 1), Flash: movy.WhiteF}
	zero := Reveal{Ramp: ramp}
	named := Reveal{Ramp: ramp, Style: RevealMirror}

	if zero.Open(revealText, 0.4) != named.Open(revealText, 0.4) {
		t.Fatal("the zero Style should render as mirror")
	}
	if got := RevealStyle(0).String(); got != "mirror" {
		t.Fatalf("zero style names itself %q", got)
	}
}

// centreSample is early enough that bloom, the fastest style, has not yet
// reached the ends — the styles open at different rates by design.
const centreSample = 0.15

func testReveal(style RevealStyle) Reveal {
	return Reveal{
		Style: style,
		Ramp:  Ramp{Base: movy.V3Of(0.2, 0.4, 1), Flash: movy.WhiteF},
	}
}

// litColumns returns the indices holding a printable rune.
func litColumns(s string) []int {
	var cols []int
	for i, r := range []rune(plain(s)) {
		if r != ' ' {
			cols = append(cols, i)
		}
	}
	return cols
}

// Case: a divider handing over its rule rune, which is what lets the text part
// the rule and the rule close over it again instead of snapping.
func TestReveal_FillOccupiesUnclaimedCells(t *testing.T) {
	n := len([]rune(revealText))

	for _, style := range RevealStyles {
		t.Run(style.String(), func(t *testing.T) {
			r := testReveal(style)
			r.Fill, r.FillColor = '─', movy.V3Of(0.5, 0.5, 0.5)

			if got, want := plain(r.Open(revealText, 0)), strings.Repeat("─", n); got != want {
				t.Fatalf("closed frame is %q, want an unbroken rule — the entrance must start from the rule", got)
			}
			if got, want := plain(r.Open(revealText, 1)), revealText; got != want {
				t.Fatalf("open frame is %q, want %q — fill must not survive into the settled text", got, want)
			}
		})
	}
}

// Case: the trap in the above — the label's own spaces must stay spaces once
// open, not be mistaken for cells the text never claimed and filled with rule.
func TestReveal_FillDoesNotSwallowTheTextsOwnSpaces(t *testing.T) {
	const spaced = " a b "

	for _, style := range RevealStyles {
		r := testReveal(style)
		r.Fill, r.FillColor = '─', movy.V3Of(0.5, 0.5, 0.5)

		if got := plain(r.Open(spaced, 1)); got != spaced {
			t.Fatalf("%s opened %q as %q; the text's own spaces were filled", style, spaced, got)
		}
	}
}

// Case: no Fill configured, the default — unclaimed cells stay blank, so a
// caller compositing over its own background is unaffected.
func TestReveal_WithoutFillUnclaimedCellsAreBlank(t *testing.T) {
	for _, style := range RevealStyles {
		if got := strings.TrimSpace(plain(testReveal(style).Open(revealText, 0.3))); got == "" {
			t.Fatalf("%s rendered nothing at all mid-entrance", style)
		}
		if got := plain(testReveal(style).Open(revealText, 0)); strings.Trim(got, " ") != "" {
			t.Fatalf("%s drew %q into unclaimed cells with no Fill set", style, got)
		}
	}
}

// Case: the collapse, which must run the entrance backwards exactly — a
// separate implementation would drift from it as the styles are tuned.
func TestReveal_CloseIsTheEntranceReversed(t *testing.T) {
	for _, style := range RevealStyles {
		t.Run(style.String(), func(t *testing.T) {
			r := testReveal(style)

			if got := plain(r.Close(revealText, 0)); got != revealText {
				t.Fatalf("a collapse starts from %q, want the fully open %q", got, revealText)
			}
			if cols := litColumns(r.Close(revealText, 1)); len(cols) != 0 {
				t.Fatalf("a collapse left columns %v behind; the divider must end whole", cols)
			}

			// The rest guards against drift: Close is defined as the reversed
			// entrance, and reimplementing it separately would let the two part.
			for step := range 21 {
				p := float32(step) / 20
				if r.Close(revealText, p) != r.Open(revealText, 1-p) {
					t.Fatalf("collapse diverged from the reversed entrance at %.2f", p)
				}
			}
		})
	}
}

// Case: the collapse only ever takes text away, so it reads as one movement
// rather than flickering back open on the way in.
func TestReveal_CloseNeverReopens(t *testing.T) {
	for _, style := range RevealStyles {
		t.Run(style.String(), func(t *testing.T) {
			prev := len([]rune(revealText)) + 1
			for step := range 21 {
				progress := float32(step) / 20
				lit := len(litColumns(testReveal(style).Close(revealText, progress)))
				if lit > prev {
					t.Fatalf("lit runes rose from %d to %d at progress %.2f", prev, lit, progress)
				}
				prev = lit
			}
		})
	}
}

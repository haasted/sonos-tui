package nowplaying

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/ui/flashsweep"
)

// Case: a paused speaker, which must run a chain although nothing else in this
// file animates while playback is stopped.
func TestBreathRunsWhilePaused(t *testing.T) {
	m := pausedModel()

	_, cmd := m.Update(breathTickMsg{gen: *m.breathGen})
	if cmd == nil {
		t.Fatal("a paused speaker should keep breathing")
	}
}

// Case: a speaker that is muted while it plays. It makes no sound, so it is
// waiting even though the transport is not.
func TestBreathRunsWhileMutedAndPlaying(t *testing.T) {
	m := playingModel(0)
	m, _ = m.Update(VolumeMsg{Volume: 40, Muted: true})

	if !m.resting() {
		t.Fatal("a muted speaker is resting even while it plays")
	}
	if _, cmd := m.Update(breathTickMsg{gen: *m.breathGen}); cmd == nil {
		t.Fatal("a muted speaker should keep breathing")
	}
}

// Case: playback resuming, which must end the chain rather than leave it
// ticking for a speaker that now spins.
func TestBreathStopsWhenPlaybackResumes(t *testing.T) {
	m := pausedModel()
	gen := *m.breathGen

	m, _ = m.Update(TrackMsg{Track: Track{Title: "So What", Playing: true}})

	if !m.restingSince.IsZero() {
		t.Fatal("a playing speaker holds no breath phase")
	}
	if _, cmd := m.Update(breathTickMsg{gen: gen}); cmd != nil {
		t.Fatal("a tick from the paused chain must not re-arm once playing")
	}
}

// Case: a tick from a chain an earlier state started. Three faults in this
// project came from a chain that outlived what started it.
func TestBreathIgnoresAStaleTick(t *testing.T) {
	m := pausedModel()
	stale := *m.breathGen

	m, _ = m.Update(VolumeMsg{Volume: 50})

	if _, cmd := m.Update(breathTickMsg{gen: stale}); cmd != nil {
		t.Fatal("a tick from a rolled generation must not re-arm")
	}
}

// Case: the moment the reader presses mute. The removed flash answered at
// once, and a breath that begins at its trough would leave the glyph dark for
// two seconds instead.
func TestBreathStartsLit(t *testing.T) {
	m := playingModel(0)
	m, _ = m.Update(VolumeMsg{Volume: 40, Muted: true})

	assertGlyph(t, m, mutedGlyph)
	assertNearer(t, m, mutedBreathLit(), restingBreathBase(), "muting should light the glyph at once")
}

// Case: the far side of the cycle, which must not look like the near side.
// Without this the test above would pass on a breath that never moves.
func TestBreathDarkensLaterInTheCycle(t *testing.T) {
	m := playingModel(0)
	m, _ = m.Update(VolumeMsg{Volume: 40, Muted: true})
	entry := m.View()

	m.restingSince = time.Now().Add(-flashsweep.BreathPeriod / 2)

	if m.View() == entry {
		t.Fatal("the glyph should be dimmer half a period into the breath")
	}
	assertNearer(t, m, restingBreathBase(), mutedBreathLit(),
		"half a period in, the breath should be near its floor")
}

// Case: mute arriving at a speaker that is already paused. It is still the
// same wait, so the breath must carry on rather than drop to its trough.
func TestMuteWhileAlreadyPausedKeepsThePhase(t *testing.T) {
	m := pausedModel()
	began := m.restingSince

	m, _ = m.Update(VolumeMsg{Volume: 40, Muted: true})

	if !m.restingSince.Equal(began) {
		t.Fatal("muting an already-paused speaker should not restart the breath")
	}
}

// Case: a speaker that is already paused when the program starts. Nothing
// entered the resting state here, so a phase derived from the change would
// never be set and the breath would stand still at whatever brightness the
// unset clock happened to give it.
func TestBreathStartsLitOnASpeakerAlreadyPaused(t *testing.T) {
	m := pausedModel()

	if m.restingSince.IsZero() {
		t.Fatal("a speaker that starts paused holds no breath phase")
	}

	assertGlyph(t, m, pausedGlyph)
	assertNearer(t, m, pausedBreathLit(), restingBreathBase(),
		"a speaker that starts paused should breathe from lit")
}

// Case: a speaker that is already muted when the program starts, which reaches
// the same trap by the other road.
func TestBreathStartsLitOnASpeakerAlreadyMuted(t *testing.T) {
	m := New(Track{Title: "So What", Playing: true, Muted: true, Volume: 40})

	if m.restingSince.IsZero() {
		t.Fatal("a speaker that starts muted holds no breath phase")
	}

	assertGlyph(t, m, mutedGlyph)
	assertNearer(t, m, mutedBreathLit(), restingBreathBase(),
		"a speaker that starts muted should breathe from lit")
}

// Case: the width rule the glyphs live under. View derives the indent of the
// rows below from the glyph, so a glyph of two cells moves the whole panel.
func TestRestingGlyphsAreOneCell(t *testing.T) {
	for _, glyph := range []string{pausedGlyph, mutedGlyph} {
		if w := lipgloss.Width(glyph); w != 1 {
			t.Fatalf("%q is %d cells wide, which moves the panel", glyph, w)
		}
	}
}

// Case: the panel across all three states, which must stay where it is. The
// width check above is the cause; this is the effect a reader would see.
func TestThePanelDoesNotMoveBetweenStates(t *testing.T) {
	playing := playingModel(0)
	paused := pausedModel()
	muted, _ := playing.Update(VolumeMsg{Volume: 40, Muted: true})

	want := artistIndent(playing)
	if got := artistIndent(paused); got != want {
		t.Fatalf("paused indents the artist by %d, playing by %d", got, want)
	}
	if got := artistIndent(muted); got != want {
		t.Fatalf("muted indents the artist by %d, playing by %d", got, want)
	}
}

// Case: the glyph a reader sees while muted, which used to be the letter X.
func TestTheMutedGlyphIsNotALetter(t *testing.T) {
	if mutedGlyph >= "A" && mutedGlyph <= "z" {
		t.Fatalf("%q reads as text among the braille glyphs", mutedGlyph)
	}
}

// assertGlyph fails unless the panel leads with the mark the state calls for.
func assertGlyph(t *testing.T, m Model, glyph string) {
	t.Helper()
	if !strings.Contains(m.View(), glyph) {
		t.Fatalf("the panel should show %q", glyph)
	}
}

// assertNearer fails unless the glyph on screen sits closer to want than to
// the far end of its breath. A comparison rather than an exact colour: the peak
// is flat but it is not still, so an exact match would turn on a millisecond.
func assertNearer(t *testing.T, m Model, want, far color.Color, msg string) {
	t.Helper()

	got := glyphColour(t, m.View())
	if distance(got, want) >= distance(got, far) {
		t.Fatalf("%s: %v is nearer %v than %v", msg, got, rgb(far), rgb(want))
	}
}

// glyphColour is the truecolor foreground of the glyph the panel leads with.
func glyphColour(t *testing.T, view string) [3]int {
	t.Helper()
	var c [3]int
	if _, err := fmt.Sscanf(view, "\x1b[38;2;%d;%d;%dm", &c[0], &c[1], &c[2]); err != nil {
		t.Fatalf("no truecolor foreground at the head of %q", view)
	}
	return c
}

func rgb(c color.Color) [3]int {
	r, g, b, _ := c.RGBA()
	return [3]int{int(r >> 8), int(g >> 8), int(b >> 8)}
}

// distance is how far apart two colours are, squared. Only the ordering
// matters here, so the root is not worth taking.
func distance(got [3]int, want color.Color) int {
	w := rgb(want)
	total := 0
	for i := range got {
		d := got[i] - w[i]
		total += d * d
	}
	return total
}

func pausedModel() Model {
	return New(Track{Title: "So What", Album: "Kind of Blue", Volume: 40,
		Elapsed: 60 * time.Second, Duration: 300 * time.Second})
}

// artistIndent is the blank run the artist row starts with, which View derives
// from the width of whichever glyph the state calls for.
func artistIndent(m Model) int {
	lines := strings.Split(m.View(), "\n")
	if len(lines) < 2 {
		return -1
	}
	return len(lines[1]) - len(strings.TrimLeft(lines[1], " "))
}

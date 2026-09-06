package panes_test

import (
	"image/color"
	"regexp"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

var fgCode = regexp.MustCompile(`38;2;\d+;\d+;\d+`)

func twoTabs() panes.Tabs {
	return panes.NewTabs([]panes.Tab{
		{Title: "Living Room", Content: panes.StaticContent("a")},
		{Title: "Kitchen", Content: panes.StaticContent("b")},
	})
}

func distinctForegrounds(s string) int {
	seen := map[string]bool{}
	for _, m := range fgCode.FindAllString(s, -1) {
		seen[m] = true
	}
	return len(seen)
}

func viewAt(t *testing.T, tabs panes.Tabs, at time.Duration) string {
	t.Helper()
	for tabs.SweepElapsed() < at {
		time.Sleep(2 * time.Millisecond)
	}
	return tabs.View()
}

func rightKey() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyRight} }

func TestTabs_SwitchingStartsAGlint(t *testing.T) {
	settled := distinctForegrounds(twoTabs().View())

	tabs, cmd := twoTabs().Update(rightKey())
	if cmd == nil {
		t.Fatal("expected a command to drive the glint")
	}

	during := distinctForegrounds(viewAt(t, tabs, panes.SweepDuration/2))
	if during <= settled {
		t.Fatalf("expected the glint to paint per-character colours: %d distinct mid-sweep vs %d settled", during, settled)
	}
}

func TestTabs_GlintIsStrongEnoughToSee(t *testing.T) {
	tabs, _ := twoTabs().Update(rightKey())

	settled := peakChannelSpread(twoTabs().View())
	peak := peakChannelSpread(viewAt(t, tabs, panes.SweepDuration/2))

	if peak-settled < 90 {
		t.Fatalf("glint too faint to notice: peak %d vs settled %d", peak, settled)
	}
}

func peakChannelSpread(s string) int {
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

func TestTabs_GlintDoesNotChangeHeaderWidth(t *testing.T) {
	tabs, _ := twoTabs().Update(rightKey())

	during := lipglossWidth(tabs.View())
	time.Sleep(panes.SweepDuration + 20*time.Millisecond)
	after := lipglossWidth(tabs.View())

	if during != after {
		t.Fatalf("tab header changed width mid-glint: %d during vs %d settled", during, after)
	}
}

func TestTabs_GlintStopsAfterItsDuration(t *testing.T) {
	tabs, _ := twoTabs().Update(rightKey())
	gen := tabs.SweepGen()

	time.Sleep(panes.SweepDuration + 20*time.Millisecond)

	_, cmd := tabs.Update(panes.NewSweepTick(gen))
	if cmd != nil {
		t.Fatal("expected the glint to stop rescheduling once its duration elapsed")
	}
}

func TestTabs_StaleGlintTickIsIgnored(t *testing.T) {
	tabs, _ := twoTabs().Update(rightKey())
	stale := tabs.SweepGen()

	tabs, _ = tabs.Update(rightKey())
	current := tabs.SweepGen()

	if stale == current {
		t.Fatal("each switch should start its own glint generation")
	}

	if _, cmd := tabs.Update(panes.NewSweepTick(stale)); cmd != nil {
		t.Fatal("a tick from a superseded glint should not keep ticking")
	}
	if _, cmd := tabs.Update(panes.NewSweepTick(current)); cmd == nil {
		t.Fatal("the current glint should still be ticking")
	}
}

func TestTabs_NoGlintBeforeAnySwitch(t *testing.T) {
	tabs := twoTabs()
	if tabs.SweepElapsed() != 0 {
		t.Fatal("a freshly built Tabs should not have a glint in flight")
	}
	if _, cmd := tabs.Update(panes.NewSweepTick(0)); cmd != nil {
		t.Fatal("an unsolicited tick should not start a glint")
	}
}

func lipglossWidth(s string) int { return lipgloss.Width(s) }

// Case: a divider sweep in flight, which must paint the line per-character
// instead of the single flat separator colour.
func TestTabs_DividerSweepPaintsTheLine(t *testing.T) {
	tabs := twoTabs()
	resting := distinctForegrounds(tabs.View())

	tabs = panes.ForceDividerSweep(tabs, false)
	tabs = panes.SetDividerElapsed(tabs, panes.DividerSweepDuration/2)

	if during := distinctForegrounds(tabs.View()); during <= resting {
		t.Fatalf("expected the sweep to paint per-character colours: %d distinct vs %d resting", during, resting)
	}
}

// Case: the two directions, which must be mirror images at the same progress.
func TestTabs_DividerSweepRunsBothWays(t *testing.T) {
	// Pin the palette: the colour is rolled per sweep, and comparing the
	// brightest cell across two different colours measures the colour, not
	// the direction.
	original := panes.DividerSweepColors
	panes.DividerSweepColors = original[:1]
	defer func() { panes.DividerSweepColors = original }()

	at := func(reverse bool, frac float64) int {
		tabs := twoTabs()
		tabs = panes.ForceDividerSweep(tabs, reverse)
		tabs = panes.SetDividerElapsed(tabs, time.Duration(frac*float64(panes.DividerSweepDuration)))
		return brightestIndexIn(tabs.View())
	}

	if forward, back := at(false, 0.2), at(true, 0.2); forward >= back {
		t.Fatalf("at the same progress the reverse sweep should be further right: forward %d, reverse %d", forward, back)
	}
}

// Case: a finished sweep, which must restore the plain divider.
func TestTabs_DividerSweepEndsCleanly(t *testing.T) {
	tabs := twoTabs()
	resting := distinctForegrounds(tabs.View())

	tabs = panes.ForceDividerSweep(tabs, false)
	tabs = panes.SetDividerElapsed(tabs, panes.DividerSweepDuration+time.Second)

	if after := distinctForegrounds(tabs.View()); after != resting {
		t.Fatalf("after the sweep the divider should be flat again: %d distinct vs %d resting", after, resting)
	}
}

func brightestIndexIn(view string) int {
	best, bestIdx := -1, -1
	for i, m := range regexp.MustCompile(`38;2;(\d+);(\d+);(\d+)`).FindAllStringSubmatch(view, -1) {
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

// Case: a schedule tick from a superseded generation, which must not start a
// sweep — the guard that stops a re-Init leaving two timer chains running.
func TestTabs_StaleDividerScheduleIsIgnored(t *testing.T) {
	tabs := twoTabs()
	tabs, _ = tabs.Update(panes.NewDividerSchedule(panes.DividerGen(tabs) + 999))

	if _, sweeping := panes.DividerActive(tabs); sweeping {
		t.Fatal("a schedule tick from another generation must not start a sweep")
	}
}

// Case: repeated sweeps, which must vary in colour rather than always using
// one, while holding that colour steady for the sweep's duration.
func TestTabs_DividerSweepRollsAColourPerSweep(t *testing.T) {
	seen := map[color.Color]bool{}
	for range 100 {
		seen[panes.DividerColor(panes.ForceDividerSweep(twoTabs(), false))] = true
	}
	if len(seen) < 2 {
		t.Fatalf("expected sweeps to vary in colour, saw only %d", len(seen))
	}

	tabs := panes.ForceDividerSweep(twoTabs(), false)
	fixed := panes.DividerColor(tabs)
	for _, frac := range []float64{0.1, 0.4, 0.9} {
		moved := panes.SetDividerElapsed(tabs, time.Duration(frac*float64(panes.DividerSweepDuration)))
		if panes.DividerColor(moved) != fixed {
			t.Fatal("the colour must hold for the whole sweep, or it strobes every frame")
		}
	}
}

// Case: an emptied palette, which must degrade to an invisible sweep rather
// than panicking on an empty slice.
func TestTabs_DividerSweepToleratesAnEmptyPalette(t *testing.T) {
	original := panes.DividerSweepColors
	panes.DividerSweepColors = nil
	defer func() { panes.DividerSweepColors = original }()

	tabs := panes.ForceDividerSweep(twoTabs(), false)
	tabs = panes.SetDividerElapsed(tabs, panes.DividerSweepDuration/2)
	tabs.View() // must not panic
}

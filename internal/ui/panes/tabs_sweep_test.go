package panes_test

import (
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

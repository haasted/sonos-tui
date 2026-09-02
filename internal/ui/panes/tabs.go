// Package panes holds reusable Bubbletea UI components shared across the
// real app and the cmd/uitest playground.
package panes

import (
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

// Tab is one entry in a Tabs component: a header label plus the model
// rendered while it's active.
type Tab struct {
	Title   string
	Content tea.Model
}

// Position controls whether the header row renders above or below the
// active tab's content.
type Position int

const (
	Top Position = iota
	Bottom
)

type Tabs struct {
	Items      []Tab
	Active     int
	Position   Position // zero value (Top) renders headers above the content
	Width      int      // 0 = unconstrained; otherwise headers scroll to fit, see headerRow
	HideBorder bool     // zero value (false) keeps the rounded border; true leaves only the divider bar between tabs and content

	sweepGen   int
	sweepStart time.Time
}

const (
	tabSweepDuration = 620 * time.Millisecond
	tabSweepInterval = 22 * time.Millisecond
)

var tabSweepGenerations atomic.Int64

func nextTabSweepGen() int { return int(tabSweepGenerations.Add(1)) }

type tabSweepTickMsg struct {
	gen int
}

func tabSweepTick(gen int) tea.Cmd {
	return tea.Tick(tabSweepInterval, func(time.Time) tea.Msg {
		return tabSweepTickMsg{gen: gen}
	})
}

func (t Tabs) activate() (Tabs, tea.Cmd) {
	initCmd := t.Items[t.Active].Content.Init()

	t.sweepGen = nextTabSweepGen()
	t.sweepStart = time.Now()

	return t, tea.Batch(initCmd, tabSweepTick(t.sweepGen))
}

func (t Tabs) sweepProgress() (float32, bool) {
	if t.sweepStart.IsZero() {
		return 0, false
	}
	elapsed := time.Since(t.sweepStart)
	if elapsed >= tabSweepDuration {
		return 0, false
	}
	return float32(elapsed) / float32(tabSweepDuration), true
}

// NewTabs builds a Tabs component, starting on the first tab.
func NewTabs(tabs []Tab) Tabs {
	return Tabs{Items: tabs}
}

func (t Tabs) AddTab(tab Tab) Tabs {
	t.Items = append(t.Items, tab)
	return t
}

func (t Tabs) InsertTabAt(index int, tab Tab) Tabs {
	if index < 0 {
		index = 0
	}
	if index > len(t.Items) {
		index = len(t.Items)
	}
	hadTabs := len(t.Items) > 0

	t.Items = append(t.Items, Tab{})
	copy(t.Items[index+1:], t.Items[index:])
	t.Items[index] = tab

	if hadTabs && index <= t.Active {
		t.Active++
	}
	return t
}

// Init forwards to the initially active tab's content.
func (t Tabs) Init() tea.Cmd {
	if len(t.Items) == 0 {
		return nil
	}
	return t.Items[t.Active].Content.Init()
}

// Update switches tabs on navigation keys, otherwise forwards to the active tab's content.
func (t Tabs) Update(msg tea.Msg) (Tabs, tea.Cmd) {
	if len(t.Items) == 0 {
		return t, nil
	}

	if tick, ok := msg.(tabSweepTickMsg); ok {
		if tick.gen != t.sweepGen {
			return t, nil
		}
		if _, sweeping := t.sweepProgress(); sweeping {
			return t, tabSweepTick(tick.gen)
		}
		return t, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch key := keyMsg.String(); key {
		case "right", "tab":
			t.Active = (t.Active + 1) % len(t.Items)
			return t.activate()
		case "left", "shift+tab":
			t.Active = (t.Active - 1 + len(t.Items)) % len(t.Items)
			return t.activate()
		case "0":
			t.Active = len(t.Items) - 1
			return t.activate()
		default:
			if idx, ok := tabIndexFromKey(key); ok && idx < len(t.Items) {
				t.Active = idx
				return t.activate()
			}
		}
	}

	content, cmd := t.Items[t.Active].Content.Update(msg)
	t.Items[t.Active].Content = content
	return t, cmd
}

// tabIndexFromKey maps digit keys "1"-"9" to zero-based tab indices.
func tabIndexFromKey(key string) (int, bool) {
	if len(key) != 1 || key[0] < '1' || key[0] > '9' {
		return 0, false
	}
	return int(key[0] - '1'), true
}

func (t Tabs) View() string {
	if len(t.Items) == 0 {
		return ""
	}

	row := t.headerRow()

	// Drop the border edge facing the tab row — a plain rule stands in for
	// it instead, so the tabs and content read as one connected panel
	// rather than two separate boxes with a gap between them. HideBorder
	// drops the other three edges too, leaving just that divider bar, and
	// also trims the padding on that same divider-facing side, so content
	// sits flush against the divider instead of leaving a blank line.
	box := contentStyle
	if t.HideBorder {
		box = box.BorderTop(false).BorderRight(false).BorderBottom(false).BorderLeft(false)
		if t.Position == Bottom {
			box = box.PaddingBottom(0)
		} else {
			box = box.PaddingTop(0)
		}
	} else if t.Position == Bottom {
		box = box.BorderBottom(false)
	} else {
		box = box.BorderTop(false)
	}
	content := box.Render(t.Items[t.Active].Content.View().Content)

	divider := separatorStyle.Render(strings.Repeat("─", max(lipgloss.Width(row), lipgloss.Width(content))))

	if t.Position == Bottom {
		return lipgloss.JoinVertical(lipgloss.Left, content, divider, row)
	}
	return lipgloss.JoinVertical(lipgloss.Left, row, divider, content)
}

// headerRow renders the tab labels, windowed around Active with ‹/›
// scroll indicators when they don't all fit within Width. Width <= 0 means
// unconstrained: every tab renders, as before Width existed.
func (t Tabs) headerRow() string {
	labels := make([]string, len(t.Items))
	widths := make([]int, len(t.Items))
	for i, tab := range t.Items {
		style := inactiveTabStyle
		label := tab.Title
		if i == t.Active {
			style = activeTabStyle
			if progress, sweeping := t.sweepProgress(); sweeping {
				label = flashsweep.Glint(tab.Title, progress, tabSweepFg)
			}
		}
		labels[i] = style.Render(label)
		widths[i] = lipgloss.Width(labels[i])
	}

	full := lipgloss.JoinHorizontal(lipgloss.Top, labels...)
	if t.Width <= 0 || lipgloss.Width(full) <= t.Width {
		return full
	}

	// Doesn't fit: reserve room for both indicators up front (simpler than
	// discovering after the fact that only one side ended up clipped, at
	// the cost of occasionally not using a sliver of space we didn't need
	// to reserve), then grow a window outward from Active, alternating
	// sides, until neither side has room left to add its next tab.
	indicatorWidth := lipgloss.Width(inactiveTabStyle.Render("‹"))
	budget := t.Width - 2*indicatorWidth
	if budget < widths[t.Active] {
		budget = widths[t.Active] // always show at least the active tab
	}

	lo, hi := t.Active, t.Active
	width := widths[t.Active]
	tryRight := true
	for lo > 0 || hi < len(t.Items)-1 {
		grew := false
		if tryRight && hi < len(t.Items)-1 && width+widths[hi+1] <= budget {
			hi++
			width += widths[hi]
			grew = true
		} else if !tryRight && lo > 0 && width+widths[lo-1] <= budget {
			lo--
			width += widths[lo]
			grew = true
		}
		if !grew {
			// Preferred side was out of room or exhausted — fall back to
			// whichever side still has room, if either does.
			if hi < len(t.Items)-1 && width+widths[hi+1] <= budget {
				hi++
				width += widths[hi]
			} else if lo > 0 && width+widths[lo-1] <= budget {
				lo--
				width += widths[lo]
			} else {
				break
			}
		}
		tryRight = !tryRight
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, labels[lo:hi+1]...)
	if lo > 0 {
		row = inactiveTabStyle.Render("‹") + row
	}
	if hi < len(t.Items)-1 {
		row += inactiveTabStyle.Render("›")
	}
	return row
}

// Colors go through theme.LightDark so the palette stays legible whether the
// user's terminal background is light or dark, rather than assuming one or the other.
var (
	activeTabAccent = theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24"))

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Foreground(activeTabAccent)

	tabSweepFg = flashsweep.Ramp{
		Base:  flashsweep.FromColor(activeTabAccent),
		Flash: flashsweep.FromColor(theme.LightDark(lipgloss.Color("#F59E0B"), lipgloss.Color("#FFFFFF"))),
	}

	inactiveTabStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))

	separatorStyle = lipgloss.NewStyle().
			Foreground(theme.LightDark(lipgloss.Color("#D0D0D0"), lipgloss.Color("#4A4A4A")))

	contentStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.LightDark(lipgloss.Color("#D0D0D0"), lipgloss.Color("#4A4A4A"))).
			Padding(1, 2)
)

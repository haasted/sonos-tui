// Package panes holds reusable Bubbletea UI components shared across the
// real app and the cmd/uitest playground.
package panes

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// Tabs is a hand-rolled tabbed pane, like browser tabs: a row of headers
// alongside whichever tab's content is currently active (see Position for
// which side). Left/right and tab/shift+tab step through tabs; digit keys
// 1-9 jump straight to a tab, and 0 jumps to the right-most tab (mirroring
// the usual browser/terminal tab-shortcut convention). Every other message
// is forwarded to the active tab's Content, so a tab's own content can't
// itself use left/right or digit keys as input for now. Switching tabs
// re-runs the newly active tab's Content.Init(), so a self-ticking model
// (e.g. an animation) restarts cleanly each time its tab regains focus.
type Tabs struct {
	Tabs       []Tab
	Active     int
	Position   Position // zero value (Top) renders headers above the content
	Width      int      // 0 = unconstrained; otherwise headers scroll to fit, see headerRow
	HideBorder bool     // zero value (false) keeps the rounded border; true leaves only the divider bar between tabs and content
}

// NewTabs builds a Tabs component, starting on the first tab.
func NewTabs(tabs []Tab) Tabs {
	return Tabs{Tabs: tabs}
}

func (t Tabs) AddTab(tab Tab) Tabs {
	t.Tabs = append(t.Tabs, tab)
	return t
}

func (t Tabs) InsertTabAt(index int, tab Tab) Tabs {
	if index < 0 {
		index = 0
	}
	if index > len(t.Tabs) {
		index = len(t.Tabs)
	}
	hadTabs := len(t.Tabs) > 0

	t.Tabs = append(t.Tabs, Tab{})
	copy(t.Tabs[index+1:], t.Tabs[index:])
	t.Tabs[index] = tab

	if hadTabs && index <= t.Active {
		t.Active++
	}
	return t
}

// Init forwards to the initially active tab's content.
func (t Tabs) Init() tea.Cmd {
	if len(t.Tabs) == 0 {
		return nil
	}
	return t.Tabs[t.Active].Content.Init()
}

// Update switches tabs on navigation keys, otherwise forwards to the active tab's content.
func (t Tabs) Update(msg tea.Msg) (Tabs, tea.Cmd) {
	if len(t.Tabs) == 0 {
		return t, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch key := keyMsg.String(); key {
		case "right", "tab":
			t.Active = (t.Active + 1) % len(t.Tabs)
			return t, t.Tabs[t.Active].Content.Init()
		case "left", "shift+tab":
			t.Active = (t.Active - 1 + len(t.Tabs)) % len(t.Tabs)
			return t, t.Tabs[t.Active].Content.Init()
		case "0":
			t.Active = len(t.Tabs) - 1
			return t, t.Tabs[t.Active].Content.Init()
		default:
			if idx, ok := tabIndexFromKey(key); ok && idx < len(t.Tabs) {
				t.Active = idx
				return t, t.Tabs[t.Active].Content.Init()
			}
		}
	}

	content, cmd := t.Tabs[t.Active].Content.Update(msg)
	t.Tabs[t.Active].Content = content
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
	if len(t.Tabs) == 0 {
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
	content := box.Render(t.Tabs[t.Active].Content.View())

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
	labels := make([]string, len(t.Tabs))
	widths := make([]int, len(t.Tabs))
	for i, tab := range t.Tabs {
		style := inactiveTabStyle
		if i == t.Active {
			style = activeTabStyle
		}
		labels[i] = style.Render(tab.Title)
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
	for lo > 0 || hi < len(t.Tabs)-1 {
		grew := false
		if tryRight && hi < len(t.Tabs)-1 && width+widths[hi+1] <= budget {
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
			if hi < len(t.Tabs)-1 && width+widths[hi+1] <= budget {
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
	if hi < len(t.Tabs)-1 {
		row += inactiveTabStyle.Render("›")
	}
	return row
}

// Colors are AdaptiveColor so the palette stays legible whether the user's
// terminal background is light or dark, rather than assuming one or the other.
var (
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#6B46C1", Dark: "#A855F7"})

	inactiveTabStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Foreground(lipgloss.AdaptiveColor{Light: "#4A4A4A", Dark: "#9B9B9B"})

	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#D0D0D0", Dark: "#4A4A4A"})

	contentStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#D0D0D0", Dark: "#4A4A4A"}).
			Padding(1, 2)
)

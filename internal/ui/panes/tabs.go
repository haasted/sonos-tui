// Package panes holds reusable Bubbletea UI components shared across the
// real app and the cmd/uitest playground.
package panes

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Tab is one entry in a Tabs component: a header label plus the model
// rendered while it's active.
type Tab struct {
	Title   string
	Content tea.Model
}

// Tabs is a hand-rolled tabbed pane, like browser tabs: a row of headers
// above whichever tab's content is currently active. Left/right and
// tab/shift+tab step through tabs; digit keys 1-9 jump straight to a tab,
// and 0 jumps to the right-most tab (mirroring the usual browser/terminal
// tab-shortcut convention). Every other message is forwarded to the active
// tab's Content, so a tab's own content can't itself use left/right or
// digit keys as input for now.
type Tabs struct {
	Tabs   []Tab
	Active int
}

// NewTabs builds a Tabs component, starting on the first tab.
func NewTabs(tabs []Tab) Tabs {
	return Tabs{Tabs: tabs}
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
			return t, nil
		case "left", "shift+tab":
			t.Active = (t.Active - 1 + len(t.Tabs)) % len(t.Tabs)
			return t, nil
		case "0":
			t.Active = len(t.Tabs) - 1
			return t, nil
		default:
			if idx, ok := tabIndexFromKey(key); ok && idx < len(t.Tabs) {
				t.Active = idx
				return t, nil
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

	headers := make([]string, len(t.Tabs))
	for i, tab := range t.Tabs {
		style := inactiveTabStyle
		if i == t.Active {
			style = activeTabStyle
		}
		headers[i] = style.Render(tab.Title)
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, headers...)
	content := contentStyle.Render(t.Tabs[t.Active].Content.View())
	return lipgloss.JoinVertical(lipgloss.Left, row, content)
}

var (
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Underline(true).
			Foreground(lipgloss.Color("212")).
			Padding(0, 2)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")).
				Padding(0, 2)

	contentStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2).
			MarginTop(1)
)

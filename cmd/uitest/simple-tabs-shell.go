package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// simpleTabsModel is the minimal Bubbletea root model every plain
// Tabs-wrapped layout in this playground needs: q/ctrl+c to quit, inline
// (no alt-screen) rendering that erases itself on the way out, and
// WindowSizeMsg forwarded so Tabs can window its header row. Pulled out
// once "interface" mockups started needing the exact same shell "tabs" and
// "effects" already had, copy-pasted twice.
type simpleTabsModel struct {
	tabs     panes.Tabs
	quitting bool
}

func newSimpleTabsModel(tabs panes.Tabs) simpleTabsModel {
	return simpleTabsModel{tabs: tabs}
}

func (m simpleTabsModel) Init() tea.Cmd {
	return m.tabs.Init()
}

func (m simpleTabsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.tabs.Width = sizeMsg.Width
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}

	tabs, cmd := m.tabs.Update(msg)
	m.tabs = tabs
	return m, cmd
}

func (m simpleTabsModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	return tea.NewView(m.tabs.View() + "\n\ntab/shift+tab or ←/→ to switch tabs · q to quit\n")
}

func runSimpleTabs(tabs panes.Tabs) {
	if _, err := tea.NewProgram(newSimpleTabsModel(tabs)).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

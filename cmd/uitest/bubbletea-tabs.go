package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// tabsRootModel is the outermost Bubbletea model for the "tabs" layout —
// wraps panes.Tabs with a q/ctrl+c quit binding so the playground has
// something runnable.
type tabsRootModel struct {
	tabs panes.Tabs
}

var roomNames = []string{
	"Kitchen",
	"Living Room",
	"Bedroom",
	"Bathroom",
	"Office",
	"Dining Room",
	"Garage",
	"Patio",
}

func newTabsRootModel() tabsRootModel {
	tabs := make([]panes.Tab, len(roomNames))
	for i, room := range roomNames {
		content := fmt.Sprintf("Nothing playing yet in the %s.", room)
		tabs[i] = panes.Tab{Title: room, Content: panes.StaticContent(content)}
	}
	return tabsRootModel{tabs: panes.NewTabs(tabs)}
}

func (m tabsRootModel) Init() tea.Cmd {
	return m.tabs.Init()
}

func (m tabsRootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}

	tabs, cmd := m.tabs.Update(msg)
	m.tabs = tabs
	return m, cmd
}

func (m tabsRootModel) View() string {
	return m.tabs.View() + "\n\ntab/shift+tab or ←/→ to switch tabs · 1-8 to jump, 0 for last · q to quit\n"
}

func layoutTabs() {
	if _, err := tea.NewProgram(newTabsRootModel()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

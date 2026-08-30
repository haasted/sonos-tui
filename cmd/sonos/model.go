package main

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

type model struct {
	tabs          panes.Tabs
	width         int
	height        int
	discoveryDone bool
}

func newModel() model {
	tabs := panes.NewTabs(nil)
	tabs.Position = panes.Bottom
	tabs.HideBorder = true
	return model{tabs: tabs}
}

func (m model) Init() tea.Cmd {
	return m.tabs.Init()
}

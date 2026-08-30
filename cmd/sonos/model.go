package main

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

type model struct {
	tabs          panes.Tabs
	ips           []string
	events        *sonos.EventManager
	width         int
	height        int
	discoveryDone bool
}

var _ tea.Model = &model{}

func newModel(events *sonos.EventManager) model {
	tabs := panes.NewTabs(nil)
	tabs.Position = panes.Bottom
	tabs.HideBorder = true
	return model{tabs: tabs, events: events}
}

func (m model) Init() tea.Cmd {
	return m.tabs.Init()
}

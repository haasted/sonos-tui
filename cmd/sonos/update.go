package main

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.tabs.Width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case speakerFoundMsg:
		return m, loadRoom(msg.speaker)
	case roomReadyMsg:
		wasEmpty := len(m.tabs.Tabs) == 0
		idx := sort.SearchStrings(m.ips, msg.ip)
		m.ips = append(m.ips, "")
		copy(m.ips[idx+1:], m.ips[idx:])
		m.ips[idx] = msg.ip
		m.tabs = m.tabs.InsertTabAt(idx, panes.Tab{Title: msg.title, Content: msg.room})
		if wasEmpty {
			return m, msg.room.Init()
		}
		return m, nil
	case discoveryDoneMsg:
		m.discoveryDone = true
		return m, nil
	}

	tabs, cmd := m.tabs.Update(msg)
	m.tabs = tabs
	return m, cmd
}

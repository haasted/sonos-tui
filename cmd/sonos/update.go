package main

import (
	"log/slog"
	"sort"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/nowplaying"
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
		return m, loadRoom(m.events, msg.speaker)
	case roomReadyMsg:
		wasEmpty := len(m.tabs.Tabs) == 0
		idx := sort.SearchStrings(m.ips, msg.ip)
		m.ips = append(m.ips, "")
		copy(m.ips[idx+1:], m.ips[idx:])
		m.ips[idx] = msg.ip
		m.tabs = m.tabs.InsertTabAt(idx, panes.Tab{Title: msg.title, Content: msg.room})

		var cmds []tea.Cmd
		if m.events != nil {
			cmds = append(cmds, waitForEvent(msg.room.device))
		}
		if wasEmpty {
			cmds = append(cmds, msg.room.Init())
		}
		return m, tea.Batch(cmds...)
	case sonos.Event:
		return m.applyEvent(msg)
	case discoveryDoneMsg:
		m.discoveryDone = true
		return m, nil
	}

	tabs, cmd := m.tabs.Update(msg)
	m.tabs = tabs
	return m, cmd
}

func (m model) applyEvent(event sonos.Event) (tea.Model, tea.Cmd) {
	for i, tab := range m.tabs.Tabs {
		room, ok := tab.Content.(roomModel)
		if !ok || !isSpeaker(room.device, event.Speaker) {
			continue
		}

		updated, cmd := room.Update(nowplaying.TrackMsg{Track: trackFromState(room.device.State())})
		m.tabs.Tabs[i].Content = updated

		if i != m.tabs.Active {
			cmd = nil
		}
		return m, tea.Batch(cmd, waitForEvent(room.device))
	}

	slog.Warn("event for a speaker with no room — dropping", "speaker", event.Speaker)
	return m, nil
}

func isSpeaker(device *sonos.Device, speaker sonos.Speaker) bool {
	if device.USN != "" && speaker.USN != "" {
		return device.USN == speaker.USN
	}
	return device.IP == speaker.IP
}

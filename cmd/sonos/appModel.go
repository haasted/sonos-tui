package main

import (
	"log/slog"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/nowplaying"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

var _ tea.Model = &appModel{}

const maxPlayerHeightRows = 7

type appModel struct {
	tabs          panes.Tabs
	ips           []string
	eventManager  *sonos.EventManager
	width         int
	height        int
	discoveryDone bool
	quitting      bool

	verticalSpaceChecked bool
}

func newAppModel(events *sonos.EventManager) appModel {
	tabs := panes.NewTabs(nil)
	tabs.Position = panes.Bottom
	tabs.HideBorder = true
	return appModel{tabs: tabs, eventManager: events}
}

func (m appModel) Init() tea.Cmd {
	return m.tabs.Init()
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		firstResize := m.height == 0
		m.width = msg.Width
		m.height = msg.Height
		m.tabs.Width = msg.Width
		if firstResize {
			return m, tea.RequestCursorPosition
		}
	case tea.CursorPositionMsg:
		if m.verticalSpaceChecked {
			return m, nil
		}
		m.verticalSpaceChecked = true
		if needed := maxPlayerHeightRows - (m.height - msg.Y); needed > 0 {
			return m, tea.Println(strings.Repeat("\n", needed-1))
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Sequence(tea.ClearScreen, tea.Quit)
		}
	case speakerFoundMsg:
		return m, loadRoom(m.eventManager, msg.speaker)
	case roomReadyMsg:
		wasEmpty := len(m.tabs.Items) == 0
		idx := sort.SearchStrings(m.ips, msg.ip)
		m.ips = append(m.ips, "")
		copy(m.ips[idx+1:], m.ips[idx:])
		m.ips[idx] = msg.ip
		m.tabs = m.tabs.InsertTabAt(idx, panes.Tab{Title: msg.title, Content: msg.room})

		var cmds []tea.Cmd
		if m.eventManager != nil {
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

func (m appModel) applyEvent(event sonos.Event) (tea.Model, tea.Cmd) {
	for i, tab := range m.tabs.Items {
		room, ok := tab.Content.(roomModel)
		if !ok || !isSameSpeaker(room.device, event.Speaker) {
			continue
		}

		updated, cmd := room.Update(nowplaying.TrackMsg{Track: trackFromState(room.device.State())})
		m.tabs.Items[i].Content = updated

		if i != m.tabs.Active {
			cmd = nil
		}
		return m, tea.Batch(cmd, waitForEvent(room.device))
	}

	slog.Warn("event for a speaker with no room — dropping", "speaker", event.Speaker)
	return m, nil
}

func isSameSpeaker(device *sonos.Device, speaker sonos.Speaker) bool {
	if device.USN != "" && speaker.USN != "" {
		return device.USN == speaker.USN
	}
	return device.IP == speaker.IP
}

func (m appModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var content string
	switch {
	case len(m.tabs.Items) == 0 && m.discoveryDone:
		content = "No Sonos speakers found on the local network."
	case len(m.tabs.Items) == 0:
		content = "Discovering Sonos speakers..."
	default:
		content = m.tabs.View()
	}

	return tea.NewView(content)
}

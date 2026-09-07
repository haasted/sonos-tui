package main

import (
	"context"
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
	ctx           context.Context // root context; Bubbletea commands carry none of their own
	tabs          panes.Tabs
	ips           []string
	eventManager  *sonos.EventManager
	width         int
	height        int
	discoveryDone bool
	fromCache     bool // tabs were seeded from the cache this run
	cached        []sonos.Speaker
	quitting      bool

	verticalSpaceChecked bool
}

// newAppModel builds the model, seeding tabs straight from the cache so the
// first frame already shows the speakers from last run. Nothing here touches
// the network; verification happens behind the rendered tabs.
func newAppModel(ctx context.Context, events *sonos.EventManager, cached []sonos.Speaker, selectedUSN string) appModel {
	tabs := panes.NewTabs(nil)
	tabs.Position = panes.Bottom
	tabs.HideBorder = true

	m := appModel{ctx: ctx, tabs: tabs, eventManager: events, fromCache: len(cached) > 0}
	for _, speaker := range cached {
		m = m.insertSpeaker(speaker.IP, speaker.DisplayName(), newSpeakerModel(sonos.NewDevice(speaker)))
	}
	for i, ip := range m.ips {
		if usnFor(cached, ip) == selectedUSN && selectedUSN != "" {
			m.tabs.Active = i
		}
	}
	m.cached = cached
	return m
}

// usnFor finds the cached USN for an IP, for restoring the selected tab.
func usnFor(speakers []sonos.Speaker, ip string) string {
	for _, s := range speakers {
		if s.IP == ip {
			return s.USN
		}
	}
	return ""
}

// insertSpeaker adds a tab in IP order, keeping m.ips as the sorted index.
func (m appModel) insertSpeaker(ip, title string, speaker speakerModel) appModel {
	idx := sort.SearchStrings(m.ips, ip)
	m.ips = append(m.ips, "")
	copy(m.ips[idx+1:], m.ips[idx:])
	m.ips[idx] = ip
	m.tabs = m.tabs.InsertTabAt(idx, panes.Tab{Title: title, Content: speaker})
	return m
}

// removeSpeaker drops a tab whose speaker turned out not to be there.
func (m appModel) removeSpeaker(ip string) appModel {
	idx := sort.SearchStrings(m.ips, ip)
	if idx >= len(m.ips) || m.ips[idx] != ip {
		return m
	}
	m.ips = append(m.ips[:idx], m.ips[idx+1:]...)
	m.tabs = m.tabs.RemoveTabAt(idx)
	return m
}

// hasSpeaker reports whether a tab for this IP already exists, so discovery
// does not duplicate one the cache already produced.
func (m appModel) hasSpeaker(ip string) bool {
	idx := sort.SearchStrings(m.ips, ip)
	return idx < len(m.ips) && m.ips[idx] == ip
}

// Init starts the tabs and, when seeded from the cache, verifies every cached
// speaker concurrently behind the already-rendered tabs.
func (m appModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.tabs.Init()}
	for i, speaker := range m.cached {
		device, ok := m.deviceFor(speaker.IP)
		if !ok {
			continue
		}
		cmds = append(cmds, verifyCachedSpeaker(m.ctx, m.eventManager, m.cached[i], device))
	}
	return tea.Batch(cmds...)
}

// deviceFor finds the Device backing a tab, so verification can refresh the
// same instance the tab is already rendering.
func (m appModel) deviceFor(ip string) (*sonos.Device, bool) {
	idx := sort.SearchStrings(m.ips, ip)
	if idx >= len(m.ips) || m.ips[idx] != ip {
		return nil, false
	}
	speaker, ok := m.tabs.Items[idx].Content.(speakerModel)
	if !ok {
		return nil, false
	}
	return speaker.device, true
}

// speakerSnapshot returns the speakers still on screen and the USN of the
// selected one, for writing to the cache on exit.
func (m appModel) speakerSnapshot() ([]sonos.Speaker, string) {
	var speakers []sonos.Speaker
	var selected string
	for i, tab := range m.tabs.Items {
		speaker, ok := tab.Content.(speakerModel)
		if !ok {
			continue
		}
		speakers = append(speakers, speaker.device.Speaker)
		if i == m.tabs.Active {
			selected = speaker.device.USN
		}
	}
	return speakers, selected
}

// Update handles the message, then re-reads playing state for every tab.
// Syncing centrally rather than at each site that changes playback means a new
// path cannot forget to do it and leave a tab's glyph lying.
func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.update(msg)
	updated, ok := model.(appModel)
	if !ok {
		return model, cmd
	}
	return updated.syncPlaying(), cmd
}

// syncPlaying refreshes each tab's playing flag from the device behind it.
// Devices update their own state optimistically on local commands and on
// arriving events, so this reflects both without waiting for a round trip.
func (m appModel) syncPlaying() appModel {
	for i, tab := range m.tabs.Items {
		speaker, ok := tab.Content.(speakerModel)
		if !ok {
			continue
		}
		m.tabs.Items[i].Playing = speaker.device.State().Playing()
	}
	return m
}

func (m appModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if m.hasSpeaker(msg.speaker.IP) {
			return m, nil // the cache already produced this tab
		}
		return m, loadSpeaker(m.ctx, m.eventManager, msg.speaker)
	case speakerVerifiedMsg:
		return m.applyVerified(msg)
	case speakerGoneMsg:
		slog.Info("cached speaker did not answer — dropping its tab", "ip", msg.ip)
		title := m.titleFor(msg.ip)
		return m.removeSpeaker(msg.ip), notify(panes.NoticeWarn, title+" did not answer — removed")
	case speakerReadyMsg:
		if m.hasSpeaker(msg.ip) {
			return m, nil // already on screen from the cache
		}
		wasEmpty := len(m.tabs.Items) == 0
		m = m.insertSpeaker(msg.ip, msg.title, msg.speaker)

		var cmds []tea.Cmd
		if m.eventManager != nil {
			cmds = append(cmds, waitForEvent(msg.speaker.device))
		}
		if wasEmpty {
			cmds = append(cmds, msg.speaker.Init())
		}
		if msg.degraded != "" {
			cmds = append(cmds, notify(panes.NoticeWarn, msg.degraded))
		}
		return m, tea.Batch(cmds...)
	case speakerMsg:
		return m.applySpeakerMsg(msg)
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

// applyVerified redraws a cached tab once its speaker has answered and its
// state has been refreshed, and picks up a room renamed since the cache was
// written.
func (m appModel) applyVerified(msg speakerVerifiedMsg) (tea.Model, tea.Cmd) {
	for i, tab := range m.tabs.Items {
		speaker, ok := tab.Content.(speakerModel)
		if !ok || speaker.device != msg.speaker {
			continue
		}
		updated, cmd := speaker.Update(nowplaying.TrackMsg{Track: trackFromState(msg.speaker.State())})
		m.tabs.Items[i].Content = updated
		if msg.title != "" {
			m.tabs.Items[i].Title = msg.title
		}
		if i != m.tabs.Active {
			cmd = nil
		}
		var degraded tea.Cmd
		if msg.degraded != "" {
			degraded = notify(panes.NoticeWarn, msg.degraded)
		}
		return m, tea.Batch(cmd, degraded, waitForEvent(msg.speaker))
	}
	return m, nil
}

// notify raises an app-wide notice. panes.Tabs handles NoticeMsg itself, so
// this only has to reach the fallthrough at the end of update.
func notify(level panes.NoticeLevel, text string) tea.Cmd {
	return func() tea.Msg {
		return panes.NoticeMsg{Level: level, Text: text}
	}
}

// titleFor is the room name shown on a tab, for naming it in a notice after
// the tab itself is gone.
func (m appModel) titleFor(ip string) string {
	idx := sort.SearchStrings(m.ips, ip)
	if idx >= len(m.ips) || m.ips[idx] != ip {
		return ip
	}
	return m.tabs.Items[idx].Title
}

// applySpeakerMsg hands a command result to the speaker that issued it. The
// tab it belongs to may no longer be the active one: every command is a round
// trip, and the user is free to move on while it is in flight.
func (m appModel) applySpeakerMsg(msg speakerMsg) (tea.Model, tea.Cmd) {
	for i, tab := range m.tabs.Items {
		speaker, ok := tab.Content.(speakerModel)
		if !ok || speaker.device != msg.device {
			continue
		}

		updated, cmd := speaker.Update(msg.msg)
		m.tabs.Items[i].Content = updated
		if i != m.tabs.Active {
			cmd = nil // background rooms animate nothing, as in applyEvent
		}
		return m, cmd
	}

	slog.Warn("command result for a speaker with no tab — dropping", "speaker", msg.device.Speaker)
	return m, nil
}

func (m appModel) applyEvent(event sonos.Event) (tea.Model, tea.Cmd) {
	for i, tab := range m.tabs.Items {
		speaker, ok := tab.Content.(speakerModel)
		if !ok || !isSameSpeaker(speaker.device, event.Speaker) {
			continue
		}

		updated, cmd := speaker.Update(eventMsg(event, speaker.device.State()))
		m.tabs.Items[i].Content = updated

		if i != m.tabs.Active {
			cmd = nil
		}
		return m, tea.Batch(cmd, waitForEvent(speaker.device))
	}

	slog.Warn("event for a speaker with no tab — dropping", "speaker", event.Speaker)
	return m, nil
}

// eventMsg narrows a volume event to a VolumeMsg. A RenderingControl event
// carries no playback position, so sending a TrackMsg would reset the clock to
// whatever stale RelTime the last AVTransport event left behind.
func eventMsg(event sonos.Event, state sonos.DeviceState) tea.Msg {
	if event.Kind == sonos.EventRenderingControl {
		return nowplaying.VolumeMsg{Volume: state.Volume, Muted: state.Muted}
	}
	return nowplaying.TrackMsg{Track: trackFromState(state)}
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

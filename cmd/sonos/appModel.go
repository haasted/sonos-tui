package main

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/nowplaying"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

var _ tea.Model = &appModel{}

// playerHeightRows is the height the inline frame always occupies. Holding it
// constant is what keeps the terminal from scrolling: a frame that grows after
// its first render scrolls to make room, and that scroll is unrecoverable —
// tea.Println reserves space ABOVE the frame, which does nothing for growth
// below it, so the startup reservation and the growth both scrolled and the
// surplus was the band of blank lines above the player.
const playerHeightRows = 6

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
	help          panes.Modal
	queue         panes.Modal

	// queueGen guards the queue arrows' breath. A new generation on every open
	// and every close, so a tick from a closed list drives nothing.
	queueGen int

	// hintDone records that the help hint has had its moment — shown, or
	// retired by a keypress or by a real notice claiming the rule first.
	hintDone bool

	// frameRows is the tallest the inline frame has ever needed to be. It
	// only ever grows: see growFrame.
	frameRows int
}

// newAppModel builds the model, seeding tabs straight from the cache so the
// first frame already shows the speakers from last run. Nothing here touches
// the network; verification happens behind the rendered tabs.
func newAppModel(ctx context.Context, events *sonos.EventManager, cached []sonos.Speaker, selectedUSN string) appModel {
	tabs := panes.NewTabs(nil)
	tabs.Position = panes.Bottom
	tabs.HideBorder = true

	help := panes.NewModal(helpContent{})
	help.Mode = panes.ModalHover

	queue := panes.NewModal(queueContent{})
	queue.Mode = panes.ModalHover

	m := appModel{ctx: ctx, tabs: tabs, eventManager: events, fromCache: len(cached) > 0,
		help: help, queue: queue}
	for _, speaker := range cached {
		m = m.insertSpeaker(speaker.IP, speaker.RoomName(), newSpeakerModel(sonos.NewDevice(speaker)))
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
	cmds := []tea.Cmd{m.tabs.Init(), hintTimer()}
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
	return updated.syncPlaying().growFrame(), cmd
}

// growFrame records the tallest the frame has needed to be, and never lets it
// shrink. Giving rows back strands them: the inline renderer works from a
// relative cursor, so once the terminal has scrolled to make room it cannot
// reach above its own frame to erase what it no longer owns. Opening and
// closing a modal would leave a copy of it behind, once per cycle.
func (m appModel) growFrame() appModel {
	needed := playerHeightRows
	for _, modal := range []panes.Modal{m.help, m.queue} {
		if !modal.IsOpen() {
			continue
		}
		if _, height := modal.BoxSize(); height > needed {
			needed = height
		}
	}
	if needed > m.frameRows {
		m.frameRows = needed
	}
	return m
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
		m.width = msg.Width
		m.height = msg.Height
		m.tabs.Width = msg.Width
		m.help = m.help.SetSize(msg.Width, msg.Height)
		m.queue = m.queue.SetSize(msg.Width, msg.Height)
		m = m.resizeQueue()
	case tea.KeyMsg:
		m.hintDone = true // the user has found the keyboard; nothing left to hint

		// Quitting is global, and comes before any modal. The help popup
		// documents q as quit, so a modal that swallows it makes the popup
		// tell the reader something untrue.
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Sequence(tea.ClearScreen, tea.Quit)
		}

		// An open modal owns the keyboard: without this the arrow keys would
		// still be changing volume behind it, unseen.
		if m.help.IsOpen() {
			if msg.String() == "?" {
				m.help = m.help.Close()
				return m, nil
			}
			help, cmd := m.help.Update(msg)
			m.help = help
			return m, cmd
		}

		// The queue modal claims the keyboard the same way, and takes the
		// arrow keys with it: up and down scroll the list, not the volume.
		if m.queue.IsOpen() {
			queue, cmd := m.queue.Update(msg)
			m.queue = queue
			if !m.queue.IsOpen() {
				m.queueGen = nextQueueGen() // the breath dies with the list
				return m, cmd
			}
			if content, ok := m.queue.Content.(queueContent); ok && content.bumping() {
				return m.restartQueueChain()
			}
			return m, cmd
		}

		switch msg.String() {
		case "?":
			m.help = m.help.Open()
			return m, nil
		case "enter":
			return m.openQueue()
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
	case hintTickMsg:
		return m.applyHintTick()
	case queueLoadedMsg:
		return m.applyQueueLoaded(msg)
	case queueTickMsg:
		return m.applyQueueTick(msg)
	case panes.NoticeMsg:
		// A degraded speaker surfaces in the same seconds the hint would, and a
		// hint replacing it would bury the one message that mattered. Falls
		// through to Tabs, which owns the rule.
		m.hintDone = true
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
	return notifyEntrance(level, text, panes.NoticeAppear)
}

// notifyEntrance is notify for a notice that has to be noticed rather than
// merely shown — one the user did not ask for and is not expecting.
func notifyEntrance(level panes.NoticeLevel, text string, entrance panes.NoticeEntrance) tea.Cmd {
	return func() tea.Msg {
		return panes.NoticeMsg{Level: level, Text: text, Entrance: entrance}
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

	// Pad first, so the modal composites onto a backdrop already at the frame's
	// full height and the frame itself never changes size while it is open.
	// The border light is read fresh each frame: it depends on when the reader
	// last pressed against an end, which changes under the modal rather than
	// on it.
	queue := m.queue
	if content, ok := queue.Content.(queueContent); ok {
		queue.Light = content.borderLight()
	}

	framed := padToHeight(content, max(m.frameRows, playerHeightRows))
	return tea.NewView(queue.ViewOver(m.help.ViewOver(framed)))
}

// padToHeight grows content to exactly rows lines. It never truncates: losing
// the bottom of the player would be worse than a frame one row too tall, and
// TestView_NeverExceedsThePlayerHeight guards the case that would cause it.
func padToHeight(content string, rows int) string {
	if missing := rows - lipgloss.Height(content); missing > 0 {
		return content + strings.Repeat("\n", missing)
	}
	return content
}

// resizeQueue tells the queue list how many rows it may draw. The modal clips
// anything past that, so a list which does not know cannot scroll correctly.
func (m appModel) resizeQueue() appModel {
	content, ok := m.queue.Content.(queueContent)
	if !ok {
		return m
	}
	content.rows = m.queue.ContentRows()
	content.width = m.queue.ContentColumns()
	m.queue.Content = content
	return m
}

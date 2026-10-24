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
	grouping      panes.Modal

	sized bool // a size has arrived; until then fits refuses nothing

	// queueGen guards the queue arrows' breath. A new generation on every open
	// and every close, so a tick from a closed list drives nothing.
	queueGen int

	// hintDone records that the help hint has had its moment — shown, or
	// retired by a keypress or by a real notice claiming the rule first.
	hintDone bool

	// frameRows is the tallest the inline frame has ever needed to be. It
	// only ever grows: see growFrame.
	frameRows int

	// household is how the speakers around here are grouped. Read once from
	// each system when it is found, and kept current from then on by the
	// events every speaker sends when its own grouping changes.
	household sonos.Household

	// askedTopology records the speakers already asked, by address, and
	// askingTopology that one of them has not answered yet.
	//
	// One at a time on purpose. Speakers arrive one after another and the
	// first answer accounts for every other speaker in its household, so
	// asking each as it appears would send a burst of reads to learn one
	// thing. Waiting means the second is only asked if the first left it out.
	askedTopology  map[string]bool
	askingTopology bool

	// sweepGen is the chain of frames driving the light round an opening
	// modal's border, matched against arriving ticks so only one runs.
	sweepGen int

	queueHintGen int // the queue hint's chain: its wait, then its frames
}

// newAppModel builds the model, seeding tabs straight from the cache so the
// first frame already shows the speakers from last run. Nothing here touches
// the network; verification happens behind the rendered tabs.
func newAppModel(ctx context.Context, events *sonos.EventManager, cached []sonos.Speaker, selectedUSN string) appModel {
	tabs := panes.NewTabs(nil)
	tabs.Position = panes.Bottom
	tabs.HideBorder = true

	help := panes.NewModal(helpContent{table: playerHelp})
	help.Mode = panes.ModalHover

	// Fixed rather than hovering: the queue changes under the reader as they
	// remove and move tracks, and a box that resized and re-centred on every
	// keypress was one they could not aim at.
	queue := panes.NewModal(queueContent{})
	queue.Mode = panes.ModalFixed

	// Hovering, not fixed: it lists a handful of rooms and has nothing that
	// changes under the reader, so it can fit itself to what it holds.
	grouping := panes.NewModal(groupContent{})
	grouping.Mode = panes.ModalHover

	m := appModel{ctx: ctx, tabs: tabs, eventManager: events, fromCache: len(cached) > 0,
		help: help, queue: queue, grouping: grouping}
	for _, speaker := range cached {
		m = m.insertSpeaker(speaker.IP, speaker.RoomName(), newSpeakerModel(ctx, sonos.NewDevice(speaker)))
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
	// Asked at once rather than waiting for discovery to finish: any speaker
	// answers for the whole household, so a cached one can say how things
	// are grouped while the rest are still being looked for.
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

// activeSpeaker is the speaker whose tab is on screen.
func (m appModel) activeSpeaker() (speakerModel, bool) {
	if m.tabs.Active >= len(m.tabs.Items) {
		return speakerModel{}, false
	}
	speaker, ok := m.tabs.Items[m.tabs.Active].Content.(speakerModel)
	return speaker, ok
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
	for _, modal := range []panes.Modal{m.help, m.queue, m.grouping} {
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
		// Resolved before anything reads it: every line below, and every
		// command this tab issues, asks the speaker that owns the playback.
		speaker.leader = m.leaderFor(speaker)
		m.tabs.Items[i].Content = speaker

		m.tabs.Items[i].Playing = speaker.shown().Playing()
		m.tabs.Items[i].Following = m.following(speaker)

		// The rooms this one drives are named under its own tab, so the
		// dimming elsewhere in the row has somewhere to be explained. The
		// tab row itself is untouched: every speaker keeps its place in it.
		m.tabs.Items[i].Notes = m.controls(speaker)
	}
	return m
}

// leaderFor is the speaker driving this one, or nil when it drives itself.
// Nil too when the coordinator is not a speaker we hold -- it may belong to
// another Sonos system -- because a command still has to go somewhere, and
// the speaker in view is where it went before any of this existed.
func (m appModel) leaderFor(speaker speakerModel) *sonos.Device {
	id := speaker.device.Speaker.ID()

	coordinator, ok := m.household.CoordinatorOf(id)
	if !ok || coordinator == id {
		return nil
	}
	device, found := m.deviceWithID(coordinator)
	if !found {
		return nil
	}
	return device
}

// controls is the rooms a speaker is playing through to, and nothing unless
// it coordinates a group of more than itself.
func (m appModel) controls(speaker speakerModel) []string {
	driven := m.household.Drives(speaker.device.Speaker.ID())

	rooms := make([]string, 0, len(driven))
	for _, member := range driven {
		rooms = append(rooms, member.RoomName)
	}
	return rooms
}

// following reports a speaker another one is playing through. False for a
// speaker the topology does not mention: it may belong to another Sonos
// system on the same network, and unknown is not the same as standing alone.
func (m appModel) following(speaker speakerModel) bool {
	_, follows := m.household.Follows(speaker.device.Speaker.ID())
	return follows
}

// readUncovered asks a speaker that nothing has accounted for yet, and
// nothing at all once every speaker on screen belongs to a group we know
// about. One read answers for a whole household, so the second speaker of
// one is never asked; a speaker in ANOTHER household is, because no answer
// so far has mentioned it.
//
// Each speaker is asked at most once, which is what ends this: a speaker
// that answers with a topology leaving itself out would otherwise be asked
// again every time an answer arrived.
func (m appModel) readUncovered() (appModel, tea.Cmd) {
	if m.askingTopology {
		return m, nil // one is already on its way; its answer may cover this
	}

	for _, tab := range m.tabs.Items {
		speaker, ok := tab.Content.(speakerModel)
		if !ok || m.askedTopology[speaker.device.IP] {
			continue
		}
		if _, found := m.household.GroupOf(speaker.device.Speaker.ID()); found {
			continue
		}

		if m.askedTopology == nil {
			m.askedTopology = map[string]bool{}
		}
		m.askedTopology[speaker.device.IP] = true
		m.askingTopology = true

		device, ctx := speaker.device, m.ctx
		return m, func() tea.Msg {
			topology, err := device.Topology(ctx)
			return topologyLoadedMsg{topology: topology, err: err}
		}
	}
	return m, nil
}

func (m appModel) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, m.sized = msg.Width, msg.Height, true
		m.tabs.Width = msg.Width
		m.help = m.help.SetSize(msg.Width, msg.Height)
		m.queue = m.queue.SetSize(msg.Width, msg.Height)
		m.grouping = m.grouping.SetSize(msg.Width, msg.Height)
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

		// Nothing else while the UI is hidden: a key would act on a screen
		// the reader cannot see.
		if !m.fits() {
			return m, nil
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
		if m.grouping.IsOpen() {
			if msg.String() == "g" {
				m.grouping = m.grouping.Close()
				return m, nil
			}
			grouping, cmd := m.grouping.Update(msg)
			m.grouping = grouping
			return m, cmd
		}

		if m.queue.IsOpen() {
			m = m.retireQueueHint()
			if msg.String() == "?" {
				return m.openHelp(queueHelp)
			}
			queue, cmd := m.queue.Update(msg)
			m.queue = queue
			if !m.queue.IsOpen() {
				m.queueGen = nextQueueGen() // the breath dies with the list
				m.queueHintGen = nextQueueGen()
				return m, cmd
			}
			if content, ok := m.queue.Content.(queueContent); ok && content.bumping() {
				return m.restartQueueChain()
			}
			return m, cmd
		}

		switch msg.String() {
		case "g":
			return m.openGrouping()
		case "?":
			return m.openHelp(playerHelp)
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

		// One read for each speaker, each in its own command and so in its own
		// goroutine. They run behind the tab that is already on screen.
		cmds := []tea.Cmd{refreshQueue(m.ctx, msg.speaker.device)}
		if m.eventManager != nil {
			cmds = append(cmds, waitForEvent(msg.speaker.device))
		}
		if wasEmpty {
			cmds = append(cmds, msg.speaker.Init())
		}

		// Asked as it arrives rather than when the search gives up: a
		// speaker can say how its household is grouped the moment it
		// answers, and only one per household is ever asked.
		var ask tea.Cmd
		m, ask = m.readUncovered()
		if ask != nil {
			cmds = append(cmds, ask)
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
	case groupChangeMsg:
		return m.applyGroupChange(msg)
	case topologyLoadedMsg:
		m.askingTopology = false
		if msg.err != nil {
			slog.Warn("reading how the speakers are grouped", "err", msg.err)
		} else {
			m.household = m.household.Apply(msg.topology)
		}

		// An answer may leave speakers it never mentioned -- a second Sonos
		// system on the same network has a topology of its own. Ask one of
		// them, and go round again until nobody is left out.
		next, ask := m.readUncovered()
		return next, ask
	case hintTickMsg:
		return m.applyHintTick()
	case queueLoadedMsg:
		return m.applyQueueLoaded(msg)
	case queueRemoveMsg:
		return m.applyQueueRemove(msg)
	case queueMovesMsg:
		return m.applyQueueMoves(msg)
	case queuePlayFromMsg:
		return m.applyQueuePlayFrom(msg)
	case queueRemoveCommitMsg:
		return m.applyQueueRemoveCommit(msg)
	case queueMoveCommitMsg:
		return m.applyQueueMoveCommit(msg)
	case queueTickMsg:
		return m.applyQueueTick(msg)
	case modalSweepDueMsg:
		return m.applyModalSweepDue(msg)
	case modalSweepTickMsg:
		return m.applyModalSweepTick(msg)
	case queueHintDueMsg:
		return m.applyQueueHintDue(msg)
	case queueHintTickMsg:
		return m.applyQueueHintTick(msg)
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

		// A cached speaker never arrives as ready, so this is where one gets
		// its turn to say how its household is grouped.
		m, ask := m.readUncovered()
		return m, tea.Batch(cmd, degraded, ask,
			refreshQueue(m.ctx, msg.speaker), waitForEvent(msg.speaker))
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
	// The grouping belongs to the household rather than to the speaker that
	// reported it, and every speaker in one reports the same change. Folded
	// in before the tabs are reached: the arrangement is what the tabs are
	// about to be drawn from.
	if event.Kind == sonos.EventZoneGroupTopology {
		m.household = m.household.Apply(event.Topology)

		// Re-derived now rather than after this message, because the loop
		// below asks each tab who drives it and the answer has just changed.
		m = m.syncPlaying()
	}

	// A coordinator's event reaches more tabs than its own: every speaker it
	// drives is playing that track too, and says so in its own pane.
	var cmds []tea.Cmd
	reported := false

	for i, tab := range m.tabs.Items {
		speaker, ok := tab.Content.(speakerModel)
		if !ok {
			continue
		}

		own := isSameSpeaker(speaker.device, event.Speaker)
		mirrors := speaker.leader != nil && isSameSpeaker(speaker.leader, event.Speaker)
		if !own && !mirrors {
			continue
		}

		// An event the panel has nothing to say about still has to re-arm the
		// wait below, so it reaches here and simply does not touch the tab.
		// shown() rather than the reporter's state: a follower draws what the
		// group is playing, with its own volume.
		if panel := eventMsg(event, speaker.shown()); panel != nil {
			updated, panelCmd := speaker.Update(panel)
			m.tabs.Items[i].Content = updated
			if i == m.tabs.Active {
				cmds = append(cmds, panelCmd)
			}
		}

		if !own {
			continue
		}
		reported = true

		// Read the queue of whichever speaker changed, on screen or not: the
		// point is that every queue is current before anyone asks for it.
		if _, queue := event.Directory.QueueUpdate(); queue {
			cmds = append(cmds, refreshQueue(m.ctx, speaker.device))
		}
		cmds = append(cmds, waitForEvent(speaker.device))
	}

	if !reported {
		slog.Warn("event for a speaker with no tab — dropping", "speaker", event.Speaker)
	}
	return m, tea.Batch(cmds...)
}

// eventMsg turns one speaker event into a message for the panel. Each kind is
// named rather than defaulted: a RenderingControl or ContentDirectory event
// carries no playback position, so a TrackMsg would reset the clock to
// whatever stale RelTime the last AVTransport event left behind.
func eventMsg(event sonos.Event, state sonos.DeviceState) tea.Msg {
	switch event.Kind {
	case sonos.EventRenderingControl:
		return nowplaying.VolumeMsg{Volume: state.Volume, Muted: state.Muted}
	case sonos.EventContentDirectory:
		// The queue changed. Nothing on the panel shows a queue, so the panel
		// has nothing to say; applyEvent reads the queue instead.
		return nil
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
	if !m.fits() {
		return tea.NewView(m.tooSmallView())
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
	// Help draws over the queue, not under it. The two can be open together
	// now: "?" inside the queue explains the queue.
	return tea.NewView(m.help.ViewOver(m.grouping.ViewOver(queue.ViewOver(framed))))
}

// applyGroupChange moves one speaker in or out of the group the room on
// screen belongs to, the moment the reader asks.
//
// Joining points the speaker at the COORDINATOR rather than at the room on
// screen, which are the same thing until that room is itself being driven:
// a group has one owner, and a speaker pointed at a follower would be
// following something that is not leading.
func (m appModel) applyGroupChange(msg groupChangeMsg) (tea.Model, tea.Cmd) {
	speaker, ok := m.activeSpeaker()
	if !ok {
		return m, nil
	}
	target, found := m.deviceWithID(msg.id)
	if !found {
		return m, notify(panes.NoticeWarn, msg.room+" is not here any more")
	}

	ctx := m.ctx
	if !msg.join {
		return m, func() tea.Msg {
			if err := target.LeaveGroup(ctx); err != nil {
				slog.Warn("taking a speaker out of a group", "speaker", msg.room, "err", err)
				return panes.NoticeMsg{Level: panes.NoticeError, Text: "could not release " + msg.room + ": " + faultText(err)}
			}
			return panes.NoticeMsg{Level: panes.NoticeInfo, Text: "released " + msg.room, Confirmation: true}
		}
	}

	id := speaker.device.Speaker.ID()
	coordinator := id
	if owner, follows := m.household.Follows(id); follows {
		coordinator = owner
	}

	return m, func() tea.Msg {
		if err := target.JoinGroup(ctx, coordinator); err != nil {
			slog.Warn("adding a speaker to a group", "speaker", msg.room, "err", err)
			return panes.NoticeMsg{Level: panes.NoticeError, Text: "could not add " + msg.room + ": " + faultText(err)}
		}
		return panes.NoticeMsg{Level: panes.NoticeInfo, Text: "added " + msg.room, Confirmation: true}
	}
}

// deviceWithID finds a speaker by the id the topology calls it.
func (m appModel) deviceWithID(id string) (*sonos.Device, bool) {
	for _, tab := range m.tabs.Items {
		speaker, ok := tab.Content.(speakerModel)
		if ok && speaker.device.Speaker.ID() == id {
			return speaker.device, true
		}
	}
	return nil, false
}

// openGrouping lists the speakers the room on screen can be grouped with.
// Built when it opens rather than read as it draws: what the reader sees is
// what was true when they asked, not whatever has arrived since.
func (m appModel) openGrouping() (tea.Model, tea.Cmd) {
	speaker, ok := m.activeSpeaker()
	if !ok {
		return m, nil
	}

	// The name on the tab rather than the device's own: they agree in the
	// ordinary case, and where they do not the reader has been reading the
	// tab.
	room := m.tabs.Items[m.tabs.Active].Title
	id := speaker.device.Speaker.ID()

	// The speakers already playing with this one start in the group: the
	// list is the group as it would be, not a set of changes to it.
	playingWith := map[string]bool{}
	for _, member := range m.household.Drives(id) {
		playingWith[member.ID] = true
	}
	if coordinator, follows := m.household.Follows(id); follows {
		for _, member := range m.household.Drives(coordinator) {
			playingWith[member.ID] = member.ID != id
		}
		playingWith[coordinator] = true
	}

	m.grouping.Content = groupingFor(room, m.household.Companions(id), playingWith)
	m.grouping = m.grouping.Open()
	return m, nil
}

// openHelp shows the popup with the table for wherever the reader is. The
// modal is reused rather than duplicated, so only one of them is ever open.
func (m appModel) openHelp(table helpTable) (appModel, tea.Cmd) {
	m.help.Content = helpContent{table: table}
	m.help = m.help.Open()
	return m.restartSweepChain()
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

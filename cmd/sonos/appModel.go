package main

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/store"
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

// cachedState is where a cached speaker stands while the program finds out
// whether it is still there.
type cachedState int

const (
	cachedChecking cachedState = iota // its check has not answered, and no search has found it
	cachedFailed                      // its check failed; it stays until the searches end without it
	cachedLoading                     // it answered, by its check or a search, and is loading or loaded
)

type appModel struct {
	ctx           context.Context // root context; Bubbletea commands carry none of their own
	tabs          panes.Tabs
	ids           []string // the RINCON id behind each tab, sorted, which is the tab order
	eventManager  *sonos.EventManager
	width         int
	height        int
	discoveryDone bool
	discovery     discoveryDoneMsg // how discovery went, for the screen shown when it found nothing
	fromCache     bool             // tabs were seeded from the cache this run
	cached        []sonos.Speaker
	cachedStates  map[string]cachedState // by RINCON id, for every cached speaker
	found         map[string]bool        // RINCON ids a search has found this run
	quitting      bool
	help          panes.Modal
	queue         panes.Modal
	favourites    panes.Modal
	grouping      panes.Modal
	marking       panes.Modal  // asks whether a mark in the queue is for the track or its playlist or album
	db            *store.Store // the database; nil only in tests

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

	favouritesGen     int // the favourites list's repaint chain, as queueGen
	favouritesHintGen int // the favourites hint's chain, as queueHintGen
}

// newAppModel builds the model, seeding tabs straight from the cache so the
// first frame already shows the speakers from last run, with the tab whose
// RINCON id is selected chosen. Nothing here touches the network.
func newAppModel(ctx context.Context, events *sonos.EventManager, cached []sonos.Speaker, selected string) appModel {
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

	// Fixed, as the queue is, and in its own colour so the two are not
	// mistaken for each other.
	favourites := panes.NewModal(favouritesContent{})
	favourites.Mode = panes.ModalFixed
	favourites.Color = favouritesBorderColor()

	// Hovering, not fixed: it lists a handful of rooms and has nothing that
	// changes under the reader, so it can fit itself to what it holds.
	grouping := panes.NewModal(groupContent{})
	grouping.Mode = panes.ModalHover

	// Hovering over the queue, and in the favourites' colour: it is about
	// marking one.
	marking := panes.NewModal(markContent{})
	marking.Mode = panes.ModalHover
	marking.Color = favouritesBorderColor()

	m := appModel{ctx: ctx, tabs: tabs, eventManager: events, fromCache: len(cached) > 0,
		help: help, queue: queue, favourites: favourites, grouping: grouping, marking: marking,
		cachedStates: map[string]cachedState{}, found: map[string]bool{}}
	for _, speaker := range cached {
		m = m.insertSpeaker(speakerKey(speaker), speaker.RoomName(), newSpeakerModel(ctx, sonos.NewDevice(speaker)))
		m.cachedStates[speakerKey(speaker)] = cachedChecking
		if selected != "" && speakerKey(speaker) == selected {
			m.tabs.Active = m.tabIndex(speakerKey(speaker))
		}
	}
	m.cached = cached
	return m
}

// speakerKey is what a tab is keyed and sorted by: the RINCON id, which no
// rename or new address changes. The address stands in only for a speaker
// that gave no id, which no search delivers.
func speakerKey(speaker sonos.Speaker) string {
	if id := speaker.ID(); id != "" {
		return id
	}
	return speaker.IP
}

// insertSpeaker adds a tab in id order, keeping m.ids as the sorted index.
// The selection stays on the speaker it was on, wherever the new tab goes.
func (m appModel) insertSpeaker(id, title string, speaker speakerModel) appModel {
	idx := sort.SearchStrings(m.ids, id)
	m.ids = append(m.ids, "")
	copy(m.ids[idx+1:], m.ids[idx:])
	m.ids[idx] = id
	m.tabs = m.tabs.InsertTabAt(idx, panes.Tab{Title: title, Content: speaker})
	return m
}

// removeSpeaker drops a tab whose speaker turned out not to be there.
func (m appModel) removeSpeaker(id string) appModel {
	idx := m.tabIndex(id)
	if idx < 0 {
		return m
	}
	m.ids = append(m.ids[:idx], m.ids[idx+1:]...)
	m.tabs = m.tabs.RemoveTabAt(idx)
	return m
}

// tabIndex is the position of the speaker's tab, or -1 when it has none.
func (m appModel) tabIndex(id string) int {
	idx := sort.SearchStrings(m.ids, id)
	if idx >= len(m.ids) || m.ids[idx] != id {
		return -1
	}
	return idx
}

// hasSpeaker reports whether this speaker already has a tab, so discovery
// does not duplicate one the cache already produced.
func (m appModel) hasSpeaker(id string) bool { return m.tabIndex(id) >= 0 }

// Init starts the tabs and, when seeded from the cache, verifies every cached
// speaker concurrently behind the already-rendered tabs.
func (m appModel) Init() tea.Cmd {
	// Asked at once rather than waiting for discovery to finish: any speaker
	// answers for the whole household, so a cached one can say how things
	// are grouped while the rest are still being looked for.
	cmds := []tea.Cmd{m.tabs.Init(), hintTimer()}
	for _, speaker := range m.cached {
		cmds = append(cmds, checkCachedSpeaker(m.ctx, speaker))
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

// deviceFor finds the Device backing a tab, so loading a cached speaker
// refreshes the same instance the tab is already rendering.
func (m appModel) deviceFor(id string) (*sonos.Device, bool) {
	idx := m.tabIndex(id)
	if idx < 0 {
		return nil, false
	}
	speaker, ok := m.tabs.Items[idx].Content.(speakerModel)
	if !ok {
		return nil, false
	}
	return speaker.device, true
}

// speakerSnapshot returns the speakers still on screen and the RINCON id of
// the selected one, for the next run to start from.
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
			selected = speakerKey(speaker.device.Speaker)
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
	for _, modal := range []panes.Modal{m.help, m.queue, m.favourites, m.grouping, m.marking} {
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
		m.grouping = m.grouping.SetSize(msg.Width, msg.Height)
		m.marking = m.marking.SetSize(msg.Width, msg.Height)
		m = m.resizeLists(msg.Width, msg.Height)
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

		// The chooser hovers over the open queue, so it takes the keys first.
		if m.marking.IsOpen() {
			marking, cmd := m.marking.Update(msg)
			m.marking = marking
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

		if _, open := m.openList(); open {
			return m.updateList(msg)
		}

		switch msg.String() {
		case "g":
			return m.openGrouping()
		case "?":
			return m.openHelp(playerHelp)
		case "enter":
			return m.openQueue()
		case "f":
			return m.openFavourites()
		}
	case speakerFoundMsg:
		return m.applyFound(msg.speaker)
	case cachedCheckedMsg:
		return m.applyChecked(msg)
	case speakerVerifiedMsg:
		return m.applyVerified(msg)
	case speakerReadyMsg:
		wasEmpty := len(m.tabs.Items) == 0
		var replaced bool
		m, replaced = m.placeSpeaker(msg)

		// One read for each speaker, each in its own command and so in its own
		// goroutine. They run behind the tab that is already on screen.
		cmds := []tea.Cmd{refreshQueue(m.ctx, msg.speaker.device)}
		if m.eventManager != nil {
			cmds = append(cmds, waitForEvent(msg.speaker.device))
		}
		if wasEmpty || (replaced && m.ids[m.tabs.Active] == msg.id) {
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
		m.discoveryDone, m.discovery = true, msg
		return m.dropUnanswered()
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
	case favouritesLoadedMsg:
		return m.applyFavouritesLoaded(msg)
	case favouriteKindChosenMsg:
		return m.applyFavouriteKindChosen(msg)
	case favouriteActionMsg:
		return m.applyFavouriteAction(msg)
	case favouriteFadedMsg:
		return m.applyFavouriteFaded(msg)
	case favouriteRemoveMsg:
		return m.applyFavouriteRemove(msg)
	case favouriteChooseMsg:
		return m.applyFavouriteChoose(msg)
	case favouriteChosenMsg:
		return m.applyFavouriteChosen(msg)
	case favouriteMarkMsg:
		return m.applyFavouriteMark(msg)
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
	case listTickMsg:
		return m.applyListTick(msg)
	case modalSweepDueMsg:
		return m.applyModalSweepDue(msg)
	case modalSweepTickMsg:
		return m.applyModalSweepTick(msg)
	case tea.MouseWheelMsg:
		return m.applyListWheel(msg)
	case listHintDueMsg:
		return m.applyListHintDue(msg)
	case listHintTickMsg:
		return m.applyListHintTick(msg)
	case panes.NoticeMsg:
		// A degraded speaker surfaces in the same seconds the hint would, and a
		// hint replacing it would bury the one message that mattered. Falls
		// through to Tabs, which owns the rule, unless a list modal covers it.
		m.hintDone = true
		if _, open := m.openList(); open {
			return m.noticeOnList(msg)
		}
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
func (m appModel) titleFor(id string) string {
	idx := m.tabIndex(id)
	if idx < 0 {
		return id
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

		// A new track leaves the queue as it is, so no read follows to move
		// the open queue's marker. The event moves it instead.
		if event.Kind == sonos.EventAVTransport && i == m.tabs.Active {
			m = m.markPlaying(speaker)
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
		content = noSpeakersText(m.discovery)
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
	framed := padToHeight(content, max(m.frameRows, playerHeightRows))
	for _, list := range m.litLists() {
		framed = list.ViewOver(framed)
	}
	// Help draws over a list, not under it. The two can be open together:
	// "?" inside the queue explains the queue.
	view := tea.NewView(m.help.ViewOver(m.marking.ViewOver(m.grouping.ViewOver(framed))))
	if m.listTakesTheWheel() {
		view.MouseMode = tea.MouseModeCellMotion
	}
	return view
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

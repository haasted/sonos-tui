// How the application drives the queue: opening the modal, reading a speaker's
// queue into it, and sending what the reader did. queue.go holds the list
// itself, which fetches nothing; listmodal.go the chains every list shares.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

const (
	// queueMoveQuiet is how long the keys must be still before a move is sent.
	// Every press restarts it, so the send lands after the reader has stopped
	// rather than part-way through what they are doing -- a fixed window from
	// the first press committed mid-run and the settle that followed then ate
	// the presses that came next.
	//
	// Generous against an observed press interval of about 150ms. Tune here.
	queueMoveQuiet = 700 * time.Millisecond

	// queueRemoveQuiet is the same wait before removals are sent, and for the
	// same reason: a run of presses becomes one call. Its own constant, since
	// nothing says the two keys want the same feel once both are tuned.
	queueRemoveQuiet = 700 * time.Millisecond

	// queueWriteSettle is how long a change sent to the speaker waits for the
	// queue it produced.
	// A read takes about a tenth of a second; this is long enough for a slow one
	// and short enough that a dropped event does not strand the reader.
	queueWriteSettle = 2 * time.Second

	queueRemoved = "removed"
	queueMoved   = "moved"

	// queueRemovedSeveral and queueRemovedPartly carry a count, which one
	// track does not need: a reader who removed several cannot otherwise tell
	// that the whole selection went.
	queueRemovedSeveral = "removed %d tracks"
	queueRemovedPartly  = "removed %d of %d tracks"
	queueMovedSeveral   = "moved %d tracks"
	queueMovedPartly    = "moved %d of %d tracks"

	// queueMoveAbandoned is said when someone else changed the queue while a
	// move was still being made here. The positions the move was built from no
	// longer name the same tracks, so it cannot be sent.
	queueMoveAbandoned = "the queue changed — move dropped"

	// queueRemoveAbandoned answers the same case for a removal that had not
	// been sent: the rows come back rather than the wrong tracks going.
	queueRemoveAbandoned = "the queue changed — removal dropped"

	// queueRemoveFailed says nothing about why. A speaker refusing because the
	// queue moved under the reader and a speaker that could not be reached look
	// the same from here, and which error means which is not yet established.
	queueRemoveFailed        = "could not remove the track"
	queueRemoveFailedSeveral = "could not remove the tracks"
	queueMoveFailed          = "could not move the track"
	queueMoveFailedSeveral   = "could not move the tracks"

	// queuePlayNextNoTrack answers a station: its queue can be seen but nothing
	// in it is playing, so there is no current track to be "next" of.
	queuePlayNextNoTrack = "nothing is playing from the queue"
	queueAlreadyPlaying  = "already playing"

	// queueAlreadyLast answers a track sent to the bottom that is already
	// there, the way queueAlreadyNext answers one already next.
	queueAlreadyLast = "already last"
	queueAlreadyNext = "already next"

	queuePlayedFrom     = "playing"
	queuePlayFromFailed = "could not play the track"
)

// groupChangeMsg asks for one speaker to join the group the room on screen
// belongs to, or to leave whatever group it is in. Sent as the reader picks
// the row, not when the modal closes.
type groupChangeMsg struct {
	id   string
	room string
	join bool
}

// topologyLoadedMsg carries how the household is grouped, or why it could
// not be read. One read answers for every speaker.
type topologyLoadedMsg struct {
	topology sonos.Topology
	err      error
}

// queueLoadedMsg carries a speaker's queue back to the modal that asked for it.
type queueLoadedMsg struct {
	device  *sonos.Device
	room    string
	items   []sonos.QueueItem
	playing int // 1-based position now playing, or 0 when the queue is not the source
	err     error
}

// queueRemoveMsg asks for one track to leave the queue on screen. It carries
// the 1-based position rather than the row, since that is what the speaker
// addresses a track by.
type queueRemoveMsg struct{ positions []int }

// queueMove is one track going from one place in the queue to another, both
// 1-based. A run of them is worked out together, each against the queue the
// one before it left -- see playNextMoves.
type queueMove struct{ from, to int }

// queueMovesMsg asks for a run of moves, worked out already, in the order
// given. Play next and send to bottom both send it: what differs is where
// the tracks were aimed, which is settled before it is built. Sent and
// answered in one step, unlike a shifted move: there is no window of
// presses to close first.
type queueMovesMsg struct{ moves []queueMove }

// queuePlayFromMsg asks for one track to start playing now, wherever the
// speaker's attention was before.
type queuePlayFromMsg struct{ position int }

// queueMoveCommitMsg closes a move window, sending whatever the reader ended
// up with. It carries the generation so a tick from an abandoned window cannot
// commit a move nobody is making any more.
type queueMoveCommitMsg struct{ gen int }

// queueRemoveCommitMsg closes a removal window, sending whatever the reader
// took off the list. It carries the generation, so a tick from a superseded
// window cannot send a removal twice.
type queueRemoveCommitMsg struct{ gen int }

// queueRemoveTick schedules the send of the removals identified by gen.
func queueRemoveTick(gen int) tea.Cmd {
	return tea.Tick(queueRemoveQuiet, func(time.Time) tea.Msg {
		return queueRemoveCommitMsg{gen: gen}
	})
}

// queueMoveTick schedules the send of the move identified by gen. A press
// arriving first takes a newer generation, and this one is then ignored on
// arrival -- which is what makes the wait trailing rather than fixed.
func queueMoveTick(gen int) tea.Cmd {
	return tea.Tick(queueMoveQuiet, func(time.Time) tea.Msg {
		return queueMoveCommitMsg{gen: gen}
	})
}

// modalSweepTickMsg is one frame of the light running round a modal's
// border. Its own chain rather than the queue's: the help popup sweeps too
// and has no list to breathe.
type modalSweepTickMsg struct{ gen int }

// modalSweepDueMsg says it is time for another light. The wait between them
// is random, so the effect is something the reader catches rather than
// something they can predict from having opened a box.
type modalSweepDueMsg struct{ gen int }

// modalSweepRest is how long a modal waits between lights: never less than
// the first, never more than the two together. Random inside that, so no two
// waits look alike and none of them look scheduled.
const (
	modalSweepRest   = 6 * time.Second
	modalSweepSpread = 14 * time.Second
)

func modalSweepDue(gen int) tea.Cmd {
	wait := modalSweepRest + time.Duration(rand.Int64N(int64(modalSweepSpread)))
	return tea.Tick(wait, func(time.Time) tea.Msg {
		return modalSweepDueMsg{gen: gen}
	})
}

func modalSweepTick(gen int) tea.Cmd {
	return tea.Tick(listPulseInterval, func(time.Time) tea.Msg {
		return modalSweepTickMsg{gen: gen}
	})
}

var queueGenerations atomic.Int64

// nextGen returns a generation unique across the process, for any chain.
func nextGen() int { return int(queueGenerations.Add(1)) }

// openQueue asks the selected speaker for its queue. Every open fetches again:
// the queue changes underneath us, and a stale list is worse than a short wait.
func (m appModel) openQueue() (tea.Model, tea.Cmd) {
	speaker, ok := m.activeSpeaker()
	if !ok {
		return m, nil
	}

	state := speaker.shown()
	m.queue = m.queue.Open()
	content := queueContent{
		listView: listView[sonos.QueueItem]{
			items:    state.Queue,
			rows:     m.queue.ContentRows(),
			width:    m.queue.ContentColumns(),
			openedAt: time.Now(),
		},
		room:    speaker.device.RoomName(),
		source:  state.QueueSource,
		playing: playingPosition(state),
		loading: !state.QueueKnown,
	}

	// Opening lands on the playing track, as if the reader had pressed c right
	// after -- a no-op here if the queue is not known yet, since jumpToPlaying
	// finds nothing in an empty list. applyQueueLoaded does the same once that
	// first read arrives.
	m.queue.Content = content.jumpToPlaying()

	// Nothing to fetch in the ordinary case: the queue was read at startup and
	// has been kept current by events since. A read here is the fallback for a
	// speaker whose first read has not landed yet.
	m, sweeping := m.restartSweepChain()
	hint := restartListHint(m.queueSlot())
	if state.QueueKnown {
		return m, tea.Batch(sweeping, hint)
	}
	return m, tea.Batch(sweeping, hint, refreshQueue(m.ctx, speaker.transport()))
}

// refreshQueue reads a speaker's queue into its Device and reports that it
// landed. Every speaker gets one at startup, and one more whenever an event
// says its queue changed.
func refreshQueue(ctx context.Context, device *sonos.Device) tea.Cmd {
	return func() tea.Msg {
		if err := device.RefreshQueue(ctx); err != nil {
			slog.Warn("reading a speaker's queue", "speaker", device.RoomName(), "err", err)
			return queueLoadedMsg{device: device, room: device.RoomName(), err: err}
		}

		state := device.State()
		return queueLoadedMsg{
			device:  device,
			room:    device.RoomName(),
			items:   state.Queue,
			playing: playingPosition(state),
		}
	}
}

// applyQueueRemove takes a track out of the speaker on screen. It updates no
// list: the removal raises a ContentDirectory event, which the program already
// answers by reading the queue again, and a second answer here would disagree
// with that one while it was in flight.
// applyQueueRemoveCommit closes a removal window and tells the speaker. The
// rows left the list when the key was pressed; this is the part that was
// waiting to see whether more were coming.
func (m appModel) applyQueueRemoveCommit(msg queueRemoveCommitMsg) (tea.Model, tea.Cmd) {
	content, ok := m.queue.Content.(queueContent)
	if !ok || !m.queue.IsOpen() || msg.gen != content.removeGen {
		return m, nil
	}

	positions, pending := content.pendingRemove()
	if !pending {
		m.queue.Content = content.clearRemoving()
		return m, nil
	}
	content = content.closeRemoveWindow()

	// The speaker is about to renumber, so nothing else may name a position
	// until its queue comes back.
	m.queue.Content = content.awaitQueue()
	return m.applyQueueRemove(queueRemoveMsg{positions: positions})
}

func (m appModel) applyQueueRemove(msg queueRemoveMsg) (tea.Model, tea.Cmd) {
	speaker, ok := m.activeSpeaker()
	if !ok {
		return m, nil
	}

	device := speaker.transport()
	ctx := m.ctx
	asked := len(msg.positions)
	return m, func() tea.Msg {
		removed, err := device.RemoveQueueTracks(ctx, msg.positions)
		if err != nil {
			slog.Warn("removing tracks from the queue", "speaker", device.RoomName(),
				"positions", msg.positions, "removed", removed, "err", err)
			return panes.NoticeMsg{Level: panes.NoticeError, Text: removeFailedNotice(removed, asked)}
		}
		return panes.NoticeMsg{Level: panes.NoticeInfo, Text: removedNotice(removed), Confirmation: true}
	}
}

// removedNotice says one thing for the whole command rather than one thing
// per track. A reader removing one knows which one, so the count is only
// worth saying when there were several.
func removedNotice(removed int) string {
	if removed == 1 {
		return queueRemoved
	}
	return fmt.Sprintf(queueRemovedSeveral, removed)
}

// removeFailedNotice tells a removal that did nothing from one that stopped
// part way. The second leaves the queue in a state the reader did not ask
// for, so the count is the whole point of the message.
func removeFailedNotice(removed, asked int) string {
	switch {
	case removed > 0:
		return fmt.Sprintf(queueRemovedPartly, removed, asked)
	case asked > 1:
		return queueRemoveFailedSeveral
	default:
		return queueRemoveFailed
	}
}

// applyQueueMoves sends the move queueContent.playNext decided on. Unlike a
// shifted move there is no window to close first: the positions were computed
// once, against the queue on screen, and are sent as they are.
func (m appModel) applyQueueMoves(msg queueMovesMsg) (tea.Model, tea.Cmd) {
	speaker, ok := m.activeSpeaker()
	if !ok {
		return m, nil
	}

	device := speaker.transport()
	ctx := m.ctx
	moves := make([]sonos.QueueMove, 0, len(msg.moves))
	for _, move := range msg.moves {
		moves = append(moves, sonos.QueueMove{From: move.from, To: move.to})
	}

	asked := len(moves)
	return m, func() tea.Msg {
		moved, err := device.MoveQueueTracks(ctx, moves)
		if err != nil {
			slog.Warn("moving tracks to play next", "speaker", device.RoomName(),
				"moves", asked, "moved", moved, "err", err)
			return panes.NoticeMsg{Level: panes.NoticeError, Text: moveFailedNotice(moved, asked)}
		}
		return panes.NoticeMsg{Level: panes.NoticeInfo, Text: movedNotice(moved), Confirmation: true}
	}
}

// movedNotice and moveFailedNotice read as the removal ones do: a count only
// where there were several, and a partial failure saying how many really
// went, since the queue is then in an order nobody asked for.
func movedNotice(moved int) string {
	if moved == 1 {
		return queueMoved
	}
	return fmt.Sprintf(queueMovedSeveral, moved)
}

func moveFailedNotice(moved, asked int) string {
	switch {
	case moved > 0:
		return fmt.Sprintf(queueMovedPartly, moved, asked)
	case asked > 1:
		return queueMoveFailedSeveral
	default:
		return queueMoveFailed
	}
}

// applyQueuePlayFrom jumps playback to the track queueContent.playFromCursor
// named. The modal closes on the way: the reader asked to listen to
// something, and the now-playing view behind it is what answers that.
func (m appModel) applyQueuePlayFrom(msg queuePlayFromMsg) (tea.Model, tea.Cmd) {
	m.queue = m.queue.Close()

	speaker, ok := m.activeSpeaker()
	if !ok {
		return m, nil
	}

	device := speaker.transport()
	ctx := m.ctx
	return m, func() tea.Msg {
		if err := device.PlayTrack(ctx, msg.position); err != nil {
			slog.Warn("playing a track from the queue",
				"speaker", device.RoomName(), "position", msg.position, "err", err)
			return panes.NoticeMsg{Level: panes.NoticeError, Text: queuePlayFromFailed}
		}
		return panes.NoticeMsg{Level: panes.NoticeInfo, Text: queuePlayedFrom, Confirmation: true}
	}
}

// applyQueueMoveCommit sends the move the reader has settled on. The rows on
// screen have been theirs since the window opened; this is where the speaker is
// told, once, however many presses it took.
func (m appModel) applyQueueMoveCommit(msg queueMoveCommitMsg) (tea.Model, tea.Cmd) {
	content, ok := m.queue.Content.(queueContent)
	if !ok || !m.queue.IsOpen() || msg.gen != content.moveGen {
		return m, nil
	}

	from, to, pending := content.pendingMove()
	content = content.clearMove()
	if !pending || from == to {
		m.queue.Content = content
		return m, nil
	}

	// Remember which track went, so the queue that comes back can put the
	// cursor on it again rather than on whatever row it left behind.
	m.queue.Content = content.settleMove(landing{at: to, uri: content.items[content.cursor].URI})

	speaker, found := m.activeSpeaker()
	if !found {
		return m, nil
	}

	device := speaker.transport()
	ctx := m.ctx
	return m, func() tea.Msg {
		if err := device.MoveQueueTrack(ctx, from, to); err != nil {
			slog.Warn("moving a track in the queue",
				"speaker", device.RoomName(), "from", from, "to", to, "err", err)
			return panes.NoticeMsg{Level: panes.NoticeError, Text: queueMoveFailed}
		}
		return panes.NoticeMsg{Level: panes.NoticeInfo, Text: queueMoved, Confirmation: true}
	}
}

// playingPosition is the queue position now playing, or zero when nothing in
// the queue is. A station plays straight past the queue without touching it,
// and DeviceState.Track then counts the stream — so highlighting item 1 would
// point confidently at the wrong track.
func playingPosition(state sonos.DeviceState) int {
	if state.Source.Kind() != sonos.SourceTrack {
		return 0
	}
	return state.Track
}

// applyQueueLoaded fills the open modal in. Reads arrive for every speaker,
// most of them for a modal that is closed or showing another room, so a result
// that does not match what is on screen is dropped rather than drawn under the
// wrong room name.
func (m appModel) applyQueueLoaded(msg queueLoadedMsg) (tea.Model, tea.Cmd) {
	content, ok := m.queue.Content.(queueContent)
	if !ok || !m.queue.IsOpen() {
		return m, nil
	}
	speaker, found := m.activeSpeaker()
	if !found || speaker.transport() != msg.device {
		return m, nil
	}

	// True only for the read that answers an open on a queue not yet known.
	// Captured before loading is cleared below, since that is the one signal
	// telling this load apart from an ordinary reload of one already on screen.
	firstLoad := content.loading

	// A queue arriving while the reader was still moving a track. The move was
	// built from positions in the OLD queue, so it can no longer be sent: what
	// sat at those positions may not be there now.
	//
	// A move the reader has already sent is a different thing, and is handled
	// below: shift refuses to start another until its queue arrives, so a move
	// cannot be both pending and waiting to land.
	var abandoned tea.Cmd
	awaiting := content.awaitingQueue()
	if _, _, pending := content.pendingMove(); pending {
		content = content.clearMove()
		abandoned = notify(panes.NoticeWarn, queueMoveAbandoned)
	}

	// The same for removals the reader has made but nobody has sent. Their
	// positions were read from the queue this one replaces, so they can no
	// longer be trusted to name the same tracks.
	if _, pending := content.pendingRemove(); pending {
		abandoned = notify(panes.NoticeWarn, queueRemoveAbandoned)
	}
	// Whether they were sent or abandoned, this queue is the speaker's own
	// answer and the rows in it are the ones that are staying.
	content = content.clearRemoving()

	content.room = msg.room
	content.source = speaker.shown().QueueSource
	content.items, content.all = msg.items, nil
	content = content.narrow() // a filter on screen still means what it says
	content.playing = msg.playing
	content.err = msg.err
	content.loading = false
	content.rows = m.queue.ContentRows()
	content.width = m.queue.ContentColumns()

	// A queue that changed under the reader can be shorter than the row they
	// were on. Keep the place, but not past the end of the list, and let the
	// window settle around wherever the cursor ends up.
	content.cursor = min(content.cursor, content.lastRow())
	content = content.follow()

	// The reader was watching a track, not a row. Now that the speaker has
	// said where it put it, the cursor goes back onto it for a committed
	// shift; play next leaves the cursor alone and only flashes the track.
	// Neither applies to a first load: nothing was pending before the queue
	// even existed on screen, so that instead lands on what is playing, as if
	// the reader had pressed c right after opening.
	switch {
	case awaiting && content.settlingFollow:
		content = content.followSettled()
	case awaiting && len(content.settling) > 0:
		content = content.highlightSettled()
	case awaiting:
		content = content.clearAwaiting() // a removal: nothing left to look for
	case firstLoad:
		content = content.jumpToPlaying()
	}

	m.queue.Content = content

	// The breath starts once there is an arrow to breathe, or a flash to fade.
	// A queue that fits and has nothing landing draws neither, and a chain for
	// it would repaint a still picture.
	if !content.overflows() && !content.highlighting() {
		return m, abandoned
	}
	m.queueGen = nextGen()
	return m, tea.Batch(abandoned, listTick(m.queueGen))
}

// applyModalSweepDue sends the next light round, and does nothing at all if
// every modal has closed in the meantime.
func (m appModel) applyModalSweepDue(msg modalSweepDueMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.sweepGen {
		return m, nil
	}
	if _, list := m.openList(); !m.help.IsOpen() && !list {
		return m, nil
	}

	m.help = m.help.StartSweep()
	for _, slot := range m.listSlots() {
		*slot.modal = slot.modal.StartSweep()
	}
	return m, modalSweepTick(msg.gen)
}

// applyModalSweepTick keeps the light moving, and lets the chain die
// the moment the last modal has finished with it.
func (m appModel) applyModalSweepTick(msg modalSweepTickMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.sweepGen {
		return m, nil
	}
	if !m.help.Sweeping() && !m.listSweeping() {
		return m, modalSweepDue(msg.gen) // rest, then go round again
	}
	return m, modalSweepTick(msg.gen)
}

// restartSweepChain begins a fresh wait for the next light and abandons any
// running one, so exactly one is alive however fast modals are opened.
//
// A wait rather than a light: a box that flashed the moment it appeared
// would teach the reader to expect it, and the point is to be caught sight
// of rather than watched for.
func (m appModel) restartSweepChain() (appModel, tea.Cmd) {
	m.sweepGen = nextGen()
	return m, modalSweepDue(m.sweepGen)
}

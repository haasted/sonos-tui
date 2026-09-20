// How the application drives the queue: opening the modal, reading a speaker's
// queue into it, and the repaint chain behind the arrows. queue.go holds the
// list itself, which fetches nothing and knows nothing of appModel.
package main

import (
	"log/slog"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

const (
	queueRemoved = "removed"

	// queueRemoveFailed says nothing about why. A speaker refusing because the
	// queue moved under the reader and a speaker that could not be reached look
	// the same from here, and which error means which is not yet established.
	queueRemoveFailed = "could not remove the track"
)

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
type queueRemoveMsg struct{ position int }

// queueTickMsg is one frame of the arrows' breath. The generation dies with the
// modal, so a chain cannot outlive the list that began it.
type queueTickMsg struct{ gen int }

var queueGenerations atomic.Int64

// nextQueueGen returns a generation unique across the process.
func nextQueueGen() int { return int(queueGenerations.Add(1)) }

func queueTick(gen int) tea.Cmd {
	return tea.Tick(queuePulseInterval, func(time.Time) tea.Msg {
		return queueTickMsg{gen: gen}
	})
}

// openQueue asks the selected speaker for its queue. Every open fetches again:
// the queue changes underneath us, and a stale list is worse than a short wait.
func (m appModel) openQueue() (tea.Model, tea.Cmd) {
	speaker, ok := m.activeSpeaker()
	if !ok {
		return m, nil
	}

	state := speaker.device.State()
	m.queue = m.queue.Open()
	m.queue.Content = queueContent{
		room:     speaker.device.RoomName(),
		items:    state.Queue,
		playing:  playingPosition(state),
		loading:  !state.QueueKnown,
		rows:     m.queue.ContentRows(),
		width:    m.queue.ContentColumns(),
		openedAt: time.Now(),
	}

	// Nothing to fetch in the ordinary case: the queue was read at startup and
	// has been kept current by events since. A read here is the fallback for a
	// speaker whose first read has not landed yet.
	if state.QueueKnown {
		return m, nil
	}
	return m, refreshQueue(speaker.device)
}

// refreshQueue reads a speaker's queue into its Device and reports that it
// landed. Every speaker gets one at startup, and one more whenever an event
// says its queue changed.
func refreshQueue(device *sonos.Device) tea.Cmd {
	return func() tea.Msg {
		if err := device.RefreshQueue(); err != nil {
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
func (m appModel) applyQueueRemove(msg queueRemoveMsg) (tea.Model, tea.Cmd) {
	speaker, ok := m.activeSpeaker()
	if !ok {
		return m, nil
	}

	device := speaker.device
	return m, func() tea.Msg {
		if err := device.RemoveQueueTrack(msg.position); err != nil {
			slog.Warn("removing a track from the queue",
				"speaker", device.RoomName(), "position", msg.position, "err", err)
			return panes.NoticeMsg{Level: panes.NoticeError, Text: queueRemoveFailed}
		}
		return panes.NoticeMsg{Level: panes.NoticeInfo, Text: queueRemoved, Confirmation: true}
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
	if speaker, found := m.activeSpeaker(); !found || speaker.device != msg.device {
		return m, nil
	}

	content.room = msg.room
	content.items = msg.items
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

	m.queue.Content = content

	// The breath starts only once there is an arrow to breathe. A queue that
	// fits draws none, and a chain for it would repaint a still picture.
	if !content.overflows() {
		return m, nil
	}
	m.queueGen = nextQueueGen()
	return m, queueTick(m.queueGen)
}

// applyQueueTick repaints the arrows. It re-arms only while the same modal is
// open and still has an arrow, so the chain cannot outlive either.
func (m appModel) applyQueueTick(msg queueTickMsg) (tea.Model, tea.Cmd) {
	content, ok := m.queue.Content.(queueContent)
	if msg.gen != m.queueGen || !m.queue.IsOpen() || !ok {
		return m, nil
	}

	// The arrows breathe while the queue overflows, and a bumped edge cools
	// whether it does or not. Neither is true: nothing on screen is moving.
	if !content.overflows() && !content.bumping() {
		return m, nil
	}
	return m, queueTick(msg.gen)
}

// restartQueueChain begins a fresh repaint chain and abandons any running one.
// A bump on a queue that fits has no chain to ride, and taking a generation
// each time is what keeps exactly one of them alive.
func (m appModel) restartQueueChain() (appModel, tea.Cmd) {
	m.queueGen = nextQueueGen()
	return m, queueTick(m.queueGen)
}

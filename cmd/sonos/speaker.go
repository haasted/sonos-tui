package main

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/nowplaying"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

var _ tea.Model = &speakerModel{}

const (
	// The playback commands, named so a refusal can say what was refused.
	actionPrevious = "previous track"
	actionNext     = "next track"
	actionPlay     = "play"
	actionPause    = "pause"

	confirmMuted   = "muted"
	confirmUnmuted = "unmuted"

	volumeDebounce = 150 * time.Millisecond // how long one volume window stays open before committing
	volumeStepUp   = 1                      // volume added per up press
	volumeStepDown = -1                     // volume removed per down press, negative by design

	// volumeUpMaxStep bounds the TOTAL a window may raise the volume, not the
	// number of presses, so it caps the rate a held key ramps at rather than
	// how finely a tap moves. Down is deliberately uncapped: it is the escape.
	volumeUpMaxStep = 6
)

// confirmations are what each playback command says once it has happened. Play
// and pause change form, where a skip reads the same as an outcome as it does
// as an order.
//
// Volume is absent on purpose. It writes its own level into the album row, and
// a held key commits every volumeDebounce, so a notice for each would flicker
// over a clearer answer.
var confirmations = map[string]string{
	actionPlay:     "playing",
	actionPause:    "paused",
	actionNext:     "next track",
	actionPrevious: "previous track",
}

var volumeGenerations atomic.Int64 // globally unique window ids, so one speaker's tick cannot fire another's

// nextVolumeGeneration returns a window id unique across every speaker in the process.
func nextVolumeGeneration() int { return int(volumeGenerations.Add(1)) }

// volumeCommitMsg closes a volume window, applying whatever accumulated in it.
type volumeCommitMsg struct {
	gen int // window this tick belongs to; ticks from closed windows are ignored
}

// volumeCommitTick schedules the close of the window identified by gen.
func volumeCommitTick(gen int) tea.Cmd {
	return tea.Tick(volumeDebounce, func(time.Time) tea.Msg {
		return volumeCommitMsg{gen: gen}
	})
}

// speakerModel is one speaker's tab: the device it controls, the now-playing
// widget, and the debounce bookkeeping for pending volume changes.
type speakerModel struct {
	ctx    context.Context  // root context; every command this tab issues carries it
	device *sonos.Device    // the speaker this tab controls
	leader *sonos.Device    // the speaker driving this one, or nil when it drives itself
	widget nowplaying.Model // renders the current track

	pendingVolume    int  // volume delta accumulated in the open window
	volumeGen        int  // id of the open window, matched against arriving ticks
	volumeWindowOpen bool // whether a commit tick is already scheduled
}

// newSpeakerModel builds a tab for device, seeded from its current state.
func newSpeakerModel(ctx context.Context, device *sonos.Device) speakerModel {
	return speakerModel{
		ctx:    ctx,
		device: device,
		widget: nowplaying.New(trackFromState(device.State())),
	}
}

// trackFromState projects device state onto the fields the widget renders.
//
// Streams need three substitutions. Broadcast radio sends no dc:title at all,
// so the title falls back to the station name; the station goes in the album
// slot, which for station playback is empty anyway; and the clock is replaced
// by a label whenever the duration is meaningless.
func trackFromState(state sonos.DeviceState) nowplaying.Track {
	title := state.Current.Title
	if title == "" {
		title = state.Source.Name
	}

	// A station's name outranks the album of a track you did not choose:
	// "Akylas" is why this is playing, "Party - Single" is incidental.
	album := state.Current.Album
	if station := state.Source.Name; station != "" && station != title {
		switch state.Source.Kind() {
		case sonos.SourceBroadcast, sonos.SourceStation:
			album = station
		default:
			if album == "" {
				album = station
			}
		}
	}

	// Broadcast radio sends no artist, so the row below the station is free
	// for the programme description — frequently the only thing a station
	// says about what is actually on.
	artist := state.Current.Artist
	if artist == "" {
		artist = state.Current.RadioShow
	}

	return nowplaying.Track{
		Title:       title,
		Artist:      artist,
		Album:       album,
		Elapsed:     sonos.ParseDuration(state.RelTime),
		Duration:    sonos.ParseDuration(state.Current.Duration),
		Playing:     state.Playing(),
		Muted:       state.Muted,
		Volume:      state.Volume,
		StreamLabel: streamLabel(state),
	}
}

// streamLabel names what kind of unbounded playback this is, or returns empty
// when the position readout still means something. A station playing a track
// with a real duration keeps its clock — being a stream and having a usable
// clock are separate questions.
func streamLabel(state sonos.DeviceState) string {
	if !state.Live() {
		return ""
	}
	switch state.Source.Kind() {
	case sonos.SourceBroadcast:
		return "live"
	case sonos.SourceStation:
		return "radio"
	case sonos.SourceLineIn:
		return "line-in"
	default:
		return "stream"
	}
}

// Init starts the widget's tickers and shows this speaker's volume, so
// arriving at a tab says how loud it is without having to nudge it.
func (m speakerModel) Init() tea.Cmd {
	return tea.Batch(m.widget.Init(), m.showVolume())
}

// showVolume surfaces the current level without changing it.
func (m speakerModel) showVolume() tea.Cmd {
	state := m.device.State()
	return func() tea.Msg {
		return m.to(nowplaying.VolumeMsg{Volume: state.Volume, Muted: state.Muted, Show: true})
	}
}

// Update applies volume commits and playback keys, forwarding anything else
// to the now-playing widget.
func (m speakerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if commit, ok := msg.(volumeCommitMsg); ok {
		if commit.gen != m.volumeGen {
			return m, nil
		}
		m.volumeWindowOpen = false
		if m.pendingVolume == 0 {
			return m, nil
		}
		delta := m.pendingVolume
		m.pendingVolume = 0
		return m, m.adjustVolume(delta)
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "p", "P", "k", "K":
			return m, m.togglePlayback()
		case "j", "J":
			return m, m.deviceCommand(actionPrevious, (*sonos.Device).Previous)
		case "l", "L":
			return m, m.deviceCommand(actionNext, (*sonos.Device).Next)
		case "m", "M":
			return m, m.toggleMute()
		case "up":
			return m.queueVolume(volumeStepUp)
		case "down":
			return m.queueVolume(volumeStepDown)
		}
	}

	widget, cmd := m.widget.Update(msg)
	m.widget = widget
	return m, cmd
}

// to addresses a command result to this speaker, so a slow round trip cannot
// deliver it to whichever tab the user switched to in the meantime.
func (m speakerModel) to(msg tea.Msg) tea.Msg {
	return speakerMsg{device: m.device, msg: msg}
}

// refused reports a failed command, saying what is true rather than what broke
// when the speaker is answering correctly.
func (m speakerModel) refused(name string, err error, state sonos.DeviceState) tea.Msg {
	because, ok := refusalText(name, err, state)
	if !ok {
		return m.failed(err)
	}

	// Info, not Error, and unnamed. The speaker did the right thing, and this
	// is the answer to a key just pressed on the tab in front of the reader —
	// unlike a fault, which can arrive from a room nobody is watching.
	return panes.NoticeMsg{Level: panes.NoticeInfo, Text: because}
}

// failed reports a command failure as an app-wide notice. Deliberately NOT
// addressed to this speaker's tab: the user may be looking at another room,
// and a failure they never see is the whole problem notices exist to fix.
func (m speakerModel) failed(err error) tea.Msg {
	return panes.NoticeMsg{
		Level: panes.NoticeError,
		Text:  m.device.RoomName() + ": " + faultText(err),
	}
}

// transport is the speaker that owns this one's playback: the coordinator of
// its group, or itself when it follows nobody. Everything about what is
// playing -- the queue included -- belongs to that speaker, because a
// follower has no queue and no transport of its own.
func (m speakerModel) transport() *sonos.Device {
	if m.leader != nil {
		return m.leader
	}
	return m.device
}

// shown is the state this pane draws: what the group is playing, with this
// speaker's own volume, which stays per-speaker however the group is made up.
func (m speakerModel) shown() sonos.DeviceState {
	state := m.transport().State()
	if m.leader == nil {
		return state
	}
	own := m.device.State()
	state.Volume, state.Muted = own.Volume, own.Muted
	return state
}

// queueVolume accumulates delta into the open window, opening one if none is.
// Only the upward direction is capped, so down stays a fast escape.
func (m speakerModel) queueVolume(delta int) (tea.Model, tea.Cmd) {
	m.pendingVolume += delta
	m.pendingVolume = min(m.pendingVolume, volumeUpMaxStep)

	if m.volumeWindowOpen {
		return m, nil // a tick is already pending; this press just joins that window
	}

	m.volumeWindowOpen = true
	m.volumeGen = nextVolumeGeneration()
	return m, volumeCommitTick(m.volumeGen)
}

// togglePlayback pauses a playing speaker and starts a stopped one.
func (m speakerModel) togglePlayback() tea.Cmd {
	return func() tea.Msg {
		if m.shown().Playing() {
			return m.runDeviceAction(actionPause, (*sonos.Device).Pause)
		}
		return m.runDeviceAction(actionPlay, (*sonos.Device).Play)
	}
}

// adjustVolume applies an accumulated delta. It reports volume only: a volume
// change cannot move playback, and a TrackMsg would reset the clock.
func (m speakerModel) adjustVolume(delta int) tea.Cmd {
	return func() tea.Msg {
		if _, err := m.device.AdjustVolume(m.ctx, delta); err != nil {
			slog.Warn("volume command failed", "speaker", m.device.Speaker, "delta", delta, "err", err)
			return m.failed(err)
		}
		state := m.device.State()
		return m.to(nowplaying.VolumeMsg{Volume: state.Volume, Muted: state.Muted})
	}
}

// toggleMute flips mute for the whole group, reporting volume only for the
// same reason as adjustVolume. Muting is an instruction about the music
// rather than about one room, so it does not stop at the speaker in view.
func (m speakerModel) toggleMute() tea.Cmd {
	return func() tea.Msg {
		muted, err := m.transport().ToggleGroupMute(m.ctx)
		if err != nil {
			slog.Warn("mute command failed", "speaker", m.device.Speaker, "err", err)
			return m.failed(err)
		}
		volume := m.to(nowplaying.VolumeMsg{Volume: m.device.State().Volume, Muted: muted})
		return withConfirmation(volume, muteConfirmation(muted))
	}
}

// muteConfirmation is what a mute toggle says once it has happened. The state
// is read back from the speaker rather than assumed from the key, so a toggle
// that landed the other way round says what is true.
func muteConfirmation(muted bool) string {
	if muted {
		return confirmMuted
	}
	return confirmUnmuted
}

// deviceCommand wraps a device action as a Bubbletea command. The name travels
// with it so a refusal can say which command the speaker would not run.
func (m speakerModel) deviceCommand(name string, action func(*sonos.Device, context.Context) error) tea.Cmd {
	return func() tea.Msg {
		return m.runDeviceAction(name, action)
	}
}

// runDeviceAction runs action, mapping failure to an error message and success
// to a refreshed track.
func (m speakerModel) runDeviceAction(name string, action func(*sonos.Device, context.Context) error) tea.Msg {
	if err := action(m.transport(), m.ctx); err != nil {
		state := m.transport().State()

		// The source and the position are what make a refusal readable later.
		// Without them every UPnP 701 in the log looks the same.
		slog.Warn("playback command failed",
			"speaker", m.device.Speaker, "action", name, "err", err,
			"source", state.Source.URI, "track", state.Track, "tracks", state.Tracks)

		return m.refused(name, err, state)
	}

	track := m.to(nowplaying.TrackMsg{Track: trackFromState(m.shown())})
	return withConfirmation(track, confirmations[name])
}

// withConfirmation sends the command's own result and a notice saying what
// happened. Two messages rather than one: the notice belongs to the whole
// program, where the result belongs to the speaker that issued the command.
//
// The runtime unpacks a batch before Update sees it, so neither message is
// wrapped in anything a handler has to know about. An empty said means the
// command answers for itself and needs no words.
func withConfirmation(result tea.Msg, said string) tea.Msg {
	if said == "" {
		return result
	}
	return tea.BatchMsg{
		func() tea.Msg { return result },
		func() tea.Msg {
			return panes.NoticeMsg{Level: panes.NoticeInfo, Text: said, Confirmation: true}
		},
	}
}

// View renders the now-playing widget.
func (m speakerModel) View() tea.View {
	return tea.NewView(m.widget.View())
}

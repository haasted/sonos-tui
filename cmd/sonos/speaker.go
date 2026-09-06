package main

import (
	"log/slog"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/nowplaying"
)

var _ tea.Model = &speakerModel{}

const (
	volumeDebounce  = 150 * time.Millisecond // how long one volume window stays open before committing
	volumeStepUp    = 2                      // volume added per up press
	volumeStepDown  = -2                     // volume removed per down press, negative by design
	volumeUpMaxStep = 6                      // upper bound on how much volume can be increased in a time window
)

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
	device *sonos.Device    // the speaker this tab controls
	widget nowplaying.Model // renders the current track

	pendingVolume    int  // volume delta accumulated in the open window
	volumeGen        int  // id of the open window, matched against arriving ticks
	volumeWindowOpen bool // whether a commit tick is already scheduled
}

// newSpeakerModel builds a tab for device, seeded from its current state.
func newSpeakerModel(device *sonos.Device) speakerModel {
	return speakerModel{
		device: device,
		widget: nowplaying.New(trackFromState(device.State())),
	}
}

// trackFromState projects device state onto the fields the widget renders.
func trackFromState(state sonos.DeviceState) nowplaying.Track {
	return nowplaying.Track{
		Title:    state.Current.Title,
		Artist:   state.Current.Artist,
		Album:    state.Current.Album,
		Elapsed:  sonos.ParseDuration(state.RelTime),
		Duration: sonos.ParseDuration(state.Current.Duration),
		Playing:  state.Playing(),
		Muted:    state.Muted,
		Volume:   state.Volume,
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
			return m, m.deviceCommand((*sonos.Device).Previous)
		case "l", "L":
			return m, m.deviceCommand((*sonos.Device).Next)
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
		if m.device.State().Playing() {
			return m.runDeviceAction((*sonos.Device).Pause)
		}
		return m.runDeviceAction((*sonos.Device).Play)
	}
}

// adjustVolume applies an accumulated delta. It reports volume only: a volume
// change cannot move playback, and a TrackMsg would reset the clock.
func (m speakerModel) adjustVolume(delta int) tea.Cmd {
	return func() tea.Msg {
		if _, err := m.device.AdjustVolume(delta); err != nil {
			slog.Warn("volume command failed", "speaker", m.device.Speaker, "delta", delta, "err", err)
			return m.to(nowplaying.ErrorMsg{Err: err})
		}
		state := m.device.State()
		return m.to(nowplaying.VolumeMsg{Volume: state.Volume, Muted: state.Muted})
	}
}

// toggleMute flips mute, reporting volume only for the same reason as adjustVolume.
func (m speakerModel) toggleMute() tea.Cmd {
	return func() tea.Msg {
		if err := m.device.ToggleMute(); err != nil {
			slog.Warn("mute command failed", "speaker", m.device.Speaker, "err", err)
			return m.to(nowplaying.ErrorMsg{Err: err})
		}
		state := m.device.State()
		return m.to(nowplaying.VolumeMsg{Volume: state.Volume, Muted: state.Muted})
	}
}

// deviceCommand wraps a device action as a Bubbletea command.
func (m speakerModel) deviceCommand(action func(*sonos.Device) error) tea.Cmd {
	return func() tea.Msg {
		return m.runDeviceAction(action)
	}
}

// runDeviceAction runs action, mapping failure to an error message and success
// to a refreshed track.
func (m speakerModel) runDeviceAction(action func(*sonos.Device) error) tea.Msg {
	if err := action(m.device); err != nil {
		slog.Warn("playback command failed", "speaker", m.device.Speaker, "err", err)
		return m.to(nowplaying.ErrorMsg{Err: err})
	}
	return m.to(nowplaying.TrackMsg{Track: trackFromState(m.device.State())})
}

// View renders the now-playing widget.
func (m speakerModel) View() tea.View {
	return tea.NewView(m.widget.View())
}

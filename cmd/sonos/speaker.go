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
	volumeUpMaxStep = 6                      // most one window may add, bounding a held up key
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
		Playing:  state.Transport.State == "PLAYING",
	}
}

// Init starts the now-playing widget's own tickers.
func (m speakerModel) Init() tea.Cmd {
	return m.widget.Init()
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
		return m, adjustVolume(m.device, delta)
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "p", "P", "k", "K":
			return m, togglePlayback(m.device)
		case "j", "J":
			return m, deviceCommand(m.device, "previous", (*sonos.Device).Previous)
		case "l", "L":
			return m, deviceCommand(m.device, "next", (*sonos.Device).Next)
		case "m", "M":
			return m, toggleMute(m.device)
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

// queueVolume accumulates delta into the open window, opening one if none is.
// Only the upward direction is capped, so down stays a fast escape.
func (m speakerModel) queueVolume(delta int) (tea.Model, tea.Cmd) {
	m.pendingVolume += delta
	if m.pendingVolume > volumeUpMaxStep {
		m.pendingVolume = volumeUpMaxStep
	}
	if m.volumeWindowOpen {
		return m, nil
	}
	m.volumeWindowOpen = true
	m.volumeGen = nextVolumeGeneration()
	return m, volumeCommitTick(m.volumeGen)
}

// togglePlayback pauses a playing speaker and starts a stopped one.
func togglePlayback(device *sonos.Device) tea.Cmd {
	return func() tea.Msg {
		if device.State().Transport.State == "PLAYING" {
			return runDeviceAction(device, "pause", (*sonos.Device).Pause)
		}
		return runDeviceAction(device, "play", (*sonos.Device).Play)
	}
}

// adjustVolume applies an accumulated delta, reporting nothing on success
// because no UI shows volume yet.
func adjustVolume(device *sonos.Device, delta int) tea.Cmd {
	return func() tea.Msg {
		if _, err := device.AdjustVolume(delta); err != nil {
			slog.Warn("volume command failed", "speaker", device.Speaker, "delta", delta, "err", err)
			return nowplaying.ErrorMsg{Err: err}
		}
		return nil
	}
}

// toggleMute flips the speaker's mute state, reporting only failures.
func toggleMute(device *sonos.Device) tea.Cmd {
	return func() tea.Msg {
		if err := device.ToggleMute(); err != nil {
			slog.Warn("mute command failed", "speaker", device.Speaker, "err", err)
			return nowplaying.ErrorMsg{Err: err}
		}
		return nil
	}
}

// deviceCommand wraps a device action as a Bubbletea command.
func deviceCommand(device *sonos.Device, name string, action func(*sonos.Device) error) tea.Cmd {
	return func() tea.Msg {
		return runDeviceAction(device, name, action)
	}
}

// runDeviceAction runs action, mapping failure to an error message and success
// to a refreshed track.
func runDeviceAction(device *sonos.Device, name string, action func(*sonos.Device) error) tea.Msg {
	if err := action(device); err != nil {
		slog.Warn("playback command failed", "speaker", device.Speaker, "action", name, "err", err)
		return nowplaying.ErrorMsg{Err: err}
	}
	return nowplaying.TrackMsg{Track: trackFromState(device.State())}
}

// View renders the now-playing widget.
func (m speakerModel) View() tea.View {
	return tea.NewView(m.widget.View())
}

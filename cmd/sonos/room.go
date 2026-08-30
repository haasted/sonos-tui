package main

import (
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/nowplaying"
)

type roomModel struct {
	device *sonos.Device
	widget nowplaying.Model
}

func newRoomModel(device *sonos.Device) roomModel {
	return roomModel{
		device: device,
		widget: nowplaying.New(trackFromState(device.State())),
	}
}

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

func (m roomModel) Init() tea.Cmd {
	return m.widget.Init()
}

func (m roomModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "p", "P", "k", "K":
			return m, togglePlayback(m.device)
		case "j", "J":
			return m, deviceCommand(m.device, "previous", (*sonos.Device).Previous)
		case "l", "L":
			return m, deviceCommand(m.device, "next", (*sonos.Device).Next)
		}
	}

	widget, cmd := m.widget.Update(msg)
	m.widget = widget
	return m, cmd
}

func togglePlayback(device *sonos.Device) tea.Cmd {
	return func() tea.Msg {
		if device.State().Transport.State == "PLAYING" {
			return runDeviceAction(device, "pause", (*sonos.Device).Pause)
		}
		return runDeviceAction(device, "play", (*sonos.Device).Play)
	}
}

func deviceCommand(device *sonos.Device, name string, action func(*sonos.Device) error) tea.Cmd {
	return func() tea.Msg {
		return runDeviceAction(device, name, action)
	}
}

func runDeviceAction(device *sonos.Device, name string, action func(*sonos.Device) error) tea.Msg {
	if err := action(device); err != nil {
		slog.Warn("playback command failed", "speaker", device.Speaker, "action", name, "err", err)
		return nowplaying.ErrorMsg{Err: err}
	}
	return nowplaying.TrackMsg{Track: trackFromState(device.State())}
}

func (m roomModel) View() string {
	return m.widget.View()
}

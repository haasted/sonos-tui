package main

import (
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
	widget, cmd := m.widget.Update(msg)
	m.widget = widget
	return m, cmd
}

func (m roomModel) View() string {
	return m.widget.View()
}

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

const discoverTimeout = 5 * time.Second

func main() {
	program := tea.NewProgram(newModel(), tea.WithAltScreen())

	go discoverSpeakers(program)

	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "bubbletea error:", err)
		os.Exit(1)
	}
}

func discoverSpeakers(program *tea.Program) {
	ctx, cancel := context.WithTimeout(context.Background(), discoverTimeout)
	defer cancel()

	ch, err := sonos.Discover(ctx)
	if err != nil {
		slog.Error("discovery failed", "err", err)
		program.Send(discoveryDoneMsg{})
		return
	}

	found := 0
	for speaker := range ch {
		found++
		program.Send(speakerFoundMsg{speaker: speaker})
	}
	program.Send(discoveryDoneMsg{found: found})
}

func loadRoom(speaker sonos.Speaker) tea.Cmd {
	return func() tea.Msg {
		device := sonos.NewDevice(speaker)
		if err := device.Refresh(); err != nil {
			slog.Warn("refreshing transport state", "speaker", speaker, "err", err)
		}
		if err := device.RefreshTrack(); err != nil {
			slog.Warn("refreshing track info", "speaker", speaker, "err", err)
		}
		return roomReadyMsg{title: speaker.DisplayName(), room: newRoomModel(device)}
	}
}

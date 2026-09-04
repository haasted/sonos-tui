package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/log"
	"github.com/haasted/sonos-sandbox/internal/sonos"
)

const (
	discoverTimeout = 5 * time.Second
	shutdownTimeout = 5 * time.Second
)

var logsFlag = flag.Bool("logs", true, "capture and print log output once the program exits; set to false to discard all logs")

func main() {
	ctx := context.Background()

	flag.Parse()

	dumpLogs := log.CaptureLogs(*logsFlag, slog.LevelDebug)
	defer dumpLogs()

	events, err := sonos.NewEventManager()
	if err != nil {
		slog.Error("starting event listener", "err", err)
		return
	}

	program := tea.NewProgram(newAppModel(events), tea.WithContext(ctx))

	go discoverSpeakers(program)

	_, runErr := program.Run()

	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	if err := events.Close(shutdownCtx); err != nil {
		slog.Warn("shutting down event listener", "err", err)
	}
	cancel()

	if runErr != nil {
		fmt.Fprintln(os.Stderr, "bubbletea error:", runErr)
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

func loadSpeaker(events *sonos.EventManager, speaker sonos.Speaker) tea.Cmd {
	return func() tea.Msg {
		slog.Debug("starting loadSpeaker", "speaker", speaker.DisplayName())
		defer slog.Debug("ending loadSpeaker", "speaker", speaker.DisplayName())

		device := sonos.NewDevice(speaker)
		if err := device.RefreshAll(); err != nil {
			slog.Warn("refreshing speaker state", "speaker", speaker, "err", err)
		}

		if err := events.Subscribe(context.Background(), device); err != nil {
			slog.Warn("subscribing to speaker events", "speaker", speaker, "err", err)
		}

		return speakerReadyMsg{title: speaker.DisplayName(), ip: speaker.IP, speaker: newSpeakerModel(device)}
	}
}

func waitForEvent(device *sonos.Device) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-device.Events()
		if !ok {
			return nil
		}
		return event
	}
}

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lmittmann/tint"

	"github.com/haasted/sonos-sandbox/internal/log"
	"github.com/haasted/sonos-sandbox/internal/sonos"
)

const discoverTimeout = 5 * time.Second

var logsFlag = flag.Bool("logs", true, "capture and print log output once the program exits; set to false to discard all logs")

func main() {
	flag.Parse()

	start := time.Now()
	var buf *log.Buffer
	var writer io.Writer = io.Discard
	if *logsFlag {
		buf = log.NewBuffer()
		writer = buf
	}
	slog.SetDefault(slog.New(log.NewElapsedHandler(
		tint.NewTextHandler(writer, &tint.Options{TimeFormat: time.TimeOnly}),
		start,
	)))

	program := tea.NewProgram(newModel(), tea.WithAltScreen())

	go discoverSpeakers(program)

	_, runErr := program.Run()

	if buf != nil {
		if err := buf.Flush(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "flushing logs:", err)
		}
	}

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

func loadRoom(speaker sonos.Speaker) tea.Cmd {
	return func() tea.Msg {
		device := sonos.NewDevice(speaker)
		if err := device.Refresh(); err != nil {
			slog.Warn("refreshing transport state", "speaker", speaker, "err", err)
		}
		if err := device.RefreshTrack(); err != nil {
			slog.Warn("refreshing track info", "speaker", speaker, "err", err)
		}
		return roomReadyMsg{title: speaker.DisplayName(), ip: speaker.IP, room: newRoomModel(device)}
	}
}

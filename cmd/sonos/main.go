package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/log"
	"github.com/haasted/sonos-sandbox/internal/sonos"
)

const (
	discoverTimeout = 5 * time.Second
	shutdownTimeout = 5 * time.Second
)

var logsFlag = flag.Bool("logs", false, "capture and print log output once the program exits; set to false to discard all logs")

func main() {
	flag.Parse()

	dumpLogs := log.CaptureLogs(*logsFlag, slog.LevelDebug)
	defer dumpLogs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := sonos.NewEventManager()
	if err != nil {
		slog.Error("starting event listener", "err", err)
		return
	}

	cached, selected := loadCachedSpeakers()

	model := newAppModel(ctx, events, cached, selected)
	program := tea.NewProgram(model, tea.WithContext(ctx))

	go discoverSpeakers(ctx, program)

	final, runErr := program.Run()
	saveCache(final)
	cancel() // stop discovery and anything else still tied to the root

	// Deliberately rooted at Background, not ctx: ctx is cancelled just above
	// and UNSUBSCRIBE still has to reach the speakers.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	if err := events.Close(shutdownCtx); err != nil {
		slog.Warn("shutting down event listener", "err", err)
	}
	shutdownCancel()

	if runErr != nil {
		fmt.Fprintln(os.Stderr, "bubbletea error:", runErr)
		os.Exit(1)
	}
}

// loadCachedSpeakers returns last run's speakers, or nothing when there is no
// cache or it was recorded on a different network. Every failure is silent:
// the cache is a shortcut, and discovery still runs regardless.
func loadCachedSpeakers() ([]sonos.Speaker, string) {
	path, pathErr := sonos.CachePath()
	if pathErr != nil {
		path = "<unknown>"
	}

	cache, ok := sonos.LoadCache()
	if !ok {
		slog.Info("no usable speaker cache — discovering from scratch", "path", path)
		return nil, ""
	}

	subnet, err := sonos.LocalSubnet()
	if err != nil {
		slog.Debug("cannot read local subnet — ignoring cache", "err", err)
		return nil, ""
	}
	if !cache.UsableOn(subnet) {
		slog.Info("cache was recorded on another network — ignoring it",
			"path", path, "cached", cache.Subnet, "current", subnet)
		return nil, ""
	}

	speakers := cache.SpeakerList()
	slog.Info("seeding tabs from cache",
		"path", path,
		"speakers", len(speakers),
		"subnet", cache.Subnet,
		"age", time.Since(cache.SavedAt).Truncate(time.Second))
	return speakers, cache.Selected
}

// saveCache records the speakers still on screen, and which one was selected,
// for the next run to start from.
func saveCache(final tea.Model) {
	app, ok := final.(appModel)
	if !ok {
		return
	}
	speakers, selected := app.speakerSnapshot()
	if len(speakers) == 0 {
		return
	}
	if err := sonos.SaveCache(speakers, selected); err != nil {
		slog.Warn("writing speaker cache", "err", err)
		return
	}
	if path, err := sonos.CachePath(); err == nil {
		slog.Info("wrote speaker cache", "path", path, "speakers", len(speakers), "selected", selected)
	}
}

// verifyCachedSpeaker proves a cached speaker is still there, refreshes it and
// subscribes to its events. All three run behind the already-rendered tab.
func verifyCachedSpeaker(ctx context.Context, events *sonos.EventManager, speaker sonos.Speaker, device *sonos.Device) tea.Cmd {
	return func() tea.Msg {
		if err := sonos.VerifySpeaker(ctx, speaker); err != nil {
			slog.Info("cached speaker failed verification", "speaker", speaker.IP, "err", err)
			return speakerGoneMsg{ip: speaker.IP}
		}

		var degraded []string
		if err := device.RefreshAll(ctx); err != nil {
			slog.Warn("refreshing cached speaker", "speaker", speaker.IP, "err", err)
			degraded = append(degraded, "state may be stale")
		}
		if err := events.Subscribe(ctx, device); err != nil {
			slog.Warn("subscribing to cached speaker", "speaker", speaker.IP, "err", err)
			degraded = append(degraded, "no live updates")
		}
		return speakerVerifiedMsg{
			ip: speaker.IP, title: speaker.RoomName(), speaker: device,
			degraded: degradedText(speaker.RoomName(), degraded),
		}
	}
}

// degradedText names the room and what about it is not working, or returns
// empty when nothing failed. A speaker that loads degraded looks identical to
// a healthy one, which is exactly why it has to say so.
func degradedText(room string, problems []string) string {
	if len(problems) == 0 {
		return ""
	}
	return room + ": " + strings.Join(problems, ", ")
}

// discoverSpeakers streams discovered speakers into the program, giving up
// after discoverTimeout or when ctx is cancelled.
func discoverSpeakers(ctx context.Context, program *tea.Program) {
	ctx, cancel := context.WithTimeout(ctx, discoverTimeout)
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

// loadSpeaker builds a speaker's tab: fetch its state, then subscribe to its events.
func loadSpeaker(ctx context.Context, events *sonos.EventManager, speaker sonos.Speaker) tea.Cmd {
	return func() tea.Msg {
		slog.Debug("starting loadSpeaker", "speaker", speaker.RoomName())
		defer slog.Debug("ending loadSpeaker", "speaker", speaker.RoomName())

		device := sonos.NewDevice(speaker)

		var degraded []string
		if err := device.RefreshAll(ctx); err != nil {
			slog.Warn("refreshing speaker state", "speaker", speaker, "err", err)
			degraded = append(degraded, "state may be stale")
		}
		if err := events.Subscribe(ctx, device); err != nil {
			slog.Warn("subscribing to speaker events", "speaker", speaker, "err", err)
			degraded = append(degraded, "no live updates")
		}

		return speakerReadyMsg{
			title: speaker.RoomName(), ip: speaker.IP, speaker: newSpeakerModel(ctx, device),
			degraded: degradedText(speaker.RoomName(), degraded),
		}
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

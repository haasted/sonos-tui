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
	"github.com/haasted/sonos-sandbox/internal/store"
)

const (
	discoverTimeout = 5 * time.Second
	shutdownTimeout = 5 * time.Second
)

var logsFlag = flag.Bool("logs", false, "capture and print log output once the program exits; set to false to discard all logs")
var dbFlag = flag.String("db", "", "path to the application database; empty means the default location")

func main() {
	flag.Parse()

	if why, ok := preflight(); !ok {
		fmt.Fprintln(os.Stderr, why)
		os.Exit(1)
	}

	dumpLogs := log.CaptureLogs(*logsFlag, slog.LevelDebug)
	defer dumpLogs()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := openStore(ctx)
	if err != nil {
		slog.Error("opening the database", "err", err)
		return
	}
	defer db.Close()

	events, err := sonos.NewEventManager()
	if err != nil {
		slog.Error("starting event listener", "err", err)
		return
	}

	cached := loadCachedSpeakers(ctx, db.Cache())
	selected, err := db.Settings().Get(ctx, store.SettingSelectedSpeaker, "")
	if err != nil {
		slog.Warn("reading the selected speaker", "err", err)
	}

	model := newAppModel(ctx, events, cached, selected)
	model.db = db
	program := tea.NewProgram(model, tea.WithContext(ctx))

	go discoverSpeakers(ctx, program)

	final, runErr := program.Run()
	saveSession(ctx, db, final)
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

// openStore opens the application database, creating it on a first run.
// The program stops if this fails: the store holds the speaker cache today
// and will hold playlists, so running without it is running degraded.
func openStore(ctx context.Context) (*store.Store, error) {
	path := *dbFlag
	if path == "" {
		var err error
		if path, err = store.DefaultPath(); err != nil {
			return nil, err
		}
	}

	db, err := store.Open(ctx, path)
	if err != nil {
		return nil, err
	}

	version, err := db.Version(ctx)
	if err != nil {
		slog.Warn("reading the schema version", "path", path, "err", err)
	}
	slog.Info("database ready", "path", path, "schema", version)
	return db, nil
}

// loadCachedSpeakers returns last run's speakers, or nothing when there is no
// cache or it was recorded on a different network. Every failure is silent:
// the cache is a shortcut, and discovery still runs regardless.
func loadCachedSpeakers(ctx context.Context, cache *store.Cache) []sonos.Speaker {
	cached, ok := sonos.LoadCache(ctx, cache)
	if !ok {
		slog.Info("no usable speaker cache — discovering from scratch")
		return nil
	}

	subnet, err := sonos.LocalSubnet()
	if err != nil {
		slog.Debug("cannot read local subnet — ignoring cache", "err", err)
		return nil
	}
	if !cached.UsableOn(subnet) {
		slog.Info("cache was recorded on another network — ignoring it",
			"cached", cached.Subnet, "current", subnet)
		return nil
	}

	slog.Info("seeding tabs from cache",
		"speakers", len(cached.Speakers),
		"subnet", cached.Subnet,
		"age", time.Since(cached.SavedAt).Truncate(time.Second))
	return cached.Speakers
}

// saveSession records what the next run starts from: the speakers still on
// screen, in the cache, and the selected one, in settings. Each is written on
// its own, so a failure in one does not lose the other.
func saveSession(ctx context.Context, db *store.Store, final tea.Model) {
	app, ok := final.(appModel)
	if !ok {
		return
	}
	speakers, selected := app.speakerSnapshot()

	if selected != "" {
		if err := db.Settings().Set(ctx, store.SettingSelectedSpeaker, selected); err != nil {
			slog.Warn("writing the selected speaker", "err", err)
		}
	}
	if len(speakers) == 0 {
		return
	}
	if err := sonos.SaveCache(ctx, db.Cache(), speakers); err != nil {
		slog.Warn("writing speaker cache", "err", err)
		return
	}
	slog.Info("wrote speaker cache", "speakers", len(speakers), "selected", selected)
}

// checkCachedSpeaker asks whether a cached speaker still answers at its cached
// address, behind the already-rendered tab. It loads nothing: the app starts
// one load per speaker, on this answer or a discovery answer, whichever is first.
func checkCachedSpeaker(ctx context.Context, speaker sonos.Speaker) tea.Cmd {
	return func() tea.Msg {
		err := sonos.VerifySpeaker(ctx, speaker)
		if err != nil {
			slog.Info("cached speaker failed verification", "speaker", speaker.IP, "err", err)
		}
		return cachedCheckedMsg{id: speakerKey(speaker), err: err}
	}
}

// activateSpeaker loads a cached speaker that has answered: reads its name
// again, in case the room was renamed, refreshes its state and subscribes to
// its events, all on the Device its tab already renders.
func activateSpeaker(ctx context.Context, events *sonos.EventManager, device *sonos.Device) tea.Cmd {
	return func() tea.Msg {
		title := device.RoomName()
		if desc, err := device.RefreshDescription(ctx); err != nil {
			slog.Warn("reading a cached speaker's description", "speaker", device.IP, "err", err)
		} else if desc.RoomName != "" {
			title = desc.RoomName
		}

		var degraded []string
		if err := device.RefreshAll(ctx); err != nil {
			slog.Warn("refreshing cached speaker", "speaker", device.IP, "err", err)
			degraded = append(degraded, "state may be stale")
		}
		if err := events.Subscribe(ctx, device); err != nil {
			slog.Warn("subscribing to cached speaker", "speaker", device.IP, "err", err)
			degraded = append(degraded, "no live updates")
		}
		return speakerVerifiedMsg{
			title: title, speaker: device,
			degraded: degradedText(title, degraded),
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

	start := time.Now()
	discovery, err := sonos.StartDiscovery(ctx)
	if err != nil {
		slog.Error("discovery failed", "err", err)
		program.Send(discoveryDoneMsg{err: err, took: time.Since(start)})
		return
	}

	found := 0
	for speaker := range discovery.Speakers {
		found++
		program.Send(speakerFoundMsg{speaker: speaker})
	}
	report := discovery.Report()
	slog.Info("discovery done", "found", found, "interfaces", report.Interfaces,
		"searches", report.Searches, "unsent", len(report.Unsent))
	program.Send(discoveryDoneMsg{found: found, report: report, took: time.Since(start)})
}

// loadSpeaker builds a speaker's tab: its name, its state, then its events.
// The tab waits for the name, so it never appears under its address and
// moves once the room name arrives.
func loadSpeaker(ctx context.Context, events *sonos.EventManager, speaker sonos.Speaker) tea.Cmd {
	return func() tea.Msg {
		slog.Debug("starting loadSpeaker", "speaker", speaker.IP)
		defer slog.Debug("ending loadSpeaker", "speaker", speaker.IP)

		var degraded []string
		if _, err := speaker.LoadDescription(ctx); err != nil {
			slog.Warn("reading speaker description", "speaker", speaker.IP, "err", err)
			degraded = append(degraded, "room name unknown")
		}
		device := sonos.NewDevice(speaker)

		if err := device.RefreshAll(ctx); err != nil {
			slog.Warn("refreshing speaker state", "speaker", speaker, "err", err)
			degraded = append(degraded, "state may be stale")
		}
		if err := events.Subscribe(ctx, device); err != nil {
			slog.Warn("subscribing to speaker events", "speaker", speaker, "err", err)
			degraded = append(degraded, "no live updates")
		}

		return speakerReadyMsg{
			title: speaker.RoomName(), id: speakerKey(speaker), speaker: newSpeakerModel(ctx, device),
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

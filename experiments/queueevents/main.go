// Command queueevents watches ContentDirectory events so we can find out what
// a speaker really reports when its queue changes.
//
// It answers two questions the ContentDirectory work depends on. First, which
// state variables the service says it sends: the SCPD lists them, and it is
// printed once at startup. Second, what actually arrives when the queue is
// edited from the Sonos app.
//
// Nothing here is production code. Its findings belong in SONOS-NOTES.md.
package main

import (
	"context"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/haasted/sonos-tui/internal/sonos"
)

const (
	// discoverFor is how long to look for speakers. Long enough for a slow
	// household, short enough to keep the harness usable.
	discoverFor = 5 * time.Second

	// scpdPath describes the ContentDirectory service: its actions, and which
	// of its state variables are evented.
	scpdPath = "/xml/ContentDirectory1.xml"
)

var (
	roomFlag   = flag.String("room", "", "watch only this room; every room by default")
	scpdFlag   = flag.Bool("scpd", true, "print which ContentDirectory variables are evented, then watch")
	browseFlag = flag.Bool("browse", true, "read the queue when an event says it changed")

	// inventoryFlag answers the question the Sonos app will not let a reader
	// ask: does a speaker playing a station still hold a queue?
	inventoryFlag = flag.Bool("inventory", true, "read every speaker's queue once before watching")
)

func main() {
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	speakers, err := discover(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "discovering speakers:", err)
		os.Exit(1)
	}
	if len(speakers) == 0 {
		fmt.Fprintln(os.Stderr, "no speakers answered")
		os.Exit(1)
	}

	if *scpdFlag {
		printSCPD(speakers[0])
	}

	events, err := sonos.NewEventManager()
	if err != nil {
		fmt.Fprintln(os.Stderr, "starting the event listener:", err)
		os.Exit(1)
	}

	watch(ctx, events, speakers)
}

// discover collects the speakers once, sorted so the numbering is the same
// from one run to the next.
func discover(ctx context.Context) ([]sonos.Speaker, error) {
	deadline, cancel := context.WithTimeout(ctx, discoverFor)
	defer cancel()

	found, err := sonos.Discover(deadline)
	if err != nil {
		return nil, err
	}

	var speakers []sonos.Speaker
	for speaker := range found {
		if *roomFlag == "" || strings.EqualFold(speaker.RoomName(), *roomFlag) {
			speakers = append(speakers, speaker)
		}
	}
	sort.Slice(speakers, func(i, j int) bool { return speakers[i].IP < speakers[j].IP })
	return speakers, nil
}

// printSCPD reports which ContentDirectory variables the speaker says it
// events. This is the question the design turns on, and the speaker is the
// only authority: the UPnP standard says what MAY be evented, not what is.
func printSCPD(speaker sonos.Speaker) {
	body, err := fetch("http://" + speaker.IP + ":1400" + scpdPath)
	if err != nil {
		fmt.Printf("could not read the SCPD from %s: %v\n\n", speaker.RoomName(), err)
		return
	}

	var doc struct {
		Variables []struct {
			Name       string `xml:"name"`
			SendEvents string `xml:"sendEvents,attr"`
			DataType   string `xml:"dataType"`
		} `xml:"serviceStateTable>stateVariable"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil {
		fmt.Printf("could not parse the SCPD: %v\n\n", err)
		return
	}

	fmt.Printf("ContentDirectory state variables on %s (%s)\n", speaker.RoomName(), speaker.IP)
	for _, v := range doc.Variables {
		mark := "  "
		if !strings.EqualFold(v.SendEvents, "no") {
			mark = "->" // evented: this one arrives in a NOTIFY
		}
		fmt.Printf("  %s %-28s %s\n", mark, v.Name, v.DataType)
	}
	fmt.Println("\n  -> marks a variable the speaker sends as an event.")
	fmt.Println()
}

func fetch(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s said %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// watch subscribes to every speaker and prints what arrives until interrupted.
func watch(ctx context.Context, events *sonos.EventManager, speakers []sonos.Speaker) {
	devices := make([]*sonos.Device, 0, len(speakers))
	for _, speaker := range speakers {
		device := sonos.NewDevice(speaker)
		if err := events.Subscribe(ctx, device); err != nil {
			fmt.Printf("  %-12s could not subscribe: %v\n", speaker.RoomName(), err)
			continue
		}
		devices = append(devices, device)
	}

	if *inventoryFlag {
		inventory(ctx, devices)
	}

	fmt.Printf("watching %d speaker(s). Edit a queue in the Sonos app.\n\n", len(devices))
	for _, device := range devices {
		go print(ctx, device)
	}

	<-ctx.Done()
}

// inventory reads every speaker's queue once, whatever it is playing. A speaker
// on a station reports one track through AVTransport, which says nothing about
// the queue behind it — this is where the two are put side by side.
func inventory(ctx context.Context, devices []*sonos.Device) {
	fmt.Println("queue of each speaker, before any event:")
	fmt.Printf("  %-12s %6s %7s %9s  %-18s %s\n",
		"room", "queue", "tracks", "UpdateID", "playing", "queue ends")

	for _, device := range devices {
		if err := device.Refresh(ctx); err != nil {
			slog.Debug("refreshing before inventory", "speaker", device.RoomName(), "err", err)
		}
		state := device.State()

		queue, err := device.Queue(ctx)
		if err != nil {
			fmt.Printf("  %-12s  could not read the queue: %v\n", device.RoomName(), err)
			continue
		}

		fmt.Printf("  %-12s %6d %7d %9d  %-18s %s\n",
			device.RoomName(), len(queue.Items), state.Tracks, queue.UpdateID,
			scheme(state.Source.URI), ends(queue.Items))
	}
	fmt.Println()
}

// print reports one speaker's events. AVTransport is shown beside the
// ContentDirectory ones on purpose: its NumberOfTracks is the partial signal we
// have today, and the point of the harness is to see where the two disagree.
func print(ctx context.Context, device *sonos.Device) {
	for event := range device.Events() {
		state := device.State()
		stamp := time.Now().Format("15:04:05.000")

		switch event.Kind {
		case sonos.EventContentDirectory:
			fmt.Printf("%s  %-12s ContentDirectory  system=%d  %s\n",
				stamp, device.RoomName(), event.Directory.SystemUpdateID, containers(event.Directory))

			// Read it at once rather than after a pause. The whole question is
			// what a reader gets by being quick, so the harness is quick and
			// reports what it got.
			if updateID, queue := event.Directory.QueueUpdate(); queue && *browseFlag {
				browse(ctx, device, updateID)
			}
		case sonos.EventAVTransport:
			fmt.Printf("%s  %-12s AVTransport       tracks=%d track=%d  queue-id=%d  %s\n",
				stamp, device.RoomName(), state.Tracks, state.Track, state.QueueUpdateID, state.Source.URI)
		}
	}
}

// browse reads the queue the event just announced, and reports three things the
// design turns on: how long the read took, whether the speaker gives back the
// same id the event carried, and what the queue held at that moment.
//
// A read that lands while the speaker is still writing returns a shorter queue
// with nothing to mark it as provisional, so the count here is the evidence for
// or against reading straight away.
func browse(ctx context.Context, device *sonos.Device, announced int) {
	start := time.Now()
	queue, err := device.Queue(ctx)
	took := time.Since(start)

	if err != nil {
		fmt.Printf("%s  %-12s browsed           failed after %s: %v\n",
			time.Now().Format("15:04:05.000"), device.RoomName(), took.Round(time.Millisecond), err)
		return
	}

	agreement := "id matches the event"
	if queue.UpdateID != announced {
		agreement = fmt.Sprintf("DISAGREES: event said %d", announced)
	}

	fmt.Printf("%s  %-12s browsed           %d items  UpdateID=%d  in %s  %s  %s\n",
		time.Now().Format("15:04:05.000"), device.RoomName(), len(queue.Items), queue.UpdateID,
		took.Round(time.Millisecond), agreement, ends(queue.Items))
}

// scheme is the part of a source URI that says what kind of thing is playing,
// which is the whole reason a queue behind a station goes unnoticed.
func scheme(uri string) string {
	if uri == "" {
		return "(nothing)"
	}
	if colon := strings.Index(uri, ":"); colon > 0 {
		return uri[:colon]
	}
	return uri
}

// ends names the first and last track, which is the cheapest way to see a queue
// replaced rather than edited.
func ends(items []sonos.QueueItem) string {
	if len(items) == 0 {
		return "(empty)"
	}
	if len(items) == 1 {
		return fmt.Sprintf("[%s]", items[0].Title)
	}
	return fmt.Sprintf("[%s … %s]", items[0].Title, items[len(items)-1].Title)
}

func containers(event sonos.ContentDirectoryEvent) string {
	if len(event.Containers) == 0 {
		return "(no container named)"
	}

	var parts []string
	for _, c := range event.Containers {
		mark := ""
		if _, queue := event.QueueUpdate(); queue && c.ID == "Q:0" {
			mark = "  <- the queue"
		}
		parts = append(parts, fmt.Sprintf("%s=%d%s", c.ID, c.UpdateID, mark))
	}
	return strings.Join(parts, " ")
}

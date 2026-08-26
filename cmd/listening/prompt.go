package main

import (
	"bufio"
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// descriptionGrace is how much longer we wait for background
// device_description.xml fetches (room names) to finish after SSDP
// discovery itself has already closed, before printing the menu with
// whatever's ready. Bounded on purpose — a straggler just shows its IP
// instead of a room name rather than delaying the prompt indefinitely.
const descriptionGrace = 2 * time.Second

// commands are the speaker actions reachable from the prompt, keyed by the
// word typed after the selector (e.g. "2 play"). Add more here as needed.
var commands = map[string]func(*sonos.Device) error{
	"play":  (*sonos.Device).Play,
	"pause": (*sonos.Device).Pause,
	"next":  (*sonos.Device).Next,
	"prev":  (*sonos.Device).Previous,
}

// menuEntry pairs a fixed number with the speaker it refers to for the
// lifetime of one prompt session.
type menuEntry struct {
	number  int
	speaker sonos.Speaker
	label   string // room name if known by menu-build time, else the IP
}

// waitForDescriptions gives background description fetches a little extra
// time to complete after discovery has already closed, so the menu shows
// room names instead of IPs where possible. Returns early if ctx is done.
func waitForDescriptions(ctx context.Context, subs *subscriptions, maxWait time.Duration) {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		allReady := true
		for _, sub := range subs.all() {
			if _, ok := sub.speaker.Description(); !ok {
				allReady = false
				break
			}
		}
		if allReady {
			return
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return
		}
	}
}

// buildMenu assigns stable numbers to every currently subscribed speaker,
// sorted by label so the numbering doesn't depend on map iteration order
// (which subs.all() otherwise gives no guarantee about).
func buildMenu(subs *subscriptions) []menuEntry {
	all := subs.all()
	sort.Slice(all, func(i, j int) bool {
		return menuLabel(all[i].speaker) < menuLabel(all[j].speaker)
	})

	entries := make([]menuEntry, len(all))
	for i, sub := range all {
		entries[i] = menuEntry{number: i + 1, speaker: sub.speaker, label: menuLabel(sub.speaker)}
	}
	return entries
}

func menuLabel(speaker sonos.Speaker) string {
	if desc, ok := speaker.Description(); ok && desc.RoomName != "" {
		return desc.RoomName
	}
	return speaker.IP
}

func printMenu(entries []menuEntry) {
	fmt.Println("Speakers:")
	for _, e := range entries {
		fmt.Printf("  %d  %s\n", e.number, e.label)
	}
	fmt.Println(`Enter a command, e.g. "2 play" or "kitchen pause". Ctrl+C to quit.`)
	fmt.Println("Available commands:", strings.Join(slices.Sorted(maps.Keys(commands)), " "))
}

// findEntry resolves a selector typed at the prompt — either a menu number
// or a case-insensitive match against a speaker's room/display name.
func findEntry(entries []menuEntry, selector string) (menuEntry, bool) {
	if n, err := strconv.Atoi(selector); err == nil {
		for _, e := range entries {
			if e.number == n {
				return e, true
			}
		}
		return menuEntry{}, false
	}

	for _, e := range entries {
		desc, _ := e.speaker.Description()
		if strings.EqualFold(desc.RoomName, selector) || strings.EqualFold(desc.DisplayName, selector) {
			return e, true
		}
	}
	return menuEntry{}, false
}

// runPrompt reads "<selector> <command>" lines from stdin until ctx is done
// or stdin closes (EOF). Reading happens on its own goroutine so Ctrl+C
// still works to quit even while blocked waiting for a line.
func runPrompt(ctx context.Context, entries []menuEntry) {
	lines := make(chan string)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-lines:
			if !ok {
				return
			}
			handleCommand(entries, line)
		}
	}
}

func handleCommand(entries []menuEntry, line string) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		fmt.Println(`usage: <number-or-name> <command>, e.g. "2 play"`)
		return
	}

	entry, ok := findEntry(entries, fields[0])
	if !ok {
		fmt.Printf("unknown speaker %q\n", fields[0])
		return
	}

	cmd, ok := commands[strings.ToLower(fields[1])]
	if !ok {
		fmt.Printf("unknown command %q (try: play, pause, prev, next etc)\n", fields[1])
		return
	}

	if err := cmd(sonos.NewDevice(entry.speaker)); err != nil {
		fmt.Printf("%s: %v\n", entry.label, err)
		return
	}
	fmt.Printf("%s: ok\n", entry.label)
}

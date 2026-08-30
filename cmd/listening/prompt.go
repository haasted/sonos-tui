package main

import (
	"bufio"
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

const menuRevealDelay = 2 * time.Second

var commands = map[string]func(*sonos.Device) error{
	"play":  (*sonos.Device).Play,
	"pause": (*sonos.Device).Pause,
	"next":  (*sonos.Device).Next,
	"prev":  (*sonos.Device).Previous,
}

func (s *subscriptions) revealMenu() {
	s.mu.Lock()
	defer s.mu.Unlock()

	fmt.Println(`Enter a command, e.g. "2 play" or "kitchen pause". Ctrl+C to quit.`)
	fmt.Println("Available commands:", strings.Join(slices.Sorted(maps.Keys(commands)), " "))
	fmt.Println("Speakers:")
	for i, sid := range s.order {
		fmt.Printf("  %d  %s\n", i+1, s.byID[sid].speaker.DisplayName())
	}
	s.revealed = true
}

func runPrompt(ctx context.Context, subs *subscriptions) {
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
			handleCommand(subs, line)
		}
	}
}

func handleCommand(subs *subscriptions, line string) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		fmt.Println(`usage: <number-or-name> <command>, e.g. "2 play"`)
		return
	}

	sub, ok := subs.find(fields[0])
	if !ok {
		fmt.Printf("unknown speaker %q\n", fields[0])
		return
	}

	cmd, ok := commands[strings.ToLower(fields[1])]
	if !ok {
		fmt.Printf("unknown command %q (try: play, pause, prev, next etc)\n", fields[1])
		return
	}

	label := sub.speaker.DisplayName()
	if err := cmd(sonos.NewDevice(sub.speaker)); err != nil {
		fmt.Printf("%s: %v\n", label, err)
		return
	}
	fmt.Printf("%s: ok\n", label)
}

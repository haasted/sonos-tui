package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

func main() {
	var ip string

	if len(os.Args) > 1 {
		// IP provided explicitly: go run ./cmd/sonos 192.168.68.57
		ip = os.Args[1]
	} else {
		// Auto-discover speakers on the local network, printing each as
		// it's found rather than waiting for the full discovery window.
		fmt.Println("Discovering Sonos speakers...")
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		ch, err := sonos.Discover(ctx)
		if err != nil {
			log.Fatalf("Discovery failed: %v", err)
		}

		var speakers []sonos.Speaker
		for s := range ch {
			speakers = append(speakers, s)
			fmt.Printf("  [%d] %s  (%s)\n", len(speakers), s.IP, s.USN)
		}
		if len(speakers) == 0 {
			log.Fatal("No Sonos speakers found on the local network.")
		}
		fmt.Println()

		// Use the first speaker found.
		ip = speakers[0].IP
	}

	c := sonos.NewClient(ip)
	fmt.Printf("Querying speaker at %s...\n\n", ip)

	// Playback state
	state, err := c.GetTransportInfo()
	if err != nil {
		log.Fatalf("GetTransportInfo: %v", err)
	}
	fmt.Printf("State:  %s\n", state.State)

	// Current track
	track, err := c.GetPositionInfo()
	if err != nil {
		log.Fatalf("GetPositionInfo: %v", err)
	}
	fmt.Printf("Title:  %s\n", track.Title)
	fmt.Printf("Artist: %s\n", track.Artist)
	fmt.Printf("Album:  %s\n", track.Album)
	fmt.Printf("Time:   %s / %s\n", track.RelTime, track.TrackDuration)
	fmt.Printf("URI:    %s\n", track.TrackURI)

	// Volume
	vol, err := c.GetVolume()
	if err != nil {
		log.Fatalf("GetVolume: %v", err)
	}
	fmt.Printf("Volume: %d\n", vol)
}


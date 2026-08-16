package sonos

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

const (
	ssdpAddr = "239.255.255.250:1900"
	ssdpST   = "urn:schemas-upnp-org:device:ZonePlayer:1"
)

var ssdpRequest = strings.Join([]string{
	"M-SEARCH * HTTP/1.1",
	"HOST: 239.255.255.250:1900",
	`MAN: "ssdp:discover"`,
	"MX: 3",
	"ST: " + ssdpST,
	"", "",
}, "\r\n")

// Speaker holds the basic information discovered about a Sonos speaker.
type Speaker struct {
	IP       string
	Location string // Full URL to device_description.xml
	USN      string // Unique Service Name (contains RINCON ID)
}

// discoverChanBuffer bounds how many Speakers can queue on the channel
// Discover returns before its background goroutine blocks on send. Small on
// purpose — this streams a handful of physical devices on a LAN, not a
// high-volume feed.
const discoverChanBuffer = 10

// Discover sends an SSDP multicast and streams each Sonos speaker found on
// the returned channel as its response arrives, closing the channel once
// ctx's deadline passes or ctx is canceled. ctx must carry a deadline (via
// context.WithTimeout or context.WithDeadline) since SSDP discovery is
// inherently time-boxed — devices may never reply, so an undeadlined
// context would block forever.
func Discover(ctx context.Context) (<-chan Speaker, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, errors.New("sonos: Discover requires a context with a deadline")
	}

	localIP, err := getLocalIP()
	if err != nil {
		return nil, fmt.Errorf("detecting local IP: %w", err)
	}

	// Bind UDP socket to the active LAN interface. Ownership passes to the
	// background goroutine below once it starts — from here on, any error
	// path before that must close conn itself.
	conn, err := net.ListenPacket("udp4", localIP+":0")
	if err != nil {
		return nil, fmt.Errorf("opening UDP socket: %w", err)
	}

	dest, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		conn.Close()
		return nil, err
	}

	if _, err := conn.WriteTo([]byte(ssdpRequest), dest); err != nil {
		conn.Close()
		return nil, fmt.Errorf("sending SSDP request: %w", err)
	}

	if err := conn.SetReadDeadline(deadline); err != nil {
		conn.Close()
		return nil, err
	}

	ch := make(chan Speaker, discoverChanBuffer)

	go func() {
		var (
			seen      = map[string]bool{}
			buf       = make([]byte, 4096)
			watchDone = make(chan struct{})
		)

		defer close(ch)
		defer conn.Close()
		defer close(watchDone)

		// net.PacketConn has no context-aware read, so a watcher closes
		// the conn on early cancellation to unblock ReadFrom before the
		// deadline.
		go func() {
			select {
			case <-ctx.Done():
				conn.Close()
			case <-watchDone:
			}
		}()

		for {
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				return // timeout or closed
			}

			ip := addr.(*net.UDPAddr).IP.String()
			if seen[ip] {
				continue
			}
			seen[ip] = true

			response := string(buf[:n])
			location := extractHeader(response, "LOCATION")
			if location == "" {
				continue
			}

			speaker := Speaker{
				IP:       ip,
				Location: location,
				USN:      extractHeader(response, "USN"),
			}
			select {
			case ch <- speaker:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

func extractHeader(response, header string) string {
	println(" --- \n", response, "\n --- ")
	for line := range strings.SplitSeq(response, "\n") {
		line = strings.TrimRight(line, "\r")
		if idx := strings.IndexByte(line, ':'); idx > 0 {
			name := line[:idx]
			if strings.EqualFold(name, header) {
				return strings.TrimSpace(line[idx+1:])
			}
		}
	}
	return ""

}

// getLocalIP finds the IP of the active LAN interface by attempting a
// non-sending UDP connection to a public address.
func getLocalIP() (string, error) {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

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
	IP          string
	Location    string // Full URL to device_description.xml
	USN         string // Unique Service Name (contains RINCON ID)
}

// Discover sends an SSDP multicast and returns all Sonos speakers found
// before ctx's deadline. ctx must carry a deadline (via context.WithTimeout
// or context.WithDeadline) since SSDP discovery is inherently time-boxed —
// devices may never reply, so an undeadlined context would block forever.
// If ctx is canceled early, whatever speakers were found up to that point
// are returned with a nil error, same as a natural deadline expiry.
func Discover(ctx context.Context) ([]Speaker, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, errors.New("sonos: Discover requires a context with a deadline")
	}

	localIP, err := getLocalIP()
	if err != nil {
		return nil, fmt.Errorf("detecting local IP: %w", err)
	}

	// Bind UDP socket to the active LAN interface.
	conn, err := net.ListenPacket("udp4", localIP+":0")
	if err != nil {
		return nil, fmt.Errorf("opening UDP socket: %w", err)
	}
	defer conn.Close()

	dest, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, err
	}

	if _, err := conn.WriteTo([]byte(ssdpRequest), dest); err != nil {
		return nil, fmt.Errorf("sending SSDP request: %w", err)
	}

	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}

	// net.PacketConn has no context-aware read, so a watcher closes the
	// conn on early cancellation to unblock ReadFrom before the deadline.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	seen := map[string]bool{}
	var speakers []Speaker
	buf := make([]byte, 4096)

	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			break // timeout or closed
		}

		ip := addr.(*net.UDPAddr).IP.String()
		if seen[ip] {
			continue
		}
		seen[ip] = true

		response := string(buf[:n])
		location := extractHeader(response, "LOCATION")
		usn := extractHeader(response, "USN")

		if location != "" {
			speakers = append(speakers, Speaker{
				IP:       ip,
				Location: location,
				USN:      usn,
			})
		}
	}

	return speakers, nil
}

func extractHeader(response, header string) string {
	for _, line := range strings.Split(response, "\n") {
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

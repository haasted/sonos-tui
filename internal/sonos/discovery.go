package sonos

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
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
// within the given timeout.
func Discover(timeout time.Duration) ([]Speaker, error) {
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

	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}

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

var headerRe = regexp.MustCompile(`(?im)^([^:]+):\s*(.+)$`)

func extractHeader(response, header string) string {
	for _, match := range headerRe.FindAllStringSubmatch(response, -1) {
		if strings.EqualFold(match[1], header) {
			return strings.TrimSpace(match[2])
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

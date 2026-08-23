package sonos

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
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

	// description holds the result of the background device_description.xml
	// fetch Discover starts for each speaker, once it completes. A *pointer*
	// to an atomic.Pointer, not an atomic.Pointer itself: Speaker is copied
	// by value everywhere (sent on channels, embedded in Device, ...), and
	// atomic.Pointer must never be copied after use — indirecting through a
	// pointer means every copy shares the same underlying slot instead.
	description *atomic.Pointer[DeviceDescription]
}

// Description returns the speaker's device_description.xml metadata —
// currently RoomName/DisplayName/ModelName/ModelNumber — if the background
// fetch Discover started for it has completed, and ok=false otherwise
// (including for a Speaker built directly rather than via Discover, e.g. in
// a test). Safe to call from any goroutine, on any copy of this Speaker.
func (s Speaker) Description() (desc DeviceDescription, ok bool) {
	if s.description == nil {
		return DeviceDescription{}, false
	}
	d := s.description.Load()
	if d == nil {
		return DeviceDescription{}, false
	}
	return *d, true
}

var _ slog.LogValuer = Speaker{}

// LogValue includes Description() when it's ready, nested via slog's
// automatic LogValuer composition — nothing extra needed here for that.
func (s Speaker) LogValue() slog.Value {
	attrs := []slog.Attr{slog.String("ip", s.IP), slog.String("usn", s.USN)}
	if desc, ok := s.Description(); ok {
		attrs = append(attrs, slog.Any("description", desc))
	}
	return slog.GroupValue(attrs...)
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

	localIP, err := GetLocalIP()
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

		// net.PacketConn has no context-aware read, so a watcher closes the conn on
		// early cancellation to unblock ReadFrom before the deadline.
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
				IP:          ip,
				Location:    location,
				USN:         extractHeader(response, "USN"),
				description: &atomic.Pointer[DeviceDescription]{},
			}
			select {
			case ch <- speaker:
				// Kick off in the background so a slow/unreachable device
				// description never delays discovery itself — only start it
				// once we know speaker was actually delivered to a receiver,
				// since nothing would ever observe the result otherwise.
				go fetchDescription(speaker)
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// fetchDescription fetches speaker's device_description.xml and stores
// the result on speaker.description once it's ready. Uses its own timeout
// (via descriptionHTTPClient) independent of Discover's ctx, which is
// scoped to the SSDP window and may already be near its deadline for a
// speaker discovered late in that window.
func fetchDescription(speaker Speaker) {
	desc, err := FetchDeviceDescription(context.Background(), descriptionHTTPClient, speaker.Location)
	if err != nil {
		return // best-effort: Description() just stays unavailable
	}

	slog.Info("Device description retrieved", "speaker", desc)
	speaker.description.Store(&desc)
}

func extractHeader(response, header string) string {
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

func GetLocalIP() (string, error) {
	//	https://stackoverflow.com/a/31551220/13075
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("Could not retrieve local IP address")
}

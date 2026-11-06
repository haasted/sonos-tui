package sonos

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
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
	"MX: 1",
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

// ID is the speaker's bare RINCON identifier, which is what the zone group
// topology refers to every speaker by.
//
// The USN wraps it: "uuid:RINCON_xxx::urn:schemas-upnp-org:device:ZonePlayer:1".
// One place unwraps it, because an id sliced out at each call site is an id
// that will eventually be sliced differently in one of them.
func (s Speaker) ID() string {
	id := strings.TrimPrefix(s.USN, "uuid:")
	if at := strings.Index(id, "::"); at >= 0 {
		id = id[:at]
	}
	return id
}

// RoomName is the room the speaker is in, falling back to its address before
// the description has arrived. Not DeviceDescription.DisplayName, which is the
// model the speaker reports — "Era 100" against a room of "Kitchen".
func (s Speaker) RoomName() string {
	if desc, ok := s.Description(); ok && desc.RoomName != "" {
		return desc.RoomName
	}
	return s.IP
}

// storeDescription publishes a freshly fetched description. Speaker is copied
// by value, but description is a shared pointer, so every copy sees it.
func (s Speaker) storeDescription(desc DeviceDescription) {
	if s.description != nil {
		s.description.Store(&desc)
	}
}

// LoadDescription is the description, read from the speaker first if no
// read has finished yet. A caller that must show the final room name waits
// on this rather than on the read Discover started in the background.
func (s Speaker) LoadDescription(ctx context.Context) (DeviceDescription, error) {
	if desc, ok := s.Description(); ok {
		return desc, nil
	}
	return s.RefreshDescription(ctx)
}

// RefreshDescription reads the description from the speaker again and keeps
// it, so every copy of this Speaker sees a room renamed since it was cached.
func (s Speaker) RefreshDescription(ctx context.Context) (DeviceDescription, error) {
	desc, err := FetchDeviceDescription(ctx, descriptionHTTPClient, s.Location)
	if err != nil {
		return DeviceDescription{}, fmt.Errorf("reading the description of %s: %w", s.IP, err)
	}
	s.storeDescription(desc)
	return desc, nil
}

func NewSpeaker(ip, location, usn string, desc DeviceDescription) Speaker {
	s := Speaker{IP: ip, Location: location, USN: usn, description: &atomic.Pointer[DeviceDescription]{}}
	s.description.Store(&desc)
	return s
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

// sighting is one search's answer, before anything has checked whether
// another search already found the same speaker.
type sighting struct {
	speaker Speaker
	via     string // which search found it, e.g. "ssdp en0" or "mdns en0"
}

// searchInterface is a network interface worth searching on, and the IPv4
// address it has.
type searchInterface struct {
	name  string
	ip    net.IP
	iface *net.Interface
}

// ErrNoNetwork is what discovery answers when no interface can search: none
// is up, able to multicast and holding an IPv4 address.
var ErrNoNetwork = errors.New("sonos: no network interface to search on")

// Discovery is one run of discovery: the speakers it finds as they answer,
// and, once Speakers has closed, a report of how the searches went.
type Discovery struct {
	Speakers <-chan Speaker

	mu     sync.Mutex
	report DiscoveryReport
}

// DiscoveryReport says what discovery tried, so a run that found nothing can
// tell a search that never left this computer from one nobody answered.
type DiscoveryReport struct {
	Interfaces []string // the interfaces searched on
	Searches   int      // the searches started, SSDP and mDNS on each interface
	Unsent     []error  // why each search that could not be sent failed
}

// Discover searches for Sonos speakers as StartDiscovery does, for a caller
// that needs only the speakers.
func Discover(ctx context.Context) (<-chan Speaker, error) {
	discovery, err := StartDiscovery(ctx)
	if err != nil {
		return nil, err
	}
	return discovery.Speakers, nil
}

// StartDiscovery searches for Sonos speakers by SSDP and by mDNS, on every
// IPv4 interface that can multicast, all at once. It streams each speaker
// once, the first time any search finds it, and closes Speakers when ctx ends.
//
// ctx must carry a deadline: a speaker may never answer, so an undeadlined
// search would never end. One search failing is logged and reported, not
// fatal: mDNS alone finds every speaker on a network that drops SSDP's answers.
func StartDiscovery(ctx context.Context) (*Discovery, error) {
	if _, ok := ctx.Deadline(); !ok {
		return nil, errors.New("sonos: Discover requires a context with a deadline")
	}
	ifaces, err := searchInterfaces()
	if err != nil {
		return nil, err
	}
	if len(ifaces) == 0 {
		return nil, ErrNoNetwork
	}

	discovery := &Discovery{}
	for _, iface := range ifaces {
		discovery.report.Interfaces = append(discovery.report.Interfaces, iface.name)
	}
	discovery.report.Searches = 2 * len(ifaces)

	sightings := make(chan sighting, discoverChanBuffer)
	var searches sync.WaitGroup
	for _, iface := range ifaces {
		searches.Add(2)
		go func() {
			defer searches.Done()
			discovery.unsent(searchSSDP(ctx, iface, sightings))
		}()
		go func() {
			defer searches.Done()
			discovery.unsent(searchMDNS(ctx, iface, sightings))
		}()
	}
	go func() {
		searches.Wait()
		close(sightings)
	}()

	found := make(chan Speaker, discoverChanBuffer)
	discovery.Speakers = found
	go firstSightings(ctx, sightings, found)
	return discovery, nil
}

// Report says how the searches went. Read it once Speakers has closed: until
// then, searches still running have not reported.
func (d *Discovery) Report() DiscoveryReport {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.report
}

// Blocked reports that every search failed to leave this computer, as when a
// VPN or a missing macOS permission stops it.
func (r DiscoveryReport) Blocked() bool {
	return r.Searches > 0 && len(r.Unsent) == r.Searches
}

// unsent records why a search could not be sent; nil records nothing.
func (d *Discovery) unsent(err error) {
	if err == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.report.Unsent = append(d.report.Unsent, err)
}

// firstSightings passes on the first sighting of each speaker and drops the
// rest, telling speakers apart by their RINCON id. It starts reading each
// speaker's description as it passes it on, and closes out when in closes.
func firstSightings(ctx context.Context, in <-chan sighting, out chan<- Speaker) {
	defer close(out)
	seen := map[string]bool{}
	for s := range in {
		id := s.speaker.ID()
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		slog.Info("speaker found", "via", s.via, "speaker", s.speaker)

		select {
		case out <- s.speaker:
			// Only once delivered: nothing would observe the result otherwise.
			go fetchDescription(s.speaker)
		case <-ctx.Done():
			for range in {
			}
			return
		}
	}
}

// searchSSDP sends one M-SEARCH from iface and reports every Sonos answer
// until ctx ends. It returns why the search could not be sent, if it was not.
func searchSSDP(ctx context.Context, iface searchInterface, out chan<- sighting) error {
	via := "ssdp " + iface.name
	deadline, _ := ctx.Deadline()

	conn, err := net.ListenPacket("udp4", iface.ip.String()+":0")
	if err != nil {
		slog.Warn("ssdp search could not start", "via", via, "err", err)
		return err
	}
	defer conn.Close()

	dest, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		slog.Warn("ssdp search could not start", "via", via, "err", err)
		return err
	}
	if _, err := conn.WriteTo([]byte(ssdpRequest), dest); err != nil {
		slog.Warn("ssdp search could not be sent", "via", via, "err", err)
		return err
	}
	if err := conn.SetReadDeadline(deadline); err != nil {
		slog.Warn("ssdp search could not start", "via", via, "err", err)
		return err
	}

	// net.PacketConn has no context-aware read, so a watcher closes the conn
	// on early cancellation to unblock ReadFrom before the deadline.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-stop:
		}
	}()

	buf := make([]byte, 4096)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			return nil // the deadline, or closed
		}
		response := string(buf[:n])
		location := extractHeader(response, "LOCATION")
		if location == "" {
			continue
		}
		out <- sighting{via: via, speaker: Speaker{
			IP:          addr.(*net.UDPAddr).IP.String(),
			Location:    location,
			USN:         extractHeader(response, "USN"),
			description: &atomic.Pointer[DeviceDescription]{},
		}}
	}
}

// searchInterfaces lists the interfaces to search on: up, running, able to
// multicast, not loopback, and holding an IPv4 address. A VPN tunnel with
// only link-local IPv6 is left out, as is an adapter with no address.
func searchInterfaces() ([]searchInterface, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("listing network interfaces: %w", err)
	}
	return usableInterfaces(all, func(iface net.Interface) ([]net.Addr, error) { return iface.Addrs() }), nil
}

// usableInterfaces is searchInterfaces without the system call, so the rule
// can be tested. addrs reads one interface's addresses.
func usableInterfaces(all []net.Interface, addrs func(net.Interface) ([]net.Addr, error)) []searchInterface {
	var usable []searchInterface
	for i := range all {
		iface := &all[i]
		flags := iface.Flags
		if flags&net.FlagUp == 0 || flags&net.FlagRunning == 0 || flags&net.FlagMulticast == 0 || flags&net.FlagLoopback != 0 {
			continue
		}
		list, err := addrs(*iface)
		if err != nil {
			continue
		}
		for _, a := range list {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				usable = append(usable, searchInterface{name: iface.Name, ip: ipnet.IP.To4(), iface: iface})
				break
			}
		}
	}
	return usable
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
	ipnet, err := localIPNet()
	if err != nil {
		return "", err
	}
	return ipnet.IP.String(), nil
}

// LocalSubnet returns the local network in CIDR form, e.g. "192.168.68.0/24".
// It is a cheap, approximate identity for "which network am I on" — two homes
// commonly share 192.168.1.0/24 — so it is only ever a fast pre-filter, never
// proof that a cached speaker is still reachable.
func LocalSubnet() (string, error) {
	ipnet, err := localIPNet()
	if err != nil {
		return "", err
	}
	network := &net.IPNet{IP: ipnet.IP.Mask(ipnet.Mask), Mask: ipnet.Mask}
	return network.String(), nil
}

// localIPNet returns the first non-loopback IPv4 interface address, with its
// mask. One syscall, no subprocess and no CGo — see ADR-0001, which rejected
// the SSID and gateway-MAC alternatives for needing both.
func localIPNet() (*net.IPNet, error) {
	//	https://stackoverflow.com/a/31551220/13075
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}

	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet, nil
			}
		}
	}
	return nil, fmt.Errorf("could not retrieve local IP address")
}

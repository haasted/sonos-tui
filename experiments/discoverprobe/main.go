// Command discoverprobe looks for Sonos speakers for a fixed time, in every way
// it knows at once, and logs everything it meets. It is for a machine where
// the client finds too few speakers, and for timing SSDP against mDNS.
//
// Ways it looks, side by side:
//   - sonos.Discover, exactly as the client does (one interface, one M-SEARCH)
//   - SSDP on every IPv4 interface that is up and can multicast, the search
//     sent again each second
//   - mDNS (_sonos._tcp) on each of those interfaces
//
// Every speaker found gets its device_description.xml read. At the end it
// prints, per speaker, which ways found it and how long each took.
//
// Nothing here is production code. Its findings belong in SONOS-NOTES.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/mdns"

	"github.com/haasted/sonos-tui/internal/sonos"
)

const (
	ssdpAddr     = "239.255.255.250:1900"
	ssdpST       = "urn:schemas-upnp-org:device:ZonePlayer:1"
	ssdpResend   = time.Second // how often each interface sends the search again
	mdnsService  = "_sonos._tcp"
	mdnsSuffix   = "." + mdnsService + ".local."
	fetchTimeout = 3 * time.Second
)

var runFor = flag.Duration("for", 10*time.Second, "how long to look for speakers")

// sighting is one answer from one way of looking: a speaker, or something
// else heard while listening for one.
type sighting struct {
	way      string // e.g. "discover", "ssdp en0", "mdns en0"
	ip       string
	location string // device_description.xml, when the answer gave one
	detail   string // what else the answer said, for the log
	sonos    bool   // false for an answer that is not a Sonos speaker
}

// speaker is everything learned about one address.
type speaker struct {
	ip       string
	location string
	firstBy  map[string]time.Duration // way -> time from start to its first answer
	desc     *sonos.DeviceDescription
	descErr  error
}

// probe holds the start time and what has been found, shared by the ways.
type probe struct {
	start    time.Time
	mu       sync.Mutex
	speakers map[string]*speaker
	ways     []string       // every way started, for the summary
	answers  map[string]int // way -> answers from Sonos speakers, repeats included
	fetches  sync.WaitGroup
	client   *http.Client
}

func main() {
	flag.Parse()

	p := &probe{start: time.Now(), speakers: map[string]*speaker{}, answers: map[string]int{},
		client: &http.Client{Timeout: fetchTimeout}}

	// The client's own package logs each description it reads. Only its
	// warnings and errors are worth a line here; this program logs the rest.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn})))
	ctx, cancel := context.WithTimeout(context.Background(), *runFor)
	defer cancel()

	p.logf("discoverprobe: looking for Sonos speakers for %v", *runFor)
	p.logf("host: %s/%s, go %s", runtime.GOOS, runtime.GOARCH, runtime.Version())
	ifaces := p.logInterfaces()

	sightings := make(chan sighting, 64)
	var ways sync.WaitGroup
	start := func(way string, run func()) {
		p.ways = append(p.ways, way)
		ways.Add(1)
		go func() {
			defer ways.Done()
			run()
		}()
	}

	start("discover", func() { p.viaDiscover(ctx, sightings) })
	for _, iface := range ifaces {
		start("ssdp "+iface.name, func() { p.viaSSDP(ctx, iface, sightings) })
		start("mdns "+iface.name, func() { p.viaMDNS(ctx, iface, sightings) })
	}

	go func() {
		ways.Wait()
		close(sightings)
	}()
	for s := range sightings {
		p.record(s)
	}

	p.logf("time is up; waiting for device descriptions still being read")
	p.fetches.Wait()
	p.summarise()
}

// candidate is one interface worth looking on, and the IPv4 address it has.
type candidate struct {
	name string
	ip   net.IP
	net  *net.Interface
}

// logInterfaces prints every interface, and which address the client would
// bind discovery to, then returns the ones SSDP and mDNS can use: up, not
// loopback, multicast-capable, with an IPv4 address.
func (p *probe) logInterfaces() []candidate {
	if ip, err := sonos.GetLocalIP(); err != nil {
		p.logf("the client would bind discovery to: nothing (%v)", err)
	} else {
		p.logf("the client would bind discovery to: %s (the first non-loopback IPv4 address)", ip)
	}

	all, err := net.Interfaces()
	if err != nil {
		p.logf("listing interfaces failed: %v", err)
		return nil
	}

	var usable []candidate
	for i := range all {
		iface := &all[i]
		addrs, _ := iface.Addrs()
		var shown []string
		var v4 net.IP
		for _, a := range addrs {
			shown = append(shown, a.String())
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil && v4 == nil {
				v4 = ipnet.IP.To4()
			}
		}
		p.logf("interface %-10s flags=%-40s addrs=%s", iface.Name, iface.Flags, strings.Join(shown, ", "))

		up := iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagRunning != 0
		multicast := iface.Flags&net.FlagMulticast != 0
		loopback := iface.Flags&net.FlagLoopback != 0
		if up && multicast && !loopback && v4 != nil {
			usable = append(usable, candidate{name: iface.Name, ip: v4, net: iface})
		}
	}

	var names []string
	for _, c := range usable {
		names = append(names, fmt.Sprintf("%s (%s)", c.name, c.ip))
	}
	p.logf("looking on %d interface(s): %s", len(usable), strings.Join(names, ", "))
	return usable
}

// viaDiscover runs the client's own discovery, unchanged.
func (p *probe) viaDiscover(ctx context.Context, out chan<- sighting) {
	found, err := sonos.Discover(ctx)
	if err != nil {
		p.logf("discover: could not start: %v", err)
		return
	}
	for s := range found {
		out <- sighting{way: "discover", ip: s.IP, location: s.Location, detail: "usn=" + s.USN, sonos: true}
	}
}

// viaSSDP searches from one interface, sending the M-SEARCH again every
// ssdpResend, and reports every answer. A repeated send is the cheap fix the
// comparison asks about: whether answers are lost rather than slow.
func (p *probe) viaSSDP(ctx context.Context, iface candidate, out chan<- sighting) {
	way := "ssdp " + iface.name
	conn, err := net.ListenPacket("udp4", iface.ip.String()+":0")
	if err != nil {
		p.logf("%s: could not open a socket on %s: %v", way, iface.ip, err)
		return
	}
	dest, _ := net.ResolveUDPAddr("udp4", ssdpAddr)
	request := strings.Join([]string{
		"M-SEARCH * HTTP/1.1", "HOST: " + ssdpAddr, `MAN: "ssdp:discover"`, "MX: 1", "ST: " + ssdpST, "", "",
	}, "\r\n")

	// The reader stops when the socket closes, which the sender does at the end.
	var reader sync.WaitGroup
	reader.Add(1)
	go func() {
		defer reader.Done()
		buf := make([]byte, 4096)
		for {
			n, from, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			response := string(buf[:n])
			out <- sighting{
				way: way, ip: from.(*net.UDPAddr).IP.String(),
				location: header(response, "LOCATION"),
				detail:   "st=" + header(response, "ST") + " server=" + header(response, "SERVER"),
				sonos:    header(response, "ST") == ssdpST,
			}
		}
	}()

	ticker := time.NewTicker(ssdpResend)
	defer ticker.Stop()
	for sent := 1; ; sent++ {
		if _, err := conn.WriteTo([]byte(request), dest); err != nil {
			p.logf("%s: search %d could not be sent: %v", way, sent, err)
		} else {
			p.logf("%s: search %d sent from %s", way, sent, iface.ip)
		}
		select {
		case <-ctx.Done():
			conn.Close()
			reader.Wait()
			return
		case <-ticker.C:
		}
	}
}

// viaMDNS asks for _sonos._tcp on one interface for the whole run. It reports
// what else it overhears too, marked as not Sonos: hashicorp/mdns passes on
// every record set it hears on the shared socket.
func (p *probe) viaMDNS(ctx context.Context, iface candidate, out chan<- sighting) {
	way := "mdns " + iface.name
	deadline, _ := ctx.Deadline()
	entries := make(chan *mdns.ServiceEntry, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range entries {
			ip := "<no ipv4>"
			if e.AddrV4 != nil {
				ip = e.AddrV4.String()
			}
			location := ""
			for _, field := range e.InfoFields {
				if after, ok := strings.CutPrefix(field, "location="); ok {
					location = after
				}
			}
			out <- sighting{
				way: way, ip: ip, location: location,
				detail: fmt.Sprintf("name=%s port=%d", e.Name, e.Port),
				sonos:  strings.HasSuffix(e.Name, mdnsSuffix),
			}
		}
	}()

	err := mdns.QueryContext(ctx, &mdns.QueryParam{
		Service:     mdnsService,
		Domain:      "local",
		Timeout:     time.Until(deadline),
		Interface:   iface.net,
		Entries:     entries,
		DisableIPv6: true,
	})
	close(entries)
	<-done
	if err != nil && ctx.Err() == nil {
		p.logf("%s: query failed: %v", way, err)
	}
}

// record logs a sighting and, for a speaker heard of for the first time,
// starts reading its device description.
func (p *probe) record(s sighting) {
	elapsed := time.Since(p.start)
	if !s.sonos {
		p.logf("%-12s heard %s, not a Sonos speaker: %s", s.way, s.ip, s.detail)
		return
	}

	p.mu.Lock()
	p.answers[s.way]++
	sp, known := p.speakers[s.ip]
	if !known {
		sp = &speaker{ip: s.ip, firstBy: map[string]time.Duration{}}
		p.speakers[s.ip] = sp
	}
	_, seenByWay := sp.firstBy[s.way]
	if !seenByWay {
		sp.firstBy[s.way] = elapsed
	}
	if sp.location == "" && s.location != "" {
		sp.location = s.location
		p.fetches.Add(1)
		go p.describe(sp, s.location)
	}
	p.mu.Unlock()

	switch {
	case !known:
		p.logf("%-12s NEW speaker %s  location=%s  %s", s.way, s.ip, s.location, s.detail)
	case !seenByWay:
		p.logf("%-12s found %s too", s.way, s.ip)
	}
}

// describe reads one speaker's device_description.xml. It has its own
// timeout, so a speaker found at the last moment is still described.
func (p *probe) describe(sp *speaker, location string) {
	defer p.fetches.Done()
	desc, err := sonos.FetchDeviceDescription(context.Background(), p.client, location)

	p.mu.Lock()
	sp.desc, sp.descErr = &desc, err
	p.mu.Unlock()

	if err != nil {
		p.logf("describe     %s FAILED: %v", sp.ip, err)
		return
	}
	p.logf("describe     %s is %q, a %s (%s)", sp.ip, desc.RoomName, desc.ModelName, desc.UDN)
}

// summarise prints each speaker and how long each way took to find it, then
// how many speakers each way found in all.
func (p *probe) summarise() {
	p.mu.Lock()
	defer p.mu.Unlock()

	ips := make([]string, 0, len(p.speakers))
	for ip := range p.speakers {
		ips = append(ips, ip)
	}
	sort.Strings(ips)

	p.logf("SUMMARY: %d speaker(s) found in %v", len(ips), *runFor)
	for _, ip := range ips {
		sp := p.speakers[ip]
		name := "(no description)"
		if sp.descErr != nil {
			name = "(description failed: " + sp.descErr.Error() + ")"
		} else if sp.desc != nil {
			name = fmt.Sprintf("%q %s", sp.desc.RoomName, sp.desc.ModelName)
		}
		p.logf("  %-15s %s", ip, name)
		for _, way := range p.ways {
			if at, ok := sp.firstBy[way]; ok {
				p.logf("      %-14s %6dms", way, at.Milliseconds())
			} else {
				p.logf("      %-14s  never", way)
			}
		}
	}

	p.logf("PER WAY: speakers found, time to first and to last, and answers heard (repeats included)")
	for _, way := range p.ways {
		var count int
		var first, last time.Duration
		for _, sp := range p.speakers {
			at, ok := sp.firstBy[way]
			if !ok {
				continue
			}
			if count == 0 || at < first {
				first = at
			}
			last = max(last, at)
			count++
		}
		if count == 0 {
			p.logf("  %-14s 0 found", way)
			continue
		}
		p.logf("  %-14s %d found, first %dms, last %dms, %d answers",
			way, count, first.Milliseconds(), last.Milliseconds(), p.answers[way])
	}
}

// logf writes one line with the time since the start.
func (p *probe) logf(format string, args ...any) {
	fmt.Fprintf(os.Stdout, "[%7.3fs] %s\n", time.Since(p.start).Seconds(), fmt.Sprintf(format, args...))
}

// header reads one header out of an SSDP response, case-insensitively.
func header(response, name string) string {
	for line := range strings.SplitSeq(response, "\n") {
		key, value, ok := strings.Cut(strings.TrimRight(line, "\r"), ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

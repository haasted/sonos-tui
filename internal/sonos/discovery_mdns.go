package sonos

import (
	"context"
	"log"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hashicorp/mdns"
)

// mdnsService is the DNS-SD service type Sonos speakers advertise. It is a
// separate channel from SSDP, on its own multicast group (224.0.0.251:5353).
const mdnsService = "_sonos._tcp"

// mdnsSuffix ends the name of every genuine answer. The library passes on
// every record set it overhears on the shared socket, so answers for other
// services arrive too and must be dropped.
const mdnsSuffix = "." + mdnsService + ".local."

// mdnsLocationField is the TXT field holding device_description.xml's URL,
// the same URL SSDP's LOCATION header gives.
const mdnsLocationField = "location="

// mdnsEntryBuffer is how many answers may wait to be read. The library drops
// an answer it cannot hand over at once, so this is set well above the
// number of speakers in a household rather than at discoverChanBuffer.
const mdnsEntryBuffer = 256

// zonePlayerURN completes a USN built from an mDNS answer, so it has the
// same form as the USN an SSDP answer carries.
const zonePlayerURN = "::urn:schemas-upnp-org:device:ZonePlayer:1"

// queryMDNS asks for Sonos speakers on one interface until ctx ends, sending
// every answer to entries. A variable so a test can answer instead of the
// network.
var queryMDNS = func(ctx context.Context, iface *net.Interface, entries chan<- *mdns.ServiceEntry) error {
	deadline, _ := ctx.Deadline()
	return mdns.QueryContext(ctx, &mdns.QueryParam{
		Service:     mdnsService,
		Domain:      "local",
		Timeout:     time.Until(deadline),
		Interface:   iface,
		Entries:     entries,
		DisableIPv6: true, // the rest of discovery is IPv4 only
		Logger:      mdnsLogger(),
	})
}

// mdnsLogger sends the library's messages, all of them errors, to the
// program's own log. Its default would write to stderr, over the terminal UI.
// Built per query, so it uses the handler the program set up at startup.
func mdnsLogger() *log.Logger {
	return slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn)
}

// searchMDNS asks for Sonos speakers on iface until ctx ends, and reports
// every answer that is one.
func searchMDNS(ctx context.Context, iface searchInterface, out chan<- sighting) {
	via := "mdns " + iface.name
	entries := make(chan *mdns.ServiceEntry, mdnsEntryBuffer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for entry := range entries {
			if speaker, ok := speakerFromMDNS(entry); ok {
				out <- sighting{speaker: speaker, via: via}
			}
		}
	}()

	err := queryMDNS(ctx, iface.iface, entries)
	close(entries)
	<-done
	if err != nil && ctx.Err() == nil {
		slog.Warn("mdns search failed", "via", via, "err", err)
	}
}

// speakerFromMDNS reads a speaker out of an mDNS answer, or reports false for
// an answer that is not a Sonos speaker. The instance name starts with the
// RINCON id ("RINCON_xxx@Room"), which is all the identity a USN needs.
func speakerFromMDNS(entry *mdns.ServiceEntry) (Speaker, bool) {
	if !strings.HasSuffix(entry.Name, mdnsSuffix) || entry.AddrV4 == nil {
		return Speaker{}, false
	}
	id, _, _ := strings.Cut(entry.Name, "@")
	id = strings.TrimSuffix(id, `\`) // the library leaves the @ escaped
	if !strings.HasPrefix(id, "RINCON_") {
		return Speaker{}, false
	}

	ip := entry.AddrV4.String()
	location := "http://" + ip + ":1400/xml/device_description.xml"
	for _, field := range entry.InfoFields {
		if value, ok := strings.CutPrefix(field, mdnsLocationField); ok && value != "" {
			location = value
		}
	}
	return Speaker{
		IP:          ip,
		Location:    location,
		USN:         "uuid:" + id + zonePlayerURN,
		description: &atomic.Pointer[DeviceDescription]{},
	}, true
}

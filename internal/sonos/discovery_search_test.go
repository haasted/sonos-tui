package sonos

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/mdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: an answer as Køkken sent it on 2026-11-02, behind a VPN. It must give
// the address, the description URL from the TXT record, and a USN of the form
// SSDP gives, so the speaker has one identity whichever search found it.
func TestSpeakerFromMDNS_ReadsARealAnswer(t *testing.T) {
	entry := &mdns.ServiceEntry{
		Name:       `RINCON_804AF293EC4C01400\@K\195\184kken._sonos._tcp.local.`,
		AddrV4:     net.ParseIP("192.168.68.51"),
		Port:       1443,
		InfoFields: []string{"info=/api/v1/players/RINCON_804AF293EC4C01400/info", "location=http://192.168.68.51:1400/xml/device_description.xml"},
	}

	speaker, ok := speakerFromMDNS(entry)

	require.True(t, ok)
	assert.Equal(t, "192.168.68.51", speaker.IP)
	assert.Equal(t, "http://192.168.68.51:1400/xml/device_description.xml", speaker.Location)
	assert.Equal(t, "RINCON_804AF293EC4C01400", speaker.ID())
	assert.Equal(t, "uuid:RINCON_804AF293EC4C01400::urn:schemas-upnp-org:device:ZonePlayer:1", speaker.USN)
}

// Case: an answer with no location in its TXT record. The description is
// always served at the same place, so the URL is made from the address.
func TestSpeakerFromMDNS_MakesTheLocationWhenTheAnswerHasNone(t *testing.T) {
	speaker, ok := speakerFromMDNS(&mdns.ServiceEntry{
		Name:   `RINCON_74CA6032755801400\@Ovenp\195\165._sonos._tcp.local.`,
		AddrV4: net.ParseIP("192.168.68.62"),
	})

	require.True(t, ok)
	assert.Equal(t, "http://192.168.68.62:1400/xml/device_description.xml", speaker.Location)
}

// Case: answers that are not a usable Sonos speaker. The library passes on
// everything it overhears, as the probe saw with a sleep proxy and a photo
// frame on the same network.
func TestSpeakerFromMDNS_DropsWhatIsNotASonosSpeaker(t *testing.T) {
	cases := map[string]*mdns.ServiceEntry{
		"another service": {Name: `70-35-60-63\.1\ Dagligstue._sleep-proxy._udp.local.`, AddrV4: net.ParseIP("192.168.68.60")},
		"no ipv4 address": {Name: `RINCON_804AF293EC4C01400\@K\195\184kken._sonos._tcp.local.`},
		"no RINCON id":    {Name: `Something\@Room._sonos._tcp.local.`, AddrV4: net.ParseIP("192.168.68.70")},
	}
	for name, entry := range cases {
		_, ok := speakerFromMDNS(entry)
		assert.False(t, ok, name)
	}
}

// Case: one speaker reported by several searches, and a second speaker. Each
// must come out once, the first sighting winning, whatever the address.
func TestFirstSightings_PassesEachSpeakerOnce(t *testing.T) {
	in := make(chan sighting, 4)
	in <- sighting{via: "mdns en0", speaker: testSpeaker("192.168.68.51", "RINCON_A")}
	in <- sighting{via: "ssdp en0", speaker: testSpeaker("192.168.68.51", "RINCON_A")}
	in <- sighting{via: "ssdp en1", speaker: testSpeaker("10.0.0.51", "RINCON_A")}
	in <- sighting{via: "ssdp en0", speaker: testSpeaker("192.168.68.55", "RINCON_B")}
	close(in)

	out := make(chan Speaker, 4)
	firstSightings(context.Background(), in, out)

	var got []string
	for speaker := range out {
		got = append(got, speaker.ID()+" "+speaker.IP)
	}
	assert.Equal(t, []string{"RINCON_A 192.168.68.51", "RINCON_B 192.168.68.55"}, got)
}

// Case: the interfaces of the Mac behind a VPN, as the probe listed them.
// Only en0 has an IPv4 address and can multicast; the tunnels, the loopback
// and the down or addressless adapters must all be left out.
func TestUsableInterfaces_KeepsOnlyWhatCanReachSpeakers(t *testing.T) {
	up := net.FlagUp | net.FlagRunning | net.FlagMulticast
	all := []net.Interface{
		{Name: "lo0", Flags: up | net.FlagLoopback},
		{Name: "en0", Flags: up | net.FlagBroadcast},
		{Name: "en5", Flags: up | net.FlagBroadcast},
		{Name: "utun0", Flags: up | net.FlagPointToPoint},
		{Name: "ap1", Flags: net.FlagBroadcast | net.FlagMulticast},
		{Name: "broken", Flags: up},
	}
	addrs := map[string][]net.Addr{
		"lo0":    {ipNet("127.0.0.1/8")},
		"en0":    {ipNet("fe80::1010:9b01:3528:7023/64"), ipNet("192.168.68.65/22")},
		"utun0":  {ipNet("fe80::9f45:29eb:2f08:459f/64")},
		"ap1":    {ipNet("192.168.2.1/24")},
		"broken": nil,
	}

	got := usableInterfaces(all, func(iface net.Interface) ([]net.Addr, error) {
		if iface.Name == "broken" {
			return nil, errors.New("no addresses")
		}
		return addrs[iface.Name], nil
	})

	require.Len(t, got, 1)
	assert.Equal(t, "en0", got[0].name)
	assert.Equal(t, "192.168.68.65", got[0].ip.String())
}

// Case: discovery cancelled early, with a library that, as hashicorp/mdns
// does, keeps its query running until its own timeout. The query must still
// return with ctx, and pass on what was found before it.
func TestQueryMDNS_ReturnsWhenCancelled(t *testing.T) {
	restore := mdnsQuery
	t.Cleanup(func() { mdnsQuery = restore })
	mdnsQuery = func(_ context.Context, params *mdns.QueryParam) error {
		params.Entries <- &mdns.ServiceEntry{Name: "first"}
		time.Sleep(params.Timeout) // deaf to ctx, as the real one is
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	entries := make(chan *mdns.ServiceEntry, mdnsEntryBuffer)
	time.AfterFunc(100*time.Millisecond, cancel)

	start := time.Now()
	err := queryMDNS(ctx, nil, entries)

	assert.Less(t, time.Since(start), time.Second, "the query waited out the library's timeout")
	assert.ErrorIs(t, err, context.Canceled)
	require.Len(t, entries, 1)
	assert.Equal(t, "first", (<-entries).Name)
}

// Case: the library ending on its own. Every answer it sent must arrive, and
// its error be returned.
func TestQueryMDNS_PassesOnEverythingWhenTheLibraryEnds(t *testing.T) {
	restore := mdnsQuery
	t.Cleanup(func() { mdnsQuery = restore })
	failed := errors.New("no route to host")
	mdnsQuery = func(_ context.Context, params *mdns.QueryParam) error {
		params.Entries <- &mdns.ServiceEntry{Name: "first"}
		params.Entries <- &mdns.ServiceEntry{Name: "second"}
		return failed
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entries := make(chan *mdns.ServiceEntry, mdnsEntryBuffer)

	assert.ErrorIs(t, queryMDNS(ctx, nil, entries), failed)
	assert.Len(t, entries, 2)
}

// Case: an mDNS answer on a network where SSDP gets none, as behind the VPN.
// Discover must still deliver the speaker.
func TestDiscover_FindsASpeakerByMDNSAlone(t *testing.T) {
	restore := queryMDNS
	t.Cleanup(func() { queryMDNS = restore })
	queryMDNS = func(ctx context.Context, _ *net.Interface, entries chan<- *mdns.ServiceEntry) error {
		entries <- &mdns.ServiceEntry{
			Name:   `RINCON_TESTMDNS01400\@Test._sonos._tcp.local.`,
			AddrV4: net.ParseIP("192.0.2.10"), // a documentation address: nothing answers its description
		}
		<-ctx.Done()
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	found, err := Discover(ctx)
	require.NoError(t, err)

	var ids []string
	for speaker := range found {
		ids = append(ids, speaker.ID())
	}
	assert.Contains(t, ids, "RINCON_TESTMDNS01400")
}

// Case: an error the mDNS library reports. It must land in the program's own
// log as a warning, not on stderr, where it would write over the terminal UI.
func TestMDNSLogger_WritesToTheProgramsLog(t *testing.T) {
	restore := slog.Default()
	t.Cleanup(func() { slog.SetDefault(restore) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

	mdnsLogger().Printf("[ERR] mdns: Failed to read packet: %v", errors.New("boom"))

	assert.Contains(t, buf.String(), "level=WARN")
	assert.Contains(t, buf.String(), "Failed to read packet: boom")
}

func testSpeaker(ip, id string) Speaker {
	return Speaker{IP: ip, USN: "uuid:" + id + zonePlayerURN}
}

func ipNet(cidr string) net.Addr {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	network.IP = ip
	return network
}

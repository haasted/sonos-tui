// Package mdns_test evaluates github.com/hashicorp/mdns (_sonos._tcp) as a
// supplement or replacement for the hand-rolled SSDP search in
// internal/sonos/discovery.go. It has no connection to production code —
// nothing here is imported by anything outside this package. Requires a live
// network with real Sonos speakers, gated behind the -live flag so
// `go test ./...` stays green without one.
//
// FINDING, from a live run against six real speakers: the _sonos._tcp TXT
// record carries a location field with the same device_description.xml URL
// SSDP's LOCATION header gives, but no room name or model directly. So the
// HTTP fetch stays mandatory regardless of which discovery mechanism finds
// the speaker; mDNS cannot skip it. mDNS reached its first speaker faster
// in this run (single-digit ms vs tens of ms for SSDP), but hashicorp/mdns
// also reports every service it overhears on the shared multicast socket
// during the query window, not only the one asked for — a stray _raop._tcp
// (AirPlay) responder's entry showed up in a _sonos._tcp query on this LAN.
// Callers must filter results by service name themselves; see
// sonosMDNSSuffix below.
package mdns_test

import (
	"context"
	"flag"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/hashicorp/mdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// live gates tests that require a real network with real Sonos speakers.
// Run with: cd experiments && go test ./mdns/... -live -v (or make test-live).
var live = flag.Bool("live", false, "run tests against real network devices")

// sonosMDNSService is the DNS-SD service type Sonos speakers advertise
// under, confirmed via `dns-sd -B _sonos._tcp local` and Home Assistant's
// own discovery list. A separate advertisement channel from ssdpST in
// internal/sonos/discovery.go.
const sonosMDNSService = "_sonos._tcp"

// mdnsQueryTimeout bounds one mdns.QueryContext call. mDNS answers trickle
// in over the LAN's multicast group, so the call must wait out a fixed
// window rather than return on the first response.
const mdnsQueryTimeout = 4 * time.Second

// sonosMDNSSuffix is what a genuine _sonos._tcp answer's Name ends with.
// hashicorp/mdns listens on the shared multicast socket and reports every
// complete record set it overhears during the query window, not only
// answers to the service asked for — filtering by this suffix is required,
// confirmed live when an unrelated _raop._tcp (AirPlay) responder's entry
// otherwise leaked into a _sonos._tcp query.
const sonosMDNSSuffix = "." + sonosMDNSService + ".local."

// TestHashicorpMDNS_ListSpeakers_Integration lists all Sonos speakers on the network via hashicorp/mdns.
// Run with: make test-live (or go test ./mdns/... -live -run HashicorpMDNS -v)
func TestHashicorpMDNS_ListSpeakers_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if !*live {
		t.Skip("skipping live test; run with -live to include it")
	}

	entries := make(chan *mdns.ServiceEntry, 16)
	var found []*mdns.ServiceEntry
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range entries {
			if !strings.HasSuffix(e.Name, sonosMDNSSuffix) {
				continue
			}
			found = append(found, e)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), mdnsQueryTimeout)
	defer cancel()

	start := time.Now()
	err := mdns.QueryContext(ctx, &mdns.QueryParam{
		Service:     sonosMDNSService,
		Domain:      "local",
		Timeout:     mdnsQueryTimeout,
		Entries:     entries,
		DisableIPv6: true, // matches sonos.Discover(), which is udp4-only
	})
	close(entries)
	<-done
	t.Logf("mdns.QueryContext returned %d entr(ies) after %v", len(found), time.Since(start))
	require.NoError(t, err)
	require.NotEmpty(t, found, "expected at least one Sonos speaker on the network")

	for _, e := range found {
		require.NotNil(t, e.AddrV4, "a _sonos._tcp entry should resolve to an IPv4 address")
		assert.NotZero(t, e.Port)
		t.Logf("Found: %s  %s:%d  info=%q  fields=%v", e.Name, e.AddrV4, e.Port, e.Info, e.InfoFields)
	}
}

// TestHashicorpMDNS_MatchesHandRolledDiscover_Integration compares hashicorp/mdns against sonos.Discover() on the
// same network. The two lookups are independent (SSDP multicast vs mDNS query, separate sockets), so they run
// concurrently via goroutines rather than back-to-back; a WaitGroup syncs both before either result is read. This
// measures discovery-phase-only timing: sonos.Discover() also kicks off a background device_description.xml fetch
// per speaker, which this test does not wait for.
func TestHashicorpMDNS_MatchesHandRolledDiscover_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if !*live {
		t.Skip("skipping live test; run with -live to include it")
	}

	ctx, cancel := context.WithTimeout(context.Background(), mdnsQueryTimeout)
	defer cancel()

	start := time.Now()

	var (
		wg         sync.WaitGroup
		handRolled []sonos.Speaker
		mdnsFound  []*mdns.ServiceEntry
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		ch, err := sonos.Discover(ctx)
		require.NoError(t, err)
		for s := range ch {
			handRolled = append(handRolled, s)
			t.Logf("sonos.Discover found after %v: %s  %s", time.Since(start), s.IP, s.USN)
		}
	}()

	go func() {
		defer wg.Done()
		entries := make(chan *mdns.ServiceEntry, 16)
		collectDone := make(chan struct{})
		go func() {
			defer close(collectDone)
			for e := range entries {
				if !strings.HasSuffix(e.Name, sonosMDNSSuffix) {
					t.Logf("ignoring non-Sonos entry overheard during the query: %s", e.Name)
					continue
				}
				mdnsFound = append(mdnsFound, e)
				t.Logf("mdns found after %v: %s  %s:%d", time.Since(start), e.Name, e.AddrV4, e.Port)
			}
		}()
		err := mdns.QueryContext(ctx, &mdns.QueryParam{
			Service:     sonosMDNSService,
			Domain:      "local",
			Timeout:     mdnsQueryTimeout,
			Entries:     entries,
			DisableIPv6: true, // matches sonos.Discover(), which is udp4-only
		})
		close(entries)
		<-collectDone
		require.NoError(t, err)
	}()
	wg.Wait()

	// mDNS entries carry an IP, not a USN, so identity is compared by address
	// rather than by sonos.Speaker.USN.
	handRolledIPs := make(map[string]bool, len(handRolled))
	for _, s := range handRolled {
		handRolledIPs[s.IP] = true
	}
	for _, e := range mdnsFound {
		ip := addrString(e)
		assert.True(t, handRolledIPs[ip], "hashicorp/mdns found a speaker (%s) that sonos.Discover() missed", ip)
	}
	assert.Len(t, mdnsFound, len(handRolled), "expected both mechanisms to find the same number of speakers")

	// The open question this experiment exists to answer: does the TXT
	// record carry room/model metadata equivalent to
	// device_description.xml, or does a full pipeline need the HTTP fetch
	// regardless of discovery mechanism? Logged rather than asserted — see
	// this package's doc comment and the beads task for how to read it.
	for _, e := range mdnsFound {
		t.Logf("TXT metadata for %s: info=%q fields=%v", addrString(e), e.Info, e.InfoFields)
	}
}

// addrString renders a ServiceEntry's resolved IPv4 address, or "<none>" for
// an entry mdns returned without one — should not happen for a complete
// entry, but t.Logf/assert callers need a string either way.
func addrString(e *mdns.ServiceEntry) string {
	if e.AddrV4 == nil {
		return "<none>"
	}
	return e.AddrV4.String()
}

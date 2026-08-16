package sonos_test

// Evaluates github.com/koron/go-ssdp as a replacement for the hand-rolled
// SSDP search in discovery.go (sonos-5e1). Requires a live network with
// real Sonos speakers — mirrors TestDiscover_Integration's skip convention
// so `go test ./...` stays green without one.

import (
	"context"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/koron/go-ssdp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sonosZonePlayerST is the SSDP search target for Sonos speakers, same as discovery.go's ssdpST.
const sonosZonePlayerST = "urn:schemas-upnp-org:device:ZonePlayer:1"

// TestKoronSSDP_ListSpeakers_Integration lists all Sonos speakers on the network via koron/go-ssdp.
// Run with: go test ./internal/sonos/... -run KoronSSDP -v (after removing the t.Skip below).
func TestKoronSSDP_ListSpeakers_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Skip("skipping by default — remove this line to run against a real network")

	start := time.Now()
	services, err := ssdp.Search(sonosZonePlayerST, 4, "")
	t.Logf("ssdp.Search returned %d service(s) after %v (blocking call — no per-response timing available)", len(services), time.Since(start))
	require.NoError(t, err)
	require.NotEmpty(t, services, "expected at least one Sonos speaker on the network")

	for _, s := range services {
		u, err := url.Parse(s.Location)
		require.NoError(t, err, "LOCATION should be a valid URL")
		assert.NotEmpty(t, u.Hostname())
		assert.Contains(t, s.Location, ":1400", "LOCATION must reference port 1400")
		assert.Contains(t, s.USN, "RINCON_")
		t.Logf("Found: %s  %s  %s", u.Hostname(), s.Location, s.USN)
	}
}

// TestKoronSSDP_MatchesHandRolledDiscover_Integration compares koron/go-ssdp against sonos.Discover() on the same network.
// The two lookups are independent (separate UDP sockets, separate multicast sends), so they run concurrently via
// goroutines rather than back-to-back; a WaitGroup syncs both before either result is read.
func TestKoronSSDP_MatchesHandRolledDiscover_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Skip("skipping by default — remove this line to run against a real network")

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	start := time.Now()

	var (
		wg         sync.WaitGroup
		handRolled []sonos.Speaker
		services   []ssdp.Service
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
		var err error
		services, err = ssdp.Search(sonosZonePlayerST, 4, "")
		require.NoError(t, err)
		t.Logf("ssdp.Search returned %d service(s) after %v (blocking call — no per-response timing available)", len(services), time.Since(start))
	}()
	wg.Wait()

	found := make(map[string]bool, len(handRolled))
	for _, s := range handRolled {
		found[s.USN] = true
	}
	for _, s := range services {
		assert.True(t, found[s.USN], "koron/go-ssdp found a speaker (%s) that sonos.Discover() missed", s.USN)
	}
	assert.Len(t, services, len(handRolled), "expected both implementations to find the same number of speakers")
}

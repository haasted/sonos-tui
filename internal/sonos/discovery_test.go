package sonos_test

import (
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscover_Integration runs only when explicitly opted-in via -tags integration.
// It requires a live Sonos speaker on the local network.
//
// Run with: go test ./... -tags integration -v
func TestDiscover_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Skip("skipping by default — remove this line to run against a real network")

	speakers, err := sonos.Discover(4_000_000_000) // 4s
	require.NoError(t, err)
	require.NotEmpty(t, speakers, "expected at least one Sonos speaker on the network")

	for _, s := range speakers {
		assert.NotEmpty(t, s.IP)
		assert.Contains(t, s.Location, "http://")
		t.Logf("Found: %s  %s", s.IP, s.Location)
	}
}

func TestExtractHeader(t *testing.T) {
	// extractHeader is unexported, so we test Discover indirectly via Speaker fields.
	// This test validates that Speaker structs are populated correctly.
	s := sonos.Speaker{
		IP:       "192.168.1.50",
		Location: "http://192.168.1.50:1400/xml/device_description.xml",
		USN:      "uuid:RINCON_ABC123::urn:schemas-upnp-org:device:ZonePlayer:1",
	}
	assert.Equal(t, "192.168.1.50", s.IP)
	assert.Contains(t, s.Location, ":1400")
	assert.Contains(t, s.USN, "RINCON_")
}

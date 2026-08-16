package sonos_test

import (
	"testing"
	"time"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realSSDP is a verbatim SSDP response as returned by a Sonos Era 100 over UDP.
// The LOCATION field is the critical one — it must contain port 1400.
const realSSDPResponse = "HTTP/1.1 200 OK\r\n" +
	"CACHE-CONTROL: max-age=1800\r\n" +
	"EXT:\r\n" +
	"LOCATION: http://192.168.68.56:1400/xml/device_description.xml\r\n" +
	"SERVER: Linux UPnP/1.0 Sonos/80.1-60010 (S57)\r\n" +
	"ST: urn:schemas-upnp-org:device:ZonePlayer:1\r\n" +
	"USN: uuid:RINCON_804AF293EC4C01400::urn:schemas-upnp-org:device:ZonePlayer:1\r\n" +
	"\r\n"

func TestExtractHeader_Location_ContainsPort1400(t *testing.T) {
	location := sonos.ExtractHeader(realSSDPResponse, "LOCATION")
	require.NotEmpty(t, location, "LOCATION header should not be empty")
	assert.Contains(t, location, ":1400", "LOCATION must reference port 1400")
	assert.Equal(t, "http://192.168.68.56:1400/xml/device_description.xml", location)
}

func TestExtractHeader_USN(t *testing.T) {
	usn := sonos.ExtractHeader(realSSDPResponse, "USN")
	require.NotEmpty(t, usn)
	assert.Contains(t, usn, "RINCON_")
}

func TestExtractHeader_CaseInsensitive(t *testing.T) {
	// Headers in SSDP responses are case-insensitive per HTTP spec.
	assert.Equal(t,
		sonos.ExtractHeader(realSSDPResponse, "location"),
		sonos.ExtractHeader(realSSDPResponse, "LOCATION"),
	)
}

func TestExtractHeader_MissingHeader(t *testing.T) {
	result := sonos.ExtractHeader(realSSDPResponse, "X-Does-Not-Exist")
	assert.Empty(t, result)
}

// TestDiscover_Integration requires a live Sonos speaker on the local network.
// Run with: go test ./... -run Integration -v
func TestDiscover_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Skip("skipping by default — remove this line to run against a real network")

	speakers, err := sonos.Discover(4 * time.Second)
	require.NoError(t, err)
	require.NotEmpty(t, speakers, "expected at least one Sonos speaker on the network")

	for _, s := range speakers {
		assert.NotEmpty(t, s.IP)
		assert.Contains(t, s.Location, "http://")
		assert.Contains(t, s.Location, ":1400", "LOCATION must reference port 1400")
		t.Logf("Found: %s  %s", s.IP, s.Location)
	}
}


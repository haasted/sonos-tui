package sonos_test

import (
	"context"
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
	// t.Skip("skipping by default — remove this line to run against a real network")

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	start := time.Now()
	ch, err := sonos.Discover(ctx)
	require.NoError(t, err)

	var speakers []sonos.Speaker
	for s := range ch {
		speakers = append(speakers, s)
		t.Logf("Found after %v: %s  %s", time.Since(start), s.IP, s.Location)
	}
	require.NotEmpty(t, speakers, "expected at least one Sonos speaker on the network")

	for _, s := range speakers {
		assert.NotEmpty(t, s.IP)
		assert.Contains(t, s.Location, "http://")
		assert.Contains(t, s.Location, ":1400", "LOCATION must reference port 1400")
	}
}

// TestDiscover_RequiresDeadline confirms Discover rejects an undeadlined context rather than blocking forever.
func TestDiscover_RequiresDeadline(t *testing.T) {
	_, err := sonos.Discover(context.Background())
	assert.Error(t, err)
}

// TestDiscover_EarlyCancellation confirms canceling ctx interrupts the blocking read well before its deadline.
func TestDiscover_EarlyCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	ch, err := sonos.Discover(ctx)
	require.NoError(t, err)

	for range ch {
	}
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 2*time.Second, "expected cancellation to interrupt the read well before the 5s deadline")
}

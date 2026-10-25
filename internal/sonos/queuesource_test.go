package sonos_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: a queue loaded from a playlist, captured from a real speaker. The
// queue is the speaker's own, but it says what was loaded into it.
func TestQueueSource_APlaylistNamesTheQueue(t *testing.T) {
	event, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_queue_playlist.xml"))
	require.NoError(t, err)

	assert.True(t, event.HasQueueSource)
	assert.Equal(t, "Awesome singles", event.QueueSource)
}

// Case: a queue loaded from an album. A container too, so it names the queue.
func TestQueueSource_AnAlbumNamesTheQueue(t *testing.T) {
	event, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_queue_album.xml"))
	require.NoError(t, err)

	assert.True(t, event.HasQueueSource)
	assert.Equal(t, "Ordensduks i Ingenmandsland", event.QueueSource)
}

// Case: live radio. The speaker reports the station as what was last
// started, and a station is not what the queue holds.
func TestQueueSource_AStationDoesNotNameTheQueue(t *testing.T) {
	event, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_broadcast_radio.xml"))
	require.NoError(t, err)

	assert.False(t, event.HasQueueSource, "a station was taken for the queue's name: %q", event.QueueSource)
}

// Case: a station started after a playlist was loaded. The queue still holds
// the playlist's tracks, so it keeps the playlist's name.
func TestQueueSource_SurvivesAStationStartedAfterIt(t *testing.T) {
	device := replayDevice(t)

	applyEvent(t, device, fixture(t, "avtransport_queue_playlist.xml"))
	applyEvent(t, device, fixture(t, "avtransport_broadcast_radio.xml"))

	assert.Equal(t, "Awesome singles", device.State().QueueSource)
}

// Case: a second playlist loaded over the first. The newer name wins.
func TestQueueSource_AnotherPlaylistReplacesIt(t *testing.T) {
	device := replayDevice(t)

	applyEvent(t, device, fixture(t, "avtransport_queue_playlist.xml"))
	applyEvent(t, device, fixture(t, "avtransport_queue_album.xml"))

	assert.Equal(t, "Ordensduks i Ingenmandsland", device.State().QueueSource)
}

// replayDevice handles real captures, which carry no position: the handler
// asks the speaker for one, and this speaker refuses, as an unreachable one
// would. The handler logs that and carries on.
func replayDevice(t *testing.T) *sonos.Device {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no position here", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	return sonos.NewDeviceWithClient(sonos.Speaker{IP: "192.168.1.10"},
		&sonos.SonosClient{BaseURL: server.URL, HTTPClient: server.Client()})
}

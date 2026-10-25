package sonos_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: a playlist played in order. The queue is emptied, filled, selected,
// set to play in order and started, in that sequence.
func TestDevice_ReplaceQueue_SendsTheWholeSequence(t *testing.T) {
	srv := newRecordingServer(t, "")
	d := newQueueDevice(t, srv.Server)

	require.NoError(t, d.ReplaceQueue(context.Background(), awesomeSingles, sonos.PlayModeNormal))

	assert.Equal(t, []string{
		"RemoveAllTracksFromQueue", "AddURIToQueue", "SetAVTransportURI", "SetPlayMode", "Play",
	}, srv.actions())
	assert.Contains(t, srv.body("AddURIToQueue"), "00060000libraryplaylist%3ap.V7VYpk7FYWLeW")
	assert.Contains(t, srv.body("SetAVTransportURI"), "x-rincon-queue:RINCON_TEST01400#0")
	assert.Contains(t, srv.body("SetPlayMode"), "<NewPlayMode>NORMAL</NewPlayMode>")
	assert.Equal(t, "PLAYING", d.State().Transport.State)
}

// Case: a shuffled playlist, which must set the shuffle mode.
func TestDevice_ReplaceQueue_SetsShuffle(t *testing.T) {
	srv := newRecordingServer(t, "")
	d := newQueueDevice(t, srv.Server)

	require.NoError(t, d.ReplaceQueue(context.Background(), awesomeSingles, sonos.PlayModeShuffleNoRepeat))

	assert.Contains(t, srv.body("SetPlayMode"), "<NewPlayMode>SHUFFLE_NOREPEAT</NewPlayMode>")
}

// Case: a speaker that refuses the playlist. Nothing after the refusal may be
// sent, so an emptied queue is not then started.
func TestDevice_ReplaceQueue_StopsAtARefusal(t *testing.T) {
	srv := newRecordingServer(t, "AddURIToQueue")
	d := newQueueDevice(t, srv.Server)

	err := d.ReplaceQueue(context.Background(), awesomeSingles, sonos.PlayModeNormal)

	require.Error(t, err)
	assert.Equal(t, []string{"RemoveAllTracksFromQueue", "AddURIToQueue"}, srv.actions())
	assert.Empty(t, d.State().Transport.State)
}

// recordingServer fakes AVTransport, keeping each action and its body. The
// action named in refuse answers with a server error.
type recordingServer struct {
	*httptest.Server

	mu     sync.Mutex
	names  []string
	bodies map[string]string
}

func newRecordingServer(t *testing.T, refuse string) *recordingServer {
	t.Helper()

	rs := &recordingServer{bodies: map[string]string{}}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("SOAPAction")
		action = action[strings.LastIndex(action, "#")+1 : len(action)-1]
		body, _ := io.ReadAll(r.Body)

		rs.mu.Lock()
		rs.names = append(rs.names, action)
		rs.bodies[action] = string(body)
		rs.mu.Unlock()

		if action == refuse {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body/></s:Envelope>`)
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *recordingServer) actions() []string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return append([]string(nil), rs.names...)
}

func (rs *recordingServer) body(action string) string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.bodies[action]
}

// newQueueDevice is a test device with a RINCON id, which selecting the
// queue needs.
func newQueueDevice(t *testing.T, srv *httptest.Server) *sonos.Device {
	t.Helper()
	speaker := sonos.Speaker{
		IP:  "test",
		USN: "uuid:RINCON_TEST01400::urn:schemas-upnp-org:device:ZonePlayer:1",
	}
	client := &sonos.SonosClient{BaseURL: srv.URL, HTTPClient: srv.Client()}
	return sonos.NewDeviceWithClient(speaker, client)
}

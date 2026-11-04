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

	require.NoError(t, d.ReplaceQueue(context.Background(), playableSingles(t), sonos.PlayModeNormal))

	assert.Equal(t, []string{
		"RemoveAllTracksFromQueue", "AddURIToQueue", "SetAVTransportURI", "SetPlayMode", "Seek", "Play",
	}, srv.actions())
	assert.Contains(t, srv.body("Seek"), "<Target>1</Target>", "in order, it must start from the first track")
	assert.Contains(t, srv.body("AddURIToQueue"), "00060000libraryplaylist%3ap.V7VYpk7FYWLeW")
	assert.Contains(t, srv.body("SetAVTransportURI"), "x-rincon-queue:RINCON_TEST01400#0")
	assert.Contains(t, srv.body("SetPlayMode"), "<NewPlayMode>NORMAL</NewPlayMode>")
	assert.Equal(t, "PLAYING", d.State().Transport.State)
}

// Case: a shuffled playlist, which must set the shuffle mode.
func TestDevice_ReplaceQueue_SetsShuffle(t *testing.T) {
	srv := newRecordingServer(t, "")
	d := newQueueDevice(t, srv.Server)

	require.NoError(t, d.ReplaceQueue(context.Background(), playableSingles(t), sonos.PlayModeShuffleNoRepeat))

	assert.Contains(t, srv.body("SetPlayMode"), "<NewPlayMode>SHUFFLE_NOREPEAT</NewPlayMode>")
	assert.NotContains(t, srv.actions(), "Seek", "shuffled, the speaker picks where to start")
}

// Case: a speaker that refuses the playlist. Nothing after the refusal may be
// sent, so an emptied queue is not then started.
func TestDevice_ReplaceQueue_StopsAtARefusal(t *testing.T) {
	srv := newRecordingServer(t, "AddURIToQueue")
	d := newQueueDevice(t, srv.Server)

	err := d.ReplaceQueue(context.Background(), playableSingles(t), sonos.PlayModeNormal)

	require.Error(t, err)
	assert.Equal(t, []string{"RemoveAllTracksFromQueue", "AddURIToQueue"}, srv.actions())
	assert.Empty(t, d.State().Transport.State)
}

// Case: play next while the queue plays track 5. It must land at 6, marked
// as next, and leave playback alone.
func TestDevice_PlayNext_FollowsThePlayingTrack(t *testing.T) {
	srv := newRecordingServer(t, "")
	d := newQueueDevice(t, srv.Server)
	applyEvent(t, d, lastChange(
		`<AVTransportURI val="x-rincon-queue:RINCON_TEST01400#0"/>`, `<CurrentTrack val="5"/>`))

	require.NoError(t, d.PlayNext(context.Background(), playableSingles(t)))

	body := srv.body("AddURIToQueue")
	assert.Contains(t, body, "<DesiredFirstTrackNumberEnqueued>6</DesiredFirstTrackNumberEnqueued>")
	assert.Contains(t, body, "<EnqueueAsNext>1</EnqueueAsNext>")
	assert.NotContains(t, srv.actions(), "Play")
}

// Case: play next while a station plays. There is no playing track in the
// queue to follow, so it must go first.
func TestDevice_PlayNext_GoesFirstWhenTheQueueIsNotPlaying(t *testing.T) {
	srv := newRecordingServer(t, "")
	d := newQueueDevice(t, srv.Server)
	applyEvent(t, d, lastChange(
		`<AVTransportURI val="x-sonosapi-hls:urn%3adr%3aradio%3achannel%3a5fa156d2?sid=290"/>`,
		`<CurrentTrack val="1"/>`))

	require.NoError(t, d.PlayNext(context.Background(), playableSingles(t)))

	assert.Contains(t, srv.body("AddURIToQueue"),
		"<DesiredFirstTrackNumberEnqueued>1</DesiredFirstTrackNumberEnqueued>")
}

// Case: adding to the end, which must ask for the end and nothing else.
func TestDevice_AddToQueue_AppendsAndLeavesPlaybackAlone(t *testing.T) {
	srv := newRecordingServer(t, "")
	d := newQueueDevice(t, srv.Server)

	require.NoError(t, d.AddToQueue(context.Background(), playableSingles(t)))

	assert.Equal(t, []string{"AddURIToQueue"}, srv.actions())
	body := srv.body("AddURIToQueue")
	assert.Contains(t, body, "<DesiredFirstTrackNumberEnqueued>0</DesiredFirstTrackNumberEnqueued>")
	assert.Contains(t, body, "<EnqueueAsNext>0</EnqueueAsNext>")
}

// Case: a station, which is played straight from its URI and DIDL and must
// not touch the queue.
func TestDevice_PlayStation_PointsTheTransportAtIt(t *testing.T) {
	srv := newRecordingServer(t, "")
	d := newQueueDevice(t, srv.Server)
	station := sonos.Playable{
		URI:      "x-sonosapi-hls:urn%3adr%3aradio%3achannel%3a5fa156d1?sid=290&flags=8232&sn=3",
		Metadata: "<DIDL-Lite>DR P1</DIDL-Lite>",
		Title:    "DR P1",
	}

	require.NoError(t, d.PlayStation(context.Background(), station))

	assert.Equal(t, []string{"SetAVTransportURI", "Play"}, srv.actions())
	body := srv.body("SetAVTransportURI")
	assert.Contains(t, body, "<CurrentURI>x-sonosapi-hls:urn%3adr%3aradio%3achannel%3a5fa156d1?sid=290&amp;flags=8232&amp;sn=3</CurrentURI>")
	assert.Contains(t, body, "<CurrentURIMetaData>&lt;DIDL-Lite&gt;DR P1&lt;/DIDL-Lite&gt;</CurrentURIMetaData>")
	assert.Equal(t, "PLAYING", d.State().Transport.State)
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

func playableSingles(t *testing.T) sonos.Playable {
	t.Helper()
	playable, err := awesomeSingles.Playable()
	require.NoError(t, err)
	return playable
}

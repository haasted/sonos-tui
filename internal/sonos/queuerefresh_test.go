package sonos_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: the read that every speaker gets at startup, which must leave the
// queue where the rest of the program looks for it.
func TestRefreshQueue_StoresTheQueueOnTheDevice(t *testing.T) {
	server, _ := browseServer(t, 5)
	defer server.Close()

	device := deviceFor(server)
	require.False(t, device.State().QueueKnown, "nothing is known before the first read")

	require.NoError(t, device.RefreshQueue())

	state := device.State()
	assert.True(t, state.QueueKnown)
	require.Len(t, state.Queue, 5)
	assert.Equal(t, "Track 1", state.Queue[0].Title)
}

// Case: an empty queue, which a speaker playing radio may well report. It must
// be told apart from a queue that has not been read, and only QueueKnown can
// do that: the length is zero either way, and so is the update id.
func TestRefreshQueue_AnEmptyQueueIsStillAnAnswer(t *testing.T) {
	server, _ := browseServer(t, 0)
	defer server.Close()

	device := deviceFor(server)
	require.NoError(t, device.RefreshQueue())

	state := device.State()
	assert.True(t, state.QueueKnown, "an empty queue was read, not missed")
	assert.Empty(t, state.Queue)
}

// Case: two reads in flight at once. A replacement can announce twice within a
// second while a read takes about a tenth of one, so the slower read must not
// overwrite the newer answer it raced.
func TestRefreshQueue_ASlowReadDoesNotOverwriteANewerOne(t *testing.T) {
	var device *sonos.Device

	// The server starts a second read while the first is still being served,
	// which is the race itself rather than an imitation of it.
	overtaken := false
	server := browseServerFunc(t, func(w http.ResponseWriter, r *http.Request, serve func(int)) {
		if !overtaken {
			overtaken = true
			require.NoError(t, device.RefreshQueue()) // the newer read, start to finish
			serve(5)                                  // then the older one finally answers
			return
		}
		serve(9)
	})
	defer server.Close()
	device = deviceFor(server)

	require.NoError(t, device.RefreshQueue())

	assert.Len(t, device.State().Queue, 9, "the overtaken read must not replace the newer queue")
}

// browseServerFunc serves Browse with a handler of the test's own, which is
// given a serve(n) that writes one page of n items.
func browseServerFunc(t *testing.T, handle func(w http.ResponseWriter, r *http.Request, serve func(int))) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handle(w, r, func(n int) {
			var didl strings.Builder
			didl.WriteString(`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
				`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">`)
			for i := range n {
				fmt.Fprintf(&didl, `<item id="Q:0/%d"><res duration="0:03:21">x-sonos-http:track-%d.mp3</res>`+
					`<dc:title>Track %d</dc:title></item>`, i+1, i+1, i+1)
			}
			didl.WriteString(`</DIDL-Lite>`)
			fmt.Fprint(w, browseResponse(didl.String(), n, n))
		})
	}))
}

func deviceFor(server *httptest.Server) *sonos.Device {
	return sonos.NewDeviceWithClient(
		sonos.Speaker{IP: "127.0.0.1", USN: "uuid:RINCON_TEST"},
		&sonos.SonosClient{BaseURL: server.URL, HTTPClient: server.Client()},
	)
}

package sonos_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: the request itself, which names the track by its container and its
// 1-based position rather than by a row on anyone's screen.
func TestRemoveTrack_AddressesTheTrackByPosition(t *testing.T) {
	body, server := captureRequest(t)
	defer server.Close()

	require.NoError(t, clientFor(server).RemoveTrackFromQueue(23, 7))

	assert.Contains(t, *body, "<ObjectID>Q:0/23</ObjectID>", "the track is named by its place in Q:0")
	assert.Contains(t, *body, "<UpdateID>7</UpdateID>", "the version the caller believes the queue is at")
	assert.Contains(t, *body, "u:RemoveTrackFromQueue", "the action is on AVTransport")
}

// Case: the service it is sent to. The queue is read from ContentDirectory
// under /MediaServer but changed through AVTransport under /MediaRenderer, and
// a removal sent to the reading half would simply not be understood.
func TestRemoveTrack_GoesToTheTransportService(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		io.Copy(io.Discard, r.Body)
		soapOK(w)
	}))
	defer server.Close()

	require.NoError(t, clientFor(server).RemoveTrackFromQueue(1, 0))
	assert.Equal(t, "/MediaRenderer/AVTransport/Control", path)
}

// Case: a position that is not a track. A queue counts from one, so zero is
// the value a caller reaches by forgetting that — and Q:0/0 is a request the
// speaker would answer somehow.
func TestRemoveTrack_RefusesAPositionThatIsNotATrack(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("nothing should reach the speaker")
	}))
	defer server.Close()

	assert.Error(t, clientFor(server).RemoveTrackFromQueue(0, 3))
	assert.Error(t, clientFor(server).RemoveTrackFromQueue(-1, 3))
}

// Case: a speaker that refuses. The error must carry which track, since the
// log is where a refusal is read back later.
func TestRemoveTrack_ReportsARefusal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
			`<s:Body><s:Fault><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0">`+
			`<errorCode>710</errorCode></UPnPError></detail></s:Fault></s:Body></s:Envelope>`)
	}))
	defer server.Close()

	err := clientFor(server).RemoveTrackFromQueue(42, 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "42", "the error must say which track")
}

// captureRequest records the body a command sends and answers it with an empty
// success.
func captureRequest(t *testing.T) (*string, *httptest.Server) {
	t.Helper()

	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		body = string(raw)
		soapOK(w)
	}))
	return &body, server
}

func soapOK(w http.ResponseWriter) {
	io.WriteString(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
		`<s:Body><u:RemoveTrackFromQueueResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"/>`+
		`</s:Body></s:Envelope>`)
}

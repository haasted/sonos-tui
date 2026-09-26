package sonos_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: the request itself, which names a run by where it starts and how long
// it is rather than by a row on anyone's screen.
func TestRemoveTrackRange_AddressesTheRunByPosition(t *testing.T) {
	body, server := captureRequest(t)
	defer server.Close()

	_, err := clientFor(server).RemoveTrackRange(context.Background(), 23, 1, 7)
	require.NoError(t, err)

	assert.Contains(t, *body, "<StartingIndex>23</StartingIndex>", "the run starts at the track's place in the queue")
	assert.Contains(t, *body, "<NumberOfTracks>1</NumberOfTracks>")
	assert.Contains(t, *body, "<UpdateID>7</UpdateID>", "the version the caller believes the queue is at")
	assert.Contains(t, *body, "<QueueID>0</QueueID>", "0 is the speaker's own queue")
	assert.Contains(t, *body, "u:RemoveTrackRange")
}

// Case: a run of several, which is one call rather than one call per track.
// This is what makes removing a selection cost a request per block.
func TestRemoveTrackRange_TakesARunInOneCall(t *testing.T) {
	body, server := captureRequest(t)
	defer server.Close()

	_, err := clientFor(server).RemoveTrackRange(context.Background(), 5, 3, 7)
	require.NoError(t, err)

	assert.Contains(t, *body, "<StartingIndex>5</StartingIndex>")
	assert.Contains(t, *body, "<NumberOfTracks>3</NumberOfTracks>")
}

// Case: the service it is sent to. The queue is read from ContentDirectory
// under /MediaServer and changed through the Queue service under
// /MediaRenderer, which is the only one that answers with a version.
func TestRemoveTrackRange_GoesToTheQueueService(t *testing.T) {
	_, path, server := captureCall(t)
	defer server.Close()

	_, err := clientFor(server).RemoveTrackRange(context.Background(), 1, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, "/MediaRenderer/Queue/Control", *path)
}

// Case: the version the speaker answers with. A caller removing several runs
// carries it into the next call; without it the next call is refused.
func TestRemoveTrackRange_ReturnsTheVersionTheSpeakerMovedTo(t *testing.T) {
	server := soapServer(t, 34)
	defer server.Close()

	version, err := clientFor(server).RemoveTrackRange(context.Background(), 3, 2, 33)
	require.NoError(t, err)
	assert.Equal(t, 34, version)
}

// Case: a position that is not a track. A queue counts from one, so zero is
// the value a caller reaches by forgetting that.
func TestRemoveTrackRange_RefusesAPositionThatIsNotATrack(t *testing.T) {
	server := serverThatMustNotBeCalled(t)
	defer server.Close()

	_, err := clientFor(server).RemoveTrackRange(context.Background(), 0, 1, 3)
	assert.Error(t, err)
	_, err = clientFor(server).RemoveTrackRange(context.Background(), -1, 1, 3)
	assert.Error(t, err)
}

// Case: a run of no tracks, which a caller reaches by computing a length from
// an empty selection. The speaker would take it as some other length.
func TestRemoveTrackRange_RefusesARunOfNoTracks(t *testing.T) {
	server := serverThatMustNotBeCalled(t)
	defer server.Close()

	_, err := clientFor(server).RemoveTrackRange(context.Background(), 3, 0, 3)
	assert.Error(t, err)
}

// Case: a speaker that refuses. The error must carry which tracks, since the
// log is where a refusal is read back later.
func TestRemoveTrackRange_ReportsARefusal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
			`<s:Body><s:Fault><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0">`+
			`<errorCode>1028</errorCode></UPnPError></detail></s:Fault></s:Body></s:Envelope>`)
	}))
	defer server.Close()

	_, err := clientFor(server).RemoveTrackRange(context.Background(), 42, 1, 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "42", "the error must say which tracks")
}

// serverThatMustNotBeCalled fails the test if anything reaches it.
func serverThatMustNotBeCalled(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("nothing should reach the speaker")
	}))
}

// soapServer answers every request with one version, for a test about what a
// caller reads back rather than what it sent.
func soapServer(t *testing.T, version int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		writeSOAP(w, version)
	}))
}

// captureRequest records the body a command sends and answers it with an
// empty success.
func captureRequest(t *testing.T) (*string, *httptest.Server) {
	body, _, server := captureCall(t)
	return body, server
}

// captureCall also records the path, for a test about which service a command
// was sent to.
func captureCall(t *testing.T) (*string, *string, *httptest.Server) {
	t.Helper()

	var body, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		body, path = string(raw), r.URL.Path
		soapOK(w)
	}))
	return &body, &path, server
}

func soapOK(w http.ResponseWriter) { writeSOAP(w, 1) }

// writeSOAP answers as a queue write does, carrying the version the speaker
// moved to. One stand-in serves every action: the response element is named
// for the action it answers, and nothing matches on that name.
func writeSOAP(w http.ResponseWriter, version int) {
	fmt.Fprintf(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
		`<s:Body><u:ReorderTracksResponse xmlns:u="urn:schemas-sonos-com:service:Queue:1">`+
		`<NewUpdateID>%d</NewUpdateID></u:ReorderTracksResponse></s:Body></s:Envelope>`, version)
}

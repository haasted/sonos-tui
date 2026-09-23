package sonos_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a station or a paused speaker, where Seek alone is not certain to be
// audible. Play must follow it regardless of what the transport was doing.
func TestDevice_PlayTrack_SeeksThenPlays(t *testing.T) {
	srv := newAVTransportActionServer(t)
	defer srv.Close()

	d := newTestDevice(t, srv.Server)
	require.NoError(t, d.PlayTrack(context.Background(), 7))

	calls := srv.calls()
	require.Len(t, calls, 2, "both Seek and Play must reach the speaker")
	assert.Equal(t, "Seek", calls[0])
	assert.Equal(t, "Play", calls[1])
	assert.Equal(t, "PLAYING", d.State().Transport.State)
}

// Case: a speaker that refuses the seek. Play must not be sent for a track
// the speaker never agreed to move to, and cached state must stay untouched.
func TestDevice_PlayTrack_SeekFailureLeavesStateUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := newTestDevice(t, srv)
	err := d.PlayTrack(context.Background(), 7)

	require.Error(t, err)
	assert.Empty(t, d.State().Transport.State, "state should stay untouched when the command fails")
}

// avTransportActionServer fakes an AVTransport service that records every
// action it receives, in order, and answers each with an empty success.
type avTransportActionServer struct {
	*httptest.Server

	mu      sync.Mutex
	actions []string
}

func newAVTransportActionServer(t *testing.T) *avTransportActionServer {
	t.Helper()

	as := &avTransportActionServer{}
	as.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("SOAPAction")
		action = action[strings.LastIndex(action, "#")+1 : len(action)-1]

		as.mu.Lock()
		as.actions = append(as.actions, action)
		as.mu.Unlock()

		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body/></s:Envelope>`)
	}))
	return as
}

// calls returns a copy of the SOAP actions received so far.
func (as *avTransportActionServer) calls() []string {
	as.mu.Lock()
	defer as.mu.Unlock()
	return append([]string(nil), as.actions...)
}

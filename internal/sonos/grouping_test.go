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

// Case: joining, which is not a group command at all -- a speaker's
// transport is pointed at another speaker, and that is what a group is.
func TestJoinGroup_PointsTheSpeakerAtTheCoordinator(t *testing.T) {
	body, path, server := captureCall(t)
	defer server.Close()

	require.NoError(t, clientFor(server).JoinGroup(context.Background(), "RINCON_ABC"))

	assert.Contains(t, *body, "u:SetAVTransportURI")
	assert.Contains(t, *body, "<CurrentURI>x-rincon:RINCON_ABC</CurrentURI>")
	assert.Equal(t, "/MediaRenderer/AVTransport/Control", *path)
}

// Case: no coordinator to point at. Sending it would set the speaker
// playing a URI of "x-rincon:", which is not nothing -- it is wrong.
func TestJoinGroup_RefusesAnEmptyCoordinator(t *testing.T) {
	server := serverThatMustNotBeCalled(t)
	defer server.Close()

	assert.Error(t, clientFor(server).JoinGroup(context.Background(), ""))
}

// Case: leaving, which stands the speaker up on its own again. The same
// call whether it was following or leading: a group whose coordinator goes
// promotes another rather than dissolving.
func TestLeaveGroup_StandsTheSpeakerOnItsOwn(t *testing.T) {
	body, path, server := captureCall(t)
	defer server.Close()

	require.NoError(t, clientFor(server).LeaveGroup(context.Background()))

	assert.Contains(t, *body, "u:BecomeCoordinatorOfStandaloneGroup")
	assert.Equal(t, "/MediaRenderer/AVTransport/Control", *path)
}

// Case: a speaker that refuses. The error has to say which way it was
// going, since the log is where a refusal is read back later.
func TestGrouping_ReportsARefusal(t *testing.T) {
	server := refusingServer(t)
	defer server.Close()

	err := clientFor(server).JoinGroup(context.Background(), "RINCON_ABC")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "RINCON_ABC")

	err = clientFor(server).LeaveGroup(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "leaving")
}

// refusingServer answers every command with a SOAP fault.
func refusingServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
			`<s:Body><s:Fault><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0">`+
			`<errorCode>701</errorCode></UPnPError></detail></s:Fault></s:Body></s:Envelope>`)
	}))
}

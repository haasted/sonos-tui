package sonos_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// avTransportServer fakes just enough of a Sonos speaker's AVTransport
// service to drive Device: any action gets an empty success envelope,
// except GetTransportInfo which returns state.
func avTransportServer(t *testing.T, state string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.Header.Get("SOAPAction")
		w.Header().Set("Content-Type", "text/xml")

		if action == `"urn:schemas-upnp-org:service:AVTransport:1#GetTransportInfo"` {
			fmt.Fprintf(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body><u:GetTransportInfoResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
    <CurrentTransportState>%s</CurrentTransportState>
    <CurrentTransportStatus>OK</CurrentTransportStatus>
    <CurrentSpeed>1</CurrentSpeed>
  </u:GetTransportInfoResponse></s:Body>
</s:Envelope>`, state)
			return
		}

		fmt.Fprint(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body/></s:Envelope>`)
	}))
}

func newTestDevice(t *testing.T, srv *httptest.Server) *sonos.Device {
	t.Helper()
	speaker := sonos.Speaker{IP: "test", Location: srv.URL + "/xml/device_description.xml"}
	client := &sonos.SonosClient{BaseURL: srv.URL, HTTPClient: srv.Client()}
	return sonos.NewDeviceWithClient(speaker, client)
}

func TestDevice_Refresh(t *testing.T) {
	srv := avTransportServer(t, "PLAYING")
	defer srv.Close()

	d := newTestDevice(t, srv)
	require.NoError(t, d.Refresh())
	assert.Equal(t, "PLAYING", d.State().Transport.State)
}

func TestDevice_Play_UpdatesStateOptimistically(t *testing.T) {
	srv := avTransportServer(t, "STOPPED")
	defer srv.Close()

	d := newTestDevice(t, srv)
	require.NoError(t, d.Play())

	// State should reflect Play() immediately, without a follow-up Refresh.
	assert.Equal(t, "PLAYING", d.State().Transport.State)
}

func TestDevice_Pause_UpdatesStateOptimistically(t *testing.T) {
	srv := avTransportServer(t, "PLAYING")
	defer srv.Close()

	d := newTestDevice(t, srv)
	require.NoError(t, d.Pause())
	assert.Equal(t, "PAUSED_PLAYBACK", d.State().Transport.State)
}

func TestDevice_Play_Error_LeavesStateUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := newTestDevice(t, srv)
	err := d.Play()

	require.Error(t, err)
	assert.Empty(t, d.State().Transport.State, "state should stay untouched when the command fails")
}

func TestDevice_RefreshTrack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body><u:GetPositionInfoResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
    <Track>1</Track>
    <TrackDuration>0:03:30</TrackDuration>
    <TrackMetaData>&lt;DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"&gt;&lt;item&gt;&lt;dc:title&gt;Test Track&lt;/dc:title&gt;&lt;dc:creator&gt;Test Artist&lt;/dc:creator&gt;&lt;upnp:album&gt;Test Album&lt;/upnp:album&gt;&lt;/item&gt;&lt;/DIDL-Lite&gt;</TrackMetaData>
    <TrackURI>x-sonos-http:test</TrackURI>
    <RelTime>0:01:15</RelTime>
  </u:GetPositionInfoResponse></s:Body>
</s:Envelope>`)
	}))
	defer srv.Close()

	d := newTestDevice(t, srv)
	require.NoError(t, d.RefreshTrack())

	state := d.State()
	assert.Equal(t, "Test Track", state.Current.Title)
	assert.Equal(t, "Test Artist", state.Current.Artist)
	assert.Equal(t, "Test Album", state.Current.Album)
	assert.Equal(t, "0:03:30", state.Current.Duration)
	assert.Equal(t, "0:01:15", state.RelTime)
}

func TestDevice_State_ReturnsCopy(t *testing.T) {
	srv := avTransportServer(t, "PLAYING")
	defer srv.Close()

	d := newTestDevice(t, srv)
	require.NoError(t, d.Refresh())

	got := d.State()
	got.Transport.State = "MUTATED"

	assert.Equal(t, "PLAYING", d.State().Transport.State, "mutating a returned State() copy must not affect the Device")
}

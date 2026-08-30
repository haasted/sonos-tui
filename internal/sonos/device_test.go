package sonos_test

import (
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

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

func notifyBody(inner string) []byte {
	return []byte(`<?xml version="1.0"?>
<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0">
  <e:property><LastChange>` +
		html.EscapeString(`<Event xmlns="urn:schemas-upnp-org:metadata-1-0/AVT/"><InstanceID val="0">`+inner+`</InstanceID></Event>`) +
		`</LastChange></e:property>
</e:propertyset>`)
}

func didlItem(title, artist, album string) string {
	return html.EscapeString(`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">` +
		`<item><dc:title>` + title + `</dc:title><dc:creator>` + artist + `</dc:creator>` +
		`<upnp:album>` + album + `</upnp:album></item></DIDL-Lite>`)
}

func TestDevice_HandleEvent_UpdatesStateAndPublishes(t *testing.T) {
	srv := avTransportServer(t, "PLAYING")
	defer srv.Close()

	d := newTestDevice(t, srv)

	body := notifyBody(
		`<TransportState val="PLAYING"/>` +
			`<CurrentTrackDuration val="0:04:00"/>` +
			`<RelativeTimePosition val="0:01:30"/>` +
			`<CurrentTrackURI val="x-sonos-http:live"/>` +
			`<CurrentTrackMetaData val="` + didlItem("Live Track", "Live Artist", "Live Album") + `"/>`)

	require.NoError(t, d.HandleEvent(body))

	state := d.State()
	assert.Equal(t, "PLAYING", state.Transport.State)
	assert.Equal(t, "Live Track", state.Current.Title)
	assert.Equal(t, "Live Artist", state.Current.Artist)
	assert.Equal(t, "0:01:30", state.RelTime, "a position present in the event should be used as-is")

	select {
	case event := <-d.Events():
		assert.Equal(t, "PLAYING", event.Transport.State)
		assert.Equal(t, "Live Track", event.Current.Title)
	default:
		t.Fatal("expected an event to be published on Events()")
	}
}

func TestDevice_HandleEvent_FetchesPositionWhenEventOmitsIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body><u:GetPositionInfoResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
    <TrackDuration>0:04:00</TrackDuration>
    <TrackMetaData>NOT_IMPLEMENTED</TrackMetaData>
    <TrackURI>x-sonos-http:live</TrackURI>
    <RelTime>0:02:45</RelTime>
  </u:GetPositionInfoResponse></s:Body>
</s:Envelope>`)
	}))
	defer srv.Close()

	d := newTestDevice(t, srv)

	require.NoError(t, d.HandleEvent(notifyBody(`<TransportState val="PAUSED_PLAYBACK"/>`)))

	assert.Equal(t, "PAUSED_PLAYBACK", d.State().Transport.State)
	assert.Equal(t, "0:02:45", d.State().RelTime, "position should come from GetPositionInfo when the event omits it")
}

func TestDevice_HandleEvent_MalformedBodyIsAnError(t *testing.T) {
	srv := avTransportServer(t, "PLAYING")
	defer srv.Close()

	d := newTestDevice(t, srv)

	require.Error(t, d.HandleEvent([]byte("not xml at all")))
}

func TestDevice_HandleEvent_DropsRatherThanBlockingWhenNobodyReceives(t *testing.T) {
	srv := avTransportServer(t, "PLAYING")
	defer srv.Close()

	d := newTestDevice(t, srv)

	body := notifyBody(`<TransportState val="PLAYING"/><RelativeTimePosition val="0:00:10"/>`)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range sonos.EventChanBuffer * 3 {
			require.NoError(t, d.HandleEvent(body))
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("HandleEvent blocked when the event channel filled up")
	}

	assert.Equal(t, "PLAYING", d.State().Transport.State, "state must still be applied even when events are dropped")
}

func TestDevice_HandleEvent_ConcurrentWithCommands(t *testing.T) {
	srv := avTransportServer(t, "PLAYING")
	defer srv.Close()

	d := newTestDevice(t, srv)
	body := notifyBody(`<TransportState val="PLAYING"/><RelativeTimePosition val="0:00:10"/>`)

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for range d.Events() {
		}
	}()

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, d.HandleEvent(body))
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, d.Play())
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = d.State()
		}()
	}
	wg.Wait()

	assert.NotEmpty(t, d.State().Transport.State)
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

package sonos_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haasted/sonos-tui/internal/sonos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderingServer is a fake speaker that holds real volume and mute state, so
// relative adjustments and mute toggles can be asserted against their effect.
type renderingServer struct {
	*httptest.Server

	mu      sync.Mutex
	volume  int      // the speaker's live volume, mutated by the actions it serves
	muted   bool     // the speaker's live mute state
	actions []string // every SOAP action received, in order
}

const renderingNS = "urn:schemas-upnp-org:service:RenderingControl:1" // service namespace echoed back in responses

const envelope = `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>%s</s:Body></s:Envelope>` // SOAP wrapper for fake responses

// Case: initial load, seeding cached state from a speaker that is already muted.
func TestDevice_RefreshVolume_SeedsVolumeAndMute(t *testing.T) {
	rs := newRenderingServer(t, 37, true)
	defer rs.Close()

	d := newVolumeDevice(t, rs)
	require.NoError(t, d.RefreshVolume(context.Background()))

	assert.Equal(t, 37, d.State().Volume)
	assert.True(t, d.State().Muted)
}

// Case: a successful adjustment, where the speaker's own NewVolume is authoritative.
func TestDevice_AdjustVolume_ReturnsSpeakerReportedLevel(t *testing.T) {
	rs := newRenderingServer(t, 40, false)
	defer rs.Close()

	d := newVolumeDevice(t, rs)

	got, err := d.AdjustVolume(context.Background(), 5)
	require.NoError(t, err)

	assert.Equal(t, 45, got, "the speaker's NewVolume should be returned, not a locally computed guess")
	assert.Equal(t, 45, d.State().Volume)
}

// Case: adjusting with no prior RefreshVolume, relying on the delta being relative.
func TestDevice_AdjustVolume_NeedsNoPriorVolumeKnowledge(t *testing.T) {
	rs := newRenderingServer(t, 60, false)
	defer rs.Close()

	d := newVolumeDevice(t, rs)

	got, err := d.AdjustVolume(context.Background(), -10)
	require.NoError(t, err)

	assert.Equal(t, 50, got, "a relative adjustment should apply to the speaker's real level without RefreshVolume first")
	assert.NotContains(t, rs.calls(), "GetVolume", "adjusting relatively should not need to read the volume first")
}

// Case: raising volume on a muted speaker, which must unmute before adjusting.
func TestDevice_AdjustVolume_UnmutesFirstWhenMuted(t *testing.T) {
	rs := newRenderingServer(t, 30, true)
	defer rs.Close()

	d := newVolumeDevice(t, rs)
	require.NoError(t, d.RefreshVolume(context.Background()))

	_, err := d.AdjustVolume(context.Background(), 2)
	require.NoError(t, err)

	assert.False(t, d.State().Muted, "changing volume should unmute, like a physical remote")

	calls := rs.calls()
	setMute := indexOf(calls, "SetMute")
	setVolume := indexOf(calls, "SetRelativeVolume")
	require.NotEqual(t, -1, setMute, "expected an unmute call")
	assert.Less(t, setMute, setVolume, "unmute must happen before the volume change, not after")
}

// Case: adjusting an already-unmuted speaker, which must not send a redundant unmute.
func TestDevice_AdjustVolume_DoesNotUnmuteWhenAlreadyUnmuted(t *testing.T) {
	rs := newRenderingServer(t, 30, false)
	defer rs.Close()

	d := newVolumeDevice(t, rs)
	require.NoError(t, d.RefreshVolume(context.Background()))

	_, err := d.AdjustVolume(context.Background(), 2)
	require.NoError(t, err)

	assert.NotContains(t, rs.calls()[2:], "SetMute", "an unmuted speaker should not be sent a redundant unmute")
}

// Case: toggling mute on, then off again.
func TestDevice_ToggleMute_FlipsBothWays(t *testing.T) {
	rs := newRenderingServer(t, 30, false)
	defer rs.Close()

	d := newVolumeDevice(t, rs)
	require.NoError(t, d.RefreshVolume(context.Background()))

	require.NoError(t, d.ToggleMute(context.Background()))
	assert.True(t, d.State().Muted)

	require.NoError(t, d.ToggleMute(context.Background()))
	assert.False(t, d.State().Muted)
}

// Case: a speaker that rejects the adjustment, leaving cached volume untouched.
func TestDevice_AdjustVolume_LeavesStateUnchangedOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "speaker exploded", http.StatusInternalServerError)
	}))
	defer srv.Close()

	speaker := sonos.Speaker{IP: "test", Location: srv.URL}
	d := sonos.NewDeviceWithClient(speaker, &sonos.SonosClient{BaseURL: srv.URL, HTTPClient: srv.Client()})

	_, err := d.AdjustVolume(context.Background(), 5)
	require.Error(t, err)
	assert.Equal(t, 0, d.State().Volume, "a failed adjustment must not record a volume that never took effect")
}

// Case: four requests (transport, position, volume, mute) against a slow speaker —
// roughly 4*delay if sequential, roughly 1*delay if genuinely concurrent.
func TestDevice_RefreshAll_RunsRequestsConcurrently(t *testing.T) {
	const delay = 60 * time.Millisecond

	srv := slowSpeaker(t, delay)
	defer srv.Close()

	speaker := sonos.Speaker{IP: "test", Location: srv.URL}
	d := sonos.NewDeviceWithClient(speaker, &sonos.SonosClient{BaseURL: srv.URL, HTTPClient: srv.Client()})

	start := time.Now()
	require.NoError(t, d.RefreshAll(context.Background()))
	elapsed := time.Since(start)

	if elapsed > 3*delay {
		t.Fatalf("RefreshAll took %v for 4 requests of %v each — that looks sequential, not concurrent", elapsed, delay)
	}

	state := d.State()
	assert.Equal(t, "PLAYING", state.Transport.State)
	assert.Equal(t, "0:01:00", state.RelTime)
	assert.Equal(t, 25, state.Volume)
}

// Case: every refresh failing, where all errors must surface rather than just the first.
func TestDevice_RefreshAll_ReportsEveryFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	speaker := sonos.Speaker{IP: "test", Location: srv.URL}
	d := sonos.NewDeviceWithClient(speaker, &sonos.SonosClient{BaseURL: srv.URL, HTTPClient: srv.Client()})

	err := d.RefreshAll(context.Background())
	require.Error(t, err)

	joined, ok := err.(interface{ Unwrap() []error })
	require.True(t, ok, "RefreshAll should join its errors so one failure doesn't hide the others")
	assert.Len(t, joined.Unwrap(), 3, "every failing refresh should be reported, not just the first")
}

// newRenderingServer starts a fake speaker seeded with the given volume and mute state.
func newRenderingServer(t *testing.T, volume int, muted bool) *renderingServer {
	t.Helper()

	rs := &renderingServer{volume: volume, muted: muted}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)

		action := r.Header.Get("SOAPAction")
		action = action[strings.LastIndex(action, "#")+1 : len(action)-1]

		rs.mu.Lock()
		defer rs.mu.Unlock()
		rs.actions = append(rs.actions, action)

		w.Header().Set("Content-Type", "text/xml")

		switch action {
		case "GetVolume":
			fmt.Fprintf(w, envelope, fmt.Sprintf(
				`<u:GetVolumeResponse xmlns:u="%s"><CurrentVolume>%d</CurrentVolume></u:GetVolumeResponse>`,
				renderingNS, rs.volume))
		case "GetMute":
			mute := 0
			if rs.muted {
				mute = 1
			}
			fmt.Fprintf(w, envelope, fmt.Sprintf(
				`<u:GetMuteResponse xmlns:u="%s"><CurrentMute>%d</CurrentMute></u:GetMuteResponse>`,
				renderingNS, mute))
		case "SetRelativeVolume":
			rs.volume += parseAdjustment(string(body))
			rs.volume = min(100, max(0, rs.volume))
			fmt.Fprintf(w, envelope, fmt.Sprintf(
				`<u:SetRelativeVolumeResponse xmlns:u="%s"><NewVolume>%d</NewVolume></u:SetRelativeVolumeResponse>`,
				renderingNS, rs.volume))
		case "SetMute":
			rs.muted = strings.Contains(string(body), "<DesiredMute>1</DesiredMute>")
			fmt.Fprintf(w, envelope, `<u:SetMuteResponse/>`)
		default:
			fmt.Fprintf(w, envelope, `<u:Response/>`)
		}
	}))
	return rs
}

// parseAdjustment reads the Adjustment argument out of a SetRelativeVolume body.
func parseAdjustment(body string) int {
	const open, close = "<Adjustment>", "</Adjustment>"
	i := strings.Index(body, open)
	if i < 0 {
		return 0
	}
	j := strings.Index(body[i:], close)
	if j < 0 {
		return 0
	}
	var n int
	_, _ = fmt.Sscanf(body[i+len(open):i+j], "%d", &n)
	return n
}

// calls returns a copy of the SOAP actions received so far.
func (rs *renderingServer) calls() []string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return append([]string(nil), rs.actions...)
}

// newVolumeDevice builds a Device wired to the fake speaker.
func newVolumeDevice(t *testing.T, rs *renderingServer) *sonos.Device {
	t.Helper()
	speaker := sonos.Speaker{IP: "test", Location: rs.URL + "/xml/device_description.xml"}
	return sonos.NewDeviceWithClient(speaker, &sonos.SonosClient{BaseURL: rs.URL, HTTPClient: rs.Client()})
}

// indexOf returns the first position of want, or -1 if absent.
func indexOf(items []string, want string) int {
	for i, item := range items {
		if item == want {
			return i
		}
	}
	return -1
}

// slowSpeaker answers every request after delay, so sequential and concurrent
// request patterns are distinguishable by elapsed time.
func slowSpeaker(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)

		action := r.Header.Get("SOAPAction")
		action = action[strings.LastIndex(action, "#")+1 : len(action)-1]

		w.Header().Set("Content-Type", "text/xml")
		switch action {
		case "GetVolume":
			fmt.Fprintf(w, envelope, `<u:GetVolumeResponse><CurrentVolume>25</CurrentVolume></u:GetVolumeResponse>`)
		case "GetMute":
			fmt.Fprintf(w, envelope, `<u:GetMuteResponse><CurrentMute>0</CurrentMute></u:GetMuteResponse>`)
		case "GetTransportInfo":
			fmt.Fprintf(w, envelope, `<u:GetTransportInfoResponse><CurrentTransportState>PLAYING</CurrentTransportState></u:GetTransportInfoResponse>`)
		case "GetPositionInfo":
			fmt.Fprintf(w, envelope, `<u:GetPositionInfoResponse><RelTime>0:01:00</RelTime></u:GetPositionInfoResponse>`)
		default:
			fmt.Fprintf(w, envelope, `<u:Response/>`)
		}
	}))
}

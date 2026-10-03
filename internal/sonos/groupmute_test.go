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

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: muting a group. Every member goes quiet from one call, so the toggle
// must reach GroupRenderingControl rather than the coordinator's own channel.
func TestDevice_ToggleGroupMute_MutesTheWholeGroup(t *testing.T) {
	gs := newGroupServer(t, false)
	device := groupDevice(t, gs)

	muted, err := device.ToggleGroupMute(context.Background())

	require.NoError(t, err)
	assert.True(t, muted, "the toggle reported the group unmuted after muting it")
	assert.True(t, gs.state(), "the speaker was never actually muted")
	assert.Equal(t, []string{"GetGroupMute", "SetGroupMute"}, gs.calls())
}

// Case: unmuting. The value is read from the speaker first, so a group muted
// by the Sonos app rather than by this program still toggles the right way.
func TestDevice_ToggleGroupMute_ReadsTheGroupRatherThanItsOwnState(t *testing.T) {
	gs := newGroupServer(t, true)
	device := groupDevice(t, gs)

	muted, err := device.ToggleGroupMute(context.Background())

	require.NoError(t, err)
	assert.False(t, muted)
	assert.False(t, gs.state(), "a group already muted elsewhere was muted again")
}

// Case: the path the request goes to. A wrong control URL answers with a
// fault, not with silence, so this pins the one that works.
func TestDevice_ToggleGroupMute_UsesTheGroupControlURL(t *testing.T) {
	gs := newGroupServer(t, false)
	device := groupDevice(t, gs)

	require.NoError(t, device.SetGroupMuted(context.Background(), true))

	assert.Equal(t, "/MediaRenderer/GroupRenderingControl/Control", gs.path())
}

// groupServer is a fake speaker holding one group mute, so a toggle can be
// asserted against its effect rather than against the bytes sent.
type groupServer struct {
	*httptest.Server

	mu       sync.Mutex
	muted    bool
	actions  []string
	lastPath string
}

func newGroupServer(t *testing.T, muted bool) *groupServer {
	t.Helper()

	gs := &groupServer{muted: muted}
	gs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		action := actionOf(r.Header.Get("SOAPACTION"))

		gs.mu.Lock()
		gs.actions = append(gs.actions, action)
		gs.lastPath = r.URL.Path
		if action == "SetGroupMute" {
			gs.muted = strings.Contains(string(body), "<DesiredMute>1</DesiredMute>")
		}
		muted := gs.muted
		gs.mu.Unlock()

		value := 0
		if muted {
			value = 1
		}
		fmt.Fprintf(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body>`+
			`<u:%sResponse xmlns:u="urn:schemas-upnp-org:service:GroupRenderingControl:1">`+
			`<CurrentMute>%d</CurrentMute></u:%sResponse></s:Body></s:Envelope>`, action, value, action)
	}))
	t.Cleanup(gs.Close)
	return gs
}

func (gs *groupServer) state() bool {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return gs.muted
}

func (gs *groupServer) calls() []string {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return append([]string(nil), gs.actions...)
}

func (gs *groupServer) path() string {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return gs.lastPath
}

func groupDevice(t *testing.T, gs *groupServer) *sonos.Device {
	t.Helper()

	return sonos.NewDeviceWithClient(
		sonos.Speaker{IP: "192.168.1.10"},
		&sonos.SonosClient{BaseURL: gs.URL, HTTPClient: gs.Client()},
	)
}

func actionOf(soapAction string) string {
	_, action, _ := strings.Cut(strings.Trim(soapAction, `"`), "#")
	return action
}

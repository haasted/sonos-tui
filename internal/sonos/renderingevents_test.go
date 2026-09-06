package sonos_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a real event captured from a Sonos Amp, where Volume and Mute repeat per
// channel and Master differs from the per-driver channels.
func TestParseRenderingControlEvent_RealCapture(t *testing.T) {
	body := readFixture(t, "renderingcontrol_lastchange.xml")

	event, err := sonos.ParseRenderingControlEvent(body)
	require.NoError(t, err)

	assert.True(t, event.HasVolume)
	assert.Equal(t, 32, event.Volume,
		"must read the Master channel, not LF/RF which both report 100")
	assert.True(t, event.HasMute)
	assert.False(t, event.Muted)
}

// Case: an event carrying only a mute change, as a targeted toggle produces.
func TestParseRenderingControlEvent_MuteOnly(t *testing.T) {
	event, err := sonos.ParseRenderingControlEvent(renderingBody(
		`<Mute channel="Master" val="1"/><Mute channel="LF" val="0"/>`))
	require.NoError(t, err)

	assert.True(t, event.Muted)
	assert.True(t, event.HasMute)
	assert.False(t, event.HasVolume, "no Master volume in the event means no volume to apply")
}

// Case: only per-driver channels present, with no Master to read.
func TestParseRenderingControlEvent_IgnoresNonMasterChannels(t *testing.T) {
	event, err := sonos.ParseRenderingControlEvent(renderingBody(
		`<Volume channel="LF" val="100"/><Volume channel="RF" val="100"/>`))
	require.NoError(t, err)

	assert.False(t, event.HasVolume, "LF/RF alone must not be mistaken for the speaker's volume")
	assert.Equal(t, 0, event.Volume)
}

func TestParseRenderingControlEvent_MalformedBodyIsAnError(t *testing.T) {
	_, err := sonos.ParseRenderingControlEvent([]byte("not xml at all"))
	require.Error(t, err)
}

// readFixture loads a captured event body. These live in testdata rather than
// captured-data/, which is gitignored scratch space.
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return body
}

func renderingBody(inner string) []byte {
	return []byte(`<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0"><e:property><LastChange>` +
		`&lt;Event xmlns=&quot;urn:schemas-upnp-org:metadata-1-0/RCS/&quot;&gt;` +
		`&lt;InstanceID val=&quot;0&quot;&gt;` + escapeAttrs(inner) +
		`&lt;/InstanceID&gt;&lt;/Event&gt;</LastChange></e:property></e:propertyset>`)
}

func escapeAttrs(s string) string {
	out := ""
	for _, r := range s {
		switch r {
		case '<':
			out += "&lt;"
		case '>':
			out += "&gt;"
		case '"':
			out += "&quot;"
		default:
			out += string(r)
		}
	}
	return out
}

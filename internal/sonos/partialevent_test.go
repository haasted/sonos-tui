package sonos_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonos"
)

// Case: an event that says only that playback paused. LastChange omits every
// variable that did not change, so this one carries no track at all and must
// leave the track state exactly as it found it.
func TestPartialEvent_LeavesUnmentionedTrackStateAlone(t *testing.T) {
	device := eventDevice(t)
	applyEvent(t, device, lastChange(
		`<TransportState val="PLAYING"/>`,
		`<CurrentTrack val="7"/>`,
		`<NumberOfTracks val="23"/>`,
		`<CurrentTrackURI val="x-sonos-http:track-7.mp3"/>`,
		`<CurrentTrackMetaData val="&lt;DIDL-Lite&gt;&lt;item&gt;&lt;dc:title&gt;Autobahn&lt;/dc:title&gt;&lt;/item&gt;&lt;/DIDL-Lite&gt;"/>`,
	))

	applyEvent(t, device, lastChange(`<TransportState val="PAUSED_PLAYBACK"/>`))

	state := device.State()
	assert.Equal(t, "PAUSED_PLAYBACK", state.Transport.State, "the one thing the event did say was not applied")
	assert.Equal(t, 7, state.Track, "the track number was erased by an event that never mentioned it")
	assert.Equal(t, 23, state.Tracks, "the track count was erased by an event that never mentioned it")
	assert.Equal(t, "Autobahn", state.Current.Title, "the playing track was erased by an event that never mentioned it")
	assert.Equal(t, "x-sonos-http:track-7.mp3", state.Current.URI)
}

// Case: an event that really does report CurrentTrack 0, which is what a
// speaker says when nothing is playing from the queue. Zero is a reading
// here, not an absence, so it must be applied.
func TestPartialEvent_AReportedZeroIsApplied(t *testing.T) {
	device := eventDevice(t)
	applyEvent(t, device, lastChange(
		`<CurrentTrack val="7"/>`,
		`<NumberOfTracks val="23"/>`,
	))
	require.Equal(t, 7, device.State().Track)

	applyEvent(t, device, lastChange(
		`<CurrentTrack val="0"/>`,
		`<NumberOfTracks val="0"/>`,
	))

	state := device.State()
	assert.Equal(t, 0, state.Track, "a track number the speaker really reported was ignored")
	assert.Equal(t, 0, state.Tracks, "a track count the speaker really reported was ignored")
}

// Case: the presence flags themselves, read off the parser rather than
// through a Device, so a failure says which half is wrong.
func TestPartialEvent_TheParserSaysWhatArrived(t *testing.T) {
	full, err := sonos.ParseAVTransportEvent(lastChange(
		`<CurrentTrack val="0"/>`, `<NumberOfTracks val="0"/>`, `<CurrentTrackURI val=""/>`))
	require.NoError(t, err)

	assert.True(t, full.HasTrack, "a reported CurrentTrack of 0 must read as present")
	assert.True(t, full.HasTracks)
	assert.True(t, full.HasCurrent, "a reported empty URI must read as present")

	bare, err := sonos.ParseAVTransportEvent(lastChange(`<TransportState val="STOPPED"/>`))
	require.NoError(t, err)

	assert.False(t, bare.HasTrack, "an absent CurrentTrack must not read as present")
	assert.False(t, bare.HasTracks)
	assert.False(t, bare.HasCurrent)
}

// Case: a source the event does not mention, the one field already guarded
// before this. Kept so the audit covers it rather than assuming.
func TestPartialEvent_LeavesAnUnmentionedSourceAlone(t *testing.T) {
	device := eventDevice(t)
	applyEvent(t, device, lastChange(`<AVTransportURI val="x-rincon-queue:RINCON_1#0"/>`))
	require.Equal(t, sonos.SourceTrack, device.State().Source.Kind())

	applyEvent(t, device, lastChange(`<TransportState val="STOPPED"/>`))

	assert.Equal(t, sonos.SourceTrack, device.State().Source.Kind(),
		"the source was erased by an event that never mentioned it")
}

// Case: a real pause, captured from an Era 100 playing a queue. Every other
// AVTransport fixture here is a SUBSCRIBE replay carrying all 25 variables,
// which is the least representative shape a speaker sends; this is the shape
// it sends the rest of the time.
func TestPartialEvent_ARealPauseCarriesTheTrackButNotTheSource(t *testing.T) {
	event, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_queue_pause.xml"))
	require.NoError(t, err)

	assert.Equal(t, "PAUSED_PLAYBACK", event.TransportState)

	// The nine-variable core every event carries, whatever changed. Measured
	// over 17 notifications: there is no transport-only event.
	assert.True(t, event.HasTrack, "a real pause carried no CurrentTrack")
	assert.True(t, event.HasTracks)
	assert.True(t, event.HasCurrent)
	assert.Equal(t, "Girl in Amber", event.Current.Title)

	// AVTransportURI is NOT in that core -- it arrived in 3 of those 17. This
	// is the omission the guard on Source exists for, and the only one these
	// captures caught a speaker making.
	assert.Empty(t, event.Source.URI,
		"this capture is the evidence that an ordinary event omits the source")
}

// xmlEscaper escapes the inner Event document, which a speaker sends as the
// text of <LastChange> rather than as markup.
var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// lastChange wraps LastChange variables in the propertyset a speaker POSTs.
// RelativeTimePosition is always included so handling never reaches for the
// network to fill a position in.
func lastChange(vars ...string) []byte {
	inner := `<Event xmlns="urn:schemas-upnp-org:metadata-1-0/AVT/"><InstanceID val="0">` +
		`<RelativeTimePosition val="0:00:12"/>`
	for _, v := range vars {
		inner += v
	}
	inner += `</InstanceID></Event>`

	return []byte(`<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0"><e:property><LastChange>` +
		xmlEscaper.Replace(inner) + `</LastChange></e:property></e:propertyset>`)
}

func eventDevice(t *testing.T) *sonos.Device {
	t.Helper()

	return sonos.NewDeviceWithClient(sonos.Speaker{IP: "192.168.1.10"}, &sonos.SonosClient{})
}

func applyEvent(t *testing.T, device *sonos.Device, body []byte) {
	t.Helper()

	require.NoError(t, device.HandleAVTransportEvent(context.Background(), body))
	<-device.Events()
}

// Case: a source the speaker really reports as empty, which is what it says
// when nothing is loaded. The old guard tested the URI for emptiness and so
// could not tell this from an event that said nothing; presence can, and a
// reading of "nothing is loaded" is one worth applying.
func TestPartialEvent_AReportedEmptySourceIsApplied(t *testing.T) {
	device := eventDevice(t)
	applyEvent(t, device, lastChange(`<AVTransportURI val="x-rincon-queue:RINCON_1#0"/>`))
	require.Equal(t, sonos.SourceTrack, device.State().Source.Kind())

	applyEvent(t, device, lastChange(`<AVTransportURI val=""/>`))

	assert.Equal(t, sonos.SourceUnknown, device.State().Source.Kind(),
		"a source the speaker reported as empty was ignored")
}

// Case: the transport state, guarded the same way. An event that does not
// mention it must not blank it, and "" is not a state a speaker reports.
func TestPartialEvent_LeavesAnUnmentionedTransportStateAlone(t *testing.T) {
	device := eventDevice(t)
	applyEvent(t, device, lastChange(`<TransportState val="PLAYING"/>`))

	applyEvent(t, device, lastChange(`<CurrentTrack val="4"/>`))

	assert.Equal(t, "PLAYING", device.State().Transport.State,
		"the transport state was erased by an event that never mentioned it")
	assert.Equal(t, 4, device.State().Track)
}

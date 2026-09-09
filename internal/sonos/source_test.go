package sonos_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: every kind of source, read from real captured events rather than
// hand-written XML. The fixtures came off actual speakers on 2026-09-09 and
// corrected two assumptions this feature was originally designed on, so
// asserting against invented bodies would defeat the point.
func TestSource_ClassifiesRealCaptures(t *testing.T) {
	for _, tc := range []struct {
		fixture  string
		kind     sonos.SourceKind
		station  string
		duration string
		title    string
	}{
		// Both station cases carry NO dc:title in their track metadata — the
		// name exists only in AVTransportURIMetaData. Anything showing a title
		// for these has to fall back to the source name.
		{"broadcast_radio", sonos.SourceBroadcast, "DR P1", "0:00:00", ""},
		{"station_no_track", sonos.SourceStation, "Henrik Aasted’s Station", "0:00:00", ""},
		{"station_with_track", sonos.SourceStation, "Akylas", "0:03:19", "Party"},
		{"queue_track", sonos.SourceTrack, "", "0:03:11", "It Was Us"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			event, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_"+tc.fixture+".xml"))
			require.NoError(t, err)

			assert.Equal(t, tc.kind, event.Source.Kind(), "source URI was %q", event.Source.URI)
			assert.Equal(t, tc.station, event.Source.Name, "station name")
			assert.Equal(t, tc.duration, event.Current.Duration)
			assert.Equal(t, tc.title, event.Current.Title)
		})
	}
}

// Case: an algorithmic station playing a real track. It is a station, but its
// clock works — so "is a stream" and "has a usable duration" are separate
// questions, and the UI must not answer the second with the first.
func TestSource_StationCanCarryATrackWithADuration(t *testing.T) {
	event, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_station_with_track.xml"))
	require.NoError(t, err)

	assert.Equal(t, sonos.SourceStation, event.Source.Kind())
	assert.Equal(t, "Akylas", event.Source.Name, "the station")
	assert.Equal(t, "Party", event.Current.Title, "the track playing on it")
	assert.Equal(t, "Marseaux & Billie Kark", event.Current.Artist)
	assert.NotEqual(t, "0:00:00", event.Current.Duration, "this one has a real clock")
}

// Case: broadcast radio, whose track metadata is empty of everything a track
// normally has. The UI variant must treat "a name and nothing else" as the
// normal case, and must take that name from the source rather than the track.
func TestSource_BroadcastCarriesNoTrackDetail(t *testing.T) {
	event, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_broadcast_radio.xml"))
	require.NoError(t, err)

	assert.Empty(t, event.Current.Title, "no dc:title at all in a broadcast's track metadata")
	assert.Empty(t, event.Current.Artist)
	assert.Empty(t, event.Current.Album)
	assert.Equal(t, "0:00:00", event.Current.Duration)
	assert.Equal(t, "DR P1", event.Source.Name, "the only place the name exists")
}

// Case: r:radioShowMd, which is the only thing a broadcast station says about
// what is on. It is comma-delimited with literal commas escaped, and Sonos
// truncates it — this capture ends mid-word.
func TestSource_BroadcastCarriesAShowDescription(t *testing.T) {
	event, err := sonos.ParseAVTransportEvent(fixture(t, "avtransport_broadcast_radio.xml"))
	require.NoError(t, err)

	show := event.Current.RadioShow
	assert.Contains(t, show, "P1 Morgen")
	assert.Contains(t, show, "Perspektiv, debat", "an escaped comma must survive as a comma")
	assert.NotContains(t, show, `\,`, "the escape itself must not reach the UI")
	assert.False(t, strings.HasSuffix(show, ","), "the field delimiter must be stripped")
}

// Case: the delimiter rules on their own, since no capture yet shows a station
// that puts anything in the second field.
func TestSource_RadioShowKeepsOnlyTheDescription(t *testing.T) {
	for raw, want := range map[string]string{
		`Just a show`:                "Just a show",
		`Morning\, with news,p12345`: "Morning, with news",
		`Trailing delimiter,`:        "Trailing delimiter",
		``:                           "",
	} {
		assert.Equal(t, want, sonos.ParseRadioShow(raw), "raw %q", raw)
	}
}

// Case: schemes documented but not present in the capture, plus junk. Kind
// must never guess "track" for something it does not recognise, or an unknown
// stream would get a countdown against a zero duration.
func TestSource_UnrecognisedSchemesAreNotTreatedAsTracks(t *testing.T) {
	for uri, want := range map[string]sonos.SourceKind{
		"x-rincon-stream:RINCON_123":  sonos.SourceLineIn,
		"x-sonosapi-stream:something": sonos.SourceStation,
		"x-file-cifs://nas/song.flac": sonos.SourceTrack,
		"something-new:whatever":      sonos.SourceUnknown,
		"":                            sonos.SourceUnknown,
	} {
		assert.Equal(t, want, sonos.SourceInfo{URI: uri}.Kind(), "uri %q", uri)
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return body
}

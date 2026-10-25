package sonos_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: URIs as speakers wrote them on 2026-10-25, which must each name the
// service, account and id. The two playlist URIs are one playlist under two
// prefixes and two cases of hex, and must read the same.
func TestParseServiceURI_ReadsWhatTheSpeakersWrote(t *testing.T) {
	singles := sonos.ServiceRef{Service: 204, Account: 2, ItemID: "libraryplaylist:p.V7VYpk7FYWLeW"}
	cases := map[string]sonos.ServiceRef{
		"x-rincon-cpcontainer:00060000libraryplaylist%3ap.V7VYpk7FYWLeW?sid=204&flags=0&sn=2":    singles,
		"x-rincon-cpcontainer:1006206clibraryplaylist%3Ap.V7VYpk7FYWLeW?sid=204&flags=8300&sn=2": singles,
		"x-rincon-cpcontainer:1004206calbum%3a1467347588?sid=204&flags=8300&sn=2": {
			Service: 204, Account: 2, ItemID: "album:1467347588"},
		"x-sonos-http:librarytrack%3ai.2P0D02rt16oe6.mp4?sid=204&flags=73768&sn=2": {
			Service: 204, Account: 2, ItemID: "librarytrack:i.2P0D02rt16oe6.mp4"},
		"x-sonosapi-hls:urn%3adr%3aradio%3achannel%3a5fa156d1da351264f87b462d?sid=290&flags=8232&sn=3": {
			Service: 290, Account: 3, ItemID: "urn:dr:radio:channel:5fa156d1da351264f87b462d"},
		"x-sonosapi-stream:r%3A418737?sid=268&flags=8292&sn=5": {
			Service: 268, Account: 5, ItemID: "r:418737"},
	}
	for uri, want := range cases {
		got, ok := sonos.ParseServiceURI(uri)
		assert.True(t, ok, uri)
		assert.Equal(t, want, got, uri)
	}
}

// Case: URIs that name no music-service item, which must be refused rather
// than read as one with a zero service.
func TestParseServiceURI_RefusesWhatIsNotAServiceItem(t *testing.T) {
	for _, uri := range []string{
		"x-rincon-queue:RINCON_542A1B50C10A01400#0",
		"x-rincon-mp3radio:http://stream.example/live.mp3",
		"x-sonos-http:song%3a1?flags=8232&sn=2",
		"x-rincon-cpcontainer:00060000?sid=204&sn=2",
		"",
	} {
		_, ok := sonos.ParseServiceURI(uri)
		assert.False(t, ok, uri)
	}
}

// Case: a DIDL holding artwork, which a stored copy must leave out.
func TestStripArtwork_RemovesTheArtworkAndNothingElse(t *testing.T) {
	didl := `<item><dc:title>DR P1</dc:title>` +
		`<upnp:albumArtURI>https://example/art.png?X-Amz-Expires=86400&amp;sig=1</upnp:albumArtURI>` +
		`<upnp:class>object.item.audioItem.audioBroadcast</upnp:class></item>`

	assert.Equal(t,
		`<item><dc:title>DR P1</dc:title><upnp:class>object.item.audioItem.audioBroadcast</upnp:class></item>`,
		sonos.StripArtwork(didl))
}

// Case: a queue row from Soveværelse, which carries no desc. The built DIDL
// must name the service, and use the id without its .mp4.
func TestTrackPlayable_BuildsTheDIDLAQueueRowLacks(t *testing.T) {
	row := sonos.TrackMetadata{
		URI:    "x-sonos-http:librarytrack%3ai.2P0D02rt16oe6.mp4?sid=204&flags=73768&sn=2",
		Title:  "Paint The Town Red",
		Artist: "Doja Cat",
		Album:  "Paint The Town Red - Single",
	}

	playable, err := sonos.TrackPlayable(row)
	require.NoError(t, err)

	assert.Equal(t, row.URI, playable.URI)
	assert.Equal(t, `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" `+
		`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" `+
		`xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" `+
		`xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">`+
		`<item id="10032020librarytrack%3ai.2P0D02rt16oe6" parentID="-1" restricted="true">`+
		`<dc:title>Paint The Town Red</dc:title>`+
		`<upnp:class>object.item.audioItem.musicTrack</upnp:class>`+
		`<dc:creator>Doja Cat</dc:creator>`+
		`<upnp:album>Paint The Town Red - Single</upnp:album>`+
		`<desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">`+
		`SA_RINCON52231_X_#Svc52231-0-Token</desc>`+
		`</item></DIDL-Lite>`, playable.Metadata)
}

// Case: a track that is no music-service item, which cannot be described.
func TestTrackPlayable_RefusesATrackWithNoService(t *testing.T) {
	_, err := sonos.TrackPlayable(sonos.TrackMetadata{URI: "x-file-cifs://nas/music/a.flac", Title: "a"})
	assert.Error(t, err)
}

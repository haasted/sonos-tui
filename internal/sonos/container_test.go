package sonos_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// awesomeSingles is the playlist Soveværelse reported in
// testdata/avtransport_queue_playlist.xml.
var awesomeSingles = sonos.ServiceContainer{
	Kind:    sonos.ContainerPlaylist,
	ID:      "libraryplaylist:p.V7VYpk7FYWLeW",
	Title:   "Awesome singles",
	Service: 204,
	Account: 2,
}

// Case: a playlist, whose URI must be the one the speaker reported.
func TestServiceContainer_URI_MatchesThePlaylistCapture(t *testing.T) {
	assert.Equal(t,
		"x-rincon-cpcontainer:00060000libraryplaylist%3ap.V7VYpk7FYWLeW?sid=204&flags=0&sn=2",
		awesomeSingles.URI())
}

// Case: an album, which differs from a playlist in its prefix and id.
func TestServiceContainer_URI_MatchesTheAlbumCapture(t *testing.T) {
	album := sonos.ServiceContainer{
		Kind:    sonos.ContainerAlbum,
		ID:      "album:1888147421",
		Title:   "Ordensduks i Ingenmandsland",
		Service: 204,
		Account: 2,
	}
	assert.Equal(t,
		"x-rincon-cpcontainer:00040000album%3a1888147421?sid=204&flags=0&sn=2",
		album.URI())
}

// Case: the built metadata, which must be the speaker's own, less the artwork.
func TestServiceContainer_Metadata_MatchesTheCaptureWithoutArtwork(t *testing.T) {
	metadata, err := awesomeSingles.Metadata()
	require.NoError(t, err)

	assert.Equal(t, `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" `+
		`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" `+
		`xmlns:r="urn:schemas-rinconnetworks-com:metadata-1-0/" `+
		`xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">`+
		`<item id="00060000libraryplaylist%3ap.V7VYpk7FYWLeW" parentID="-1" restricted="true">`+
		`<dc:title>Awesome singles</dc:title>`+
		`<upnp:class>object.container.playlistContainer</upnp:class>`+
		`<desc id="cdudn" nameSpace="urn:schemas-rinconnetworks-com:metadata-1-0/">`+
		`SA_RINCON52231_X_#Svc52231-0-Token</desc>`+
		`</item></DIDL-Lite>`, metadata)
}

// Case: a title holding XML's own characters, which must arrive escaped.
func TestServiceContainer_Metadata_EscapesTheTitle(t *testing.T) {
	container := awesomeSingles
	container.Title = "Rock & <Roll>"

	metadata, err := container.Metadata()
	require.NoError(t, err)
	assert.Contains(t, metadata, "<dc:title>Rock &amp; &lt;Roll&gt;</dc:title>")
}

// Case: a kind with no known class, which must be refused rather than sent.
func TestServiceContainer_Metadata_RefusesAnUnknownKind(t *testing.T) {
	container := awesomeSingles
	container.Kind = "00090000"

	_, err := container.Metadata()
	assert.Error(t, err)
}

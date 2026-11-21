package sonos

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// trackItemPrefix is what Sonos puts in front of a single track's id, as
// containers have theirs.
const trackItemPrefix = "10032020"

// trackFileSuffix ends a track's id in its URI, and not in its item id.
const trackFileSuffix = ".mp4"

// trackClass is the upnp:class of a single track.
const trackClass = "object.item.audioItem.musicTrack"

// trackFlags is what a speaker's queue row of a catalogue song carries in its
// URI. A track from the reader's own library carries other flags.
const trackFlags = 8232

// artwork matches the artwork element of a DIDL, which a stored copy leaves
// out: some services sign the URL to expire within a day.
var artwork = regexp.MustCompile(`<upnp:albumArtURI>[^<]*</upnp:albumArtURI>`)

// Playable is something a speaker can be given: the URI and the DIDL that
// goes with it. Title names it in an error.
type Playable struct {
	URI      string
	Metadata string
	Title    string
}

// ServiceTrack is one track on a music service, kept as the values that
// differ from one to the next, as ServiceContainer keeps a container. Its URI
// is the form Apple Music's tracks take; other services are untested.
type ServiceTrack struct {
	ID      string // the service's own id, e.g. "song:1774570881"
	Title   string
	Artist  string
	Album   string
	Service int // sid: 204 is Apple Music
	Account int // sn: which account on the service, as this household numbers them
}

// ServiceRef is what a music-service URI says about the item it names. Two
// URIs with the same ref are the same thing, whatever else differs.
type ServiceRef struct {
	Service int    // sid
	Account int    // sn
	ItemID  string // the service's own id, unescaped: "libraryplaylist:p.V7VYpk7FYWLeW"
}

// ParseServiceURI reads the service, account and id out of a URI, or reports
// false for a URI that names no music-service item. A container's kind
// prefix is dropped: one playlist arrives under more than one.
func ParseServiceURI(uri string) (ServiceRef, bool) {
	scheme, rest, ok := strings.Cut(uri, ":")
	if !ok {
		return ServiceRef{}, false
	}
	escaped, query, ok := strings.Cut(rest, "?")
	if !ok {
		return ServiceRef{}, false
	}

	values, err := url.ParseQuery(query)
	if err != nil {
		return ServiceRef{}, false
	}
	service, serviceErr := strconv.Atoi(values.Get("sid"))
	account, accountErr := strconv.Atoi(values.Get("sn"))
	if serviceErr != nil || accountErr != nil {
		return ServiceRef{}, false
	}

	if scheme == "x-rincon-cpcontainer" {
		if len(escaped) <= len(ContainerPlaylist) {
			return ServiceRef{}, false
		}
		escaped = escaped[len(ContainerPlaylist):]
	}
	id, err := url.PathUnescape(escaped)
	if err != nil || id == "" {
		return ServiceRef{}, false
	}
	return ServiceRef{Service: service, Account: account, ItemID: id}, true
}

// StripArtwork removes the artwork from a DIDL before it is stored.
func StripArtwork(didl string) string {
	return artwork.ReplaceAllString(didl, "")
}

// TrackPlayable makes a single track playable from a queue row. A queue row
// carries no desc, so the DIDL is built from what its URI says; the URI is
// kept as the speaker wrote it.
func TrackPlayable(track TrackMetadata) (Playable, error) {
	ref, ok := ParseServiceURI(track.URI)
	if !ok {
		return Playable{}, fmt.Errorf("describing %q: %q is not from a music service", track.Title, track.URI)
	}
	metadata, err := ServiceTrack{
		ID:      strings.TrimSuffix(ref.ItemID, trackFileSuffix),
		Title:   track.Title,
		Artist:  track.Artist,
		Album:   track.Album,
		Service: ref.Service,
		Account: ref.Account,
	}.Metadata()
	if err != nil {
		return Playable{}, err
	}
	return Playable{URI: track.URI, Metadata: metadata, Title: track.Title}, nil
}

// URI is what a speaker takes to play or queue the track.
func (t ServiceTrack) URI() string {
	return fmt.Sprintf("x-sonos-http:%s%s?sid=%d&flags=%d&sn=%d",
		escapeID(t.ID), trackFileSuffix, t.Service, trackFlags, t.Account)
}

// Metadata is the DIDL that goes with URI: the track prefix and the id, without
// the .mp4 the URI adds.
func (t ServiceTrack) Metadata() (string, error) {
	return marshalDIDL(didlItem{
		ID:      trackItemPrefix + escapeID(t.ID),
		Title:   t.Title,
		Class:   trackClass,
		Creator: t.Artist,
		Album:   t.Album,
	}, t.Service)
}

// Playable is the track as a speaker is given it.
func (t ServiceTrack) Playable() (Playable, error) {
	metadata, err := t.Metadata()
	if err != nil {
		return Playable{}, err
	}
	return Playable{URI: t.URI(), Metadata: metadata, Title: t.Title}, nil
}

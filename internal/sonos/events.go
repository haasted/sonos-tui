package sonos

import (
	"encoding/xml"
	"fmt"
	"html"
	"strconv"
)

// AVTransportEvent is a parsed snapshot of AVTransport's LastChange payload.
// It's a snapshot, not a diff: Sonos resends the speaker's entire current
// state on every change, not just what changed (see ADR-0002) — comparing
// against a previously received AVTransportEvent is how a caller would
// determine what actually changed.
type AVTransportEvent struct {
	TransportState string // "PLAYING", "PAUSED_PLAYBACK", "STOPPED", "TRANSITIONING", ...
	PlayMode       string // "NORMAL", "SHUFFLE_NOREPEAT", "REPEAT_ALL", ...
	RelTime        string // "H:MM:SS"; rarely present — see ParseAVTransportEvent

	Track          int
	NumberOfTracks int

	Current TrackMetadata
	Next    TrackMetadata // zero value if nothing queued after Current

	QueueSource string // title of the playlist/container playback started from, if any
}

// TrackMetadata is a track's URI plus its parsed DIDL-Lite fields — the same
// shape LastChange uses for both the current and next track.
type TrackMetadata struct {
	URI         string
	Duration    string // "H:MM:SS"
	Title       string
	Artist      string
	Album       string
	AlbumArtURI string
}

// val unwraps the UPnP eventing convention of representing every state
// variable as a self-closing element with a single "val" attribute, e.g.
// <TransportState val="PLAYING"/>.
type val struct {
	Val string `xml:"val,attr"`
}

// ParseAVTransportEvent parses a GENA NOTIFY body from AVTransport's event
// service. The wire format nests two layers of escaped XML: the outer
// <e:propertyset> envelope carries <LastChange> as XML-escaped text (one
// xml.Unmarshal pass unescapes that automatically), and several fields
// inside *that* — the track metadata ones — are themselves escaped
// DIDL-Lite, needing the same extra html.UnescapeString + second
// xml.Unmarshal pattern GetPositionInfo already uses in transport.go.
func ParseAVTransportEvent(body []byte) (AVTransportEvent, error) {
	var envelope struct {
		LastChange string `xml:"property>LastChange"`
	}
	if err := xml.Unmarshal(body, &envelope); err != nil {
		return AVTransportEvent{}, fmt.Errorf("parsing propertyset envelope: %w", err)
	}

	// envelope.LastChange's root element is itself <Event> — xml.Unmarshal
	// matches doc against whatever that root is, so the path tag below only
	// names Event's *children*, not "Event" again.
	var doc struct {
		Instance struct {
			TransportState               val `xml:"TransportState"`
			CurrentPlayMode              val `xml:"CurrentPlayMode"`
			RelativeTimePosition         val `xml:"RelativeTimePosition"`
			NumberOfTracks               val `xml:"NumberOfTracks"`
			CurrentTrack                 val `xml:"CurrentTrack"`
			CurrentTrackURI              val `xml:"CurrentTrackURI"`
			CurrentTrackDuration         val `xml:"CurrentTrackDuration"`
			CurrentTrackMetaData         val `xml:"CurrentTrackMetaData"`
			NextTrackURI                 val `xml:"NextTrackURI"`
			NextTrackMetaData            val `xml:"NextTrackMetaData"`
			EnqueuedTransportURIMetaData val `xml:"EnqueuedTransportURIMetaData"`
		} `xml:"InstanceID"`
	}
	if err := xml.Unmarshal([]byte(envelope.LastChange), &doc); err != nil {
		return AVTransportEvent{}, fmt.Errorf("parsing LastChange: %w", err)
	}
	instance := doc.Instance

	current := parseDIDLTrack(instance.CurrentTrackMetaData.Val)
	current.URI = instance.CurrentTrackURI.Val
	current.Duration = instance.CurrentTrackDuration.Val

	next := parseDIDLTrack(instance.NextTrackMetaData.Val)
	next.URI = instance.NextTrackURI.Val

	queueSource := parseDIDLTrack(instance.EnqueuedTransportURIMetaData.Val).Title

	return AVTransportEvent{
		TransportState: instance.TransportState.Val,
		PlayMode:       instance.CurrentPlayMode.Val,
		RelTime:        instance.RelativeTimePosition.Val, // rarely sent — LastChange usually omits it (see AVTransportEvent doc)
		Track:          atoiOrZero(instance.CurrentTrack.Val),
		NumberOfTracks: atoiOrZero(instance.NumberOfTracks.Val),
		Current:        current,
		Next:           next,
		QueueSource:    queueSource,
	}, nil
}

// parseDIDLTrack parses an escaped DIDL-Lite <item> blob — used for both
// track metadata (CurrentTrackMetaData/NextTrackMetaData) and queue source
// metadata (EnqueuedTransportURIMetaData), which share the same <item>
// shape even though the latter actually describes a playlist container.
// Also used by GetPositionInfo in transport.go for the same TrackMetaData field.
func parseDIDLTrack(escaped string) TrackMetadata {
	if escaped == "" || escaped == "NOT_IMPLEMENTED" {
		return TrackMetadata{}
	}

	var doc struct {
		Item struct {
			Title       string `xml:"title"`
			Creator     string `xml:"creator"`
			Album       string `xml:"album"`
			AlbumArtURI string `xml:"albumArtURI"`
		} `xml:"item"`
	}
	if err := xml.Unmarshal([]byte(unescapeUntilStable(escaped)), &doc); err != nil {
		return TrackMetadata{}
	}

	return TrackMetadata{
		Title:       unescapeUntilStable(doc.Item.Title),
		Artist:      unescapeUntilStable(doc.Item.Creator),
		Album:       unescapeUntilStable(doc.Item.Album),
		AlbumArtURI: doc.Item.AlbumArtURI,
	}
}

// atoiOrZero parses a UPnP integer field, defaulting to 0 rather than
// failing the whole event just because one numeric field was malformed.
func atoiOrZero(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// unescapeUntilStable repeatedly HTML-unescapes s until it stops changing
// (bounded, to avoid a pathological loop). DIDL-Lite metadata ends up
// escaped a vendor-specific number of times depending on which response it
// came from — unescaping to a fixed point is more robust than assuming a
// fixed pass count, and a no-op on already-clean text.
func unescapeUntilStable(s string) string {
	for range 4 {
		next := html.UnescapeString(s)
		if next == s {
			return s
		}
		s = next
	}
	return s
}

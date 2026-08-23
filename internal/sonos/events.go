package sonos

import (
	"encoding/xml"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"
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

var _ slog.LogValuer = AVTransportEvent{}

// LogValue reports the fields most worth seeing in a log line — the same
// set handleNotify (cmd/listening) used to assemble by hand at the call
// site before this existed.
func (e AVTransportEvent) LogValue() slog.Value {
	attrs := []slog.Attr{
		slog.String("state", e.TransportState),
		slog.String("track", fmt.Sprintf("%d/%d", e.Track, e.NumberOfTracks)),
		slog.Any("current", e.Current),
	}
	if e.Next.Title != "" {
		attrs = append(attrs, slog.String("nextTitle", e.Next.Title))
	}
	if e.QueueSource != "" {
		attrs = append(attrs, slog.String("queue", e.QueueSource))
	}
	return slog.GroupValue(attrs...)
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

var _ slog.LogValuer = TrackMetadata{}

func (m TrackMetadata) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("title", m.Title),
		slog.String("artist", m.Artist),
		slog.String("album", m.Album),
	)
}

// val unwraps the UPnP eventing convention of representing every state
// variable as a self-closing element with a single "val" attribute, e.g.
// <TransportState val="PLAYING"/>.
type val struct {
	Val string `xml:"val,attr"`
}

// ParseAVTransportEvent parses a GENA NOTIFY body from AVTransport's event
// service.
//
// <LastChange>'s content is nominally XML-escaped text per the GENA/UPnP
// convention (<LastChange>&lt;Event ...&gt;...</LastChange>), but real
// Sonos hardware has been observed sending it as literal, unescaped nested
// XML instead (<LastChange><Event>...</Event></LastChange>) — apparently
// depending on circumstances, since the same speaker sends both forms at
// different times. A plain string field only captures text content, so it
// silently dropped the entire payload for the unescaped form (only
// whitespace text nodes got captured, and everything downstream came back
// empty). ,innerxml captures the raw bytes verbatim either way.
//
// That raw XML must NOT be blindly unescaped before parsing, though: valid
// XML legitimately contains entities like &amp; (e.g. inside URLs) that
// have to stay escaped for the document to remain well-formed — the parser
// itself correctly resolves them while reading attribute/text values.
// Unescaping the whole document up front corrupts those into bare,
// invalid &s. So: try parsing the raw bytes as-is first (succeeds
// immediately for the real-nested-XML form); only fall back to exactly one
// html.UnescapeString pass + a retry if that fails, meaning the content
// was actually the escaped-as-text form instead. One pass, not a loop —
// looping past that point would strip the same legitimate entities
// (&amp; inside a URL, say) that the now-freed inner XML still needs.
func ParseAVTransportEvent(body []byte) (AVTransportEvent, error) {
	var envelope struct {
		Property struct {
			LastChange struct {
				InnerXML string `xml:",innerxml"`
			} `xml:"LastChange"`
		} `xml:"property"`
	}
	if err := xml.Unmarshal(body, &envelope); err != nil {
		return AVTransportEvent{}, fmt.Errorf("parsing propertyset envelope: %w", err)
	}
	lastChange := envelope.Property.LastChange.InnerXML

	// lastChange's root element is itself <Event> — xml.Unmarshal matches
	// doc against whatever that root is, so the path tag below only names
	// Event's *children*, not "Event" again.
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
	if err := xml.Unmarshal([]byte(lastChange), &doc); err != nil {
		if err := xml.Unmarshal([]byte(html.UnescapeString(lastChange)), &doc); err != nil {
			return AVTransportEvent{}, fmt.Errorf("parsing LastChange: %w", err)
		}
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

// parseDIDLTrack parses a DIDL-Lite <item> blob — used for both track
// metadata (CurrentTrackMetaData/NextTrackMetaData) and queue source
// metadata (EnqueuedTransportURIMetaData), which share the same <item>
// shape even though the latter actually describes a playlist container.
// Also used by GetPositionInfo in transport.go for the same TrackMetaData field.
//
// raw's escaping depth varies by caller (see ParseAVTransportEvent's
// LastChange handling for the same issue one level up), so the same
// try-raw-then-one-unescape-pass approach applies here too.
func parseDIDLTrack(raw string) TrackMetadata {
	if raw == "" || raw == "NOT_IMPLEMENTED" {
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
	if err := xml.Unmarshal([]byte(raw), &doc); err != nil {
		if err := xml.Unmarshal([]byte(html.UnescapeString(raw)), &doc); err != nil {
			return TrackMetadata{}
		}
	}

	return TrackMetadata{
		Title:       normalizeText(doc.Item.Title),
		Artist:      normalizeText(doc.Item.Creator),
		Album:       normalizeText(doc.Item.Album),
		AlbumArtURI: doc.Item.AlbumArtURI, // a URL — never whitespace-wrapped, and Fields/Join would corrupt one that used '+' spacing
	}
}

// normalizeText collapses whitespace runs (including newlines) into single
// spaces. Real Sonos hardware has been observed word-wrapping long text
// fields at some column width, inserting literal newlines/indentation
// *into* the text itself, e.g. "<dc:title>Frankie\n            Sinatra".
func normalizeText(s string) string {
	return strings.Join(strings.Fields(s), " ")
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

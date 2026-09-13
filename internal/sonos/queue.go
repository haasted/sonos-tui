package sonos

import (
	"encoding/xml"
	"fmt"
	"html"
	"log/slog"
)

// contentDirectoryService is the MediaServer half of a speaker. Everything
// else this package talks to lives under MediaRenderer; the queue does not.
const (
	contentDirectoryService = "urn:schemas-upnp-org:service:ContentDirectory:1"
	contentDirectoryPath    = "/MediaServer/ContentDirectory/Control"
)

// queueObjectID is the speaker's own playback queue, as opposed to a playlist
// or a music-service container.
const queueObjectID = "Q:0"

// browsePageSize is how many items one Browse asks for. Sonos may return
// fewer, which is why GetQueue pages rather than trusting a single call.
const browsePageSize = 100

// QueueItem is one entry in a speaker's queue.
type QueueItem struct {
	Position int // 1-based, matching AVTransport's CurrentTrack
	TrackMetadata
}

// GetQueue reads the speaker's queue, paging until it has everything.
//
// Browse returns at most RequestedCount items and reports how many it sent and
// how many exist, so a single call is only ever correct by luck on a short
// queue.
func (c *SonosClient) GetQueue() ([]QueueItem, error) {
	var queue []QueueItem

	for {
		page, err := c.browseQueue(len(queue), browsePageSize)
		if err != nil {
			return nil, err
		}

		for _, item := range page.items {
			queue = append(queue, QueueItem{
				Position:      len(queue) + 1,
				TrackMetadata: item,
			})
		}

		// Stop on a page that returned nothing, not only on reaching the
		// claimed total: a speaker reporting a total it will not serve would
		// otherwise loop forever.
		if page.returned == 0 || len(queue) >= page.total {
			// The claim is only worth reporting when it is wrong. Returning it
			// alongside the items would be a second count that equals len in
			// every correct case and differs only during a fault — which is
			// precisely when a caller would reach for the wrong one.
			if len(queue) != page.total {
				slog.Warn("speaker served a different number of queue items than it claimed",
					"served", len(queue), "claimed", page.total)
			}
			return queue, nil
		}
	}
}

// browsePage is one Browse response: the items it carried, and the counts that
// say whether more remain.
type browsePage struct {
	items    []TrackMetadata
	returned int
	total    int
}

func (c *SonosClient) browseQueue(start, count int) (browsePage, error) {
	// ObjectID is a constant today, but it is the field that becomes a playlist
	// identifier later. Marshalling escapes it, where a format string would not.
	data, err := c.Do(contentDirectoryPath, contentDirectoryService, "Browse", browseAction{
		XMLName:        xml.Name{Local: "u:Browse"},
		Namespace:      contentDirectoryService,
		ObjectID:       queueObjectID,
		BrowseFlag:     "BrowseDirectChildren",
		Filter:         "*",
		StartingIndex:  start,
		RequestedCount: count,
	})
	if err != nil {
		return browsePage{}, fmt.Errorf("browsing queue: %w", err)
	}
	return parseBrowseResponse(data)
}

// parseBrowseResponse unwraps the SOAP envelope and the escaped DIDL-Lite
// document inside its Result element.
func parseBrowseResponse(data []byte) (browsePage, error) {
	var envelope struct {
		Body struct {
			Response struct {
				Result         string `xml:"Result"`
				NumberReturned int    `xml:"NumberReturned"`
				TotalMatches   int    `xml:"TotalMatches"`
			} `xml:"BrowseResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return browsePage{}, fmt.Errorf("parsing Browse envelope: %w", err)
	}

	response := envelope.Body.Response
	return browsePage{
		items:    parseDIDLTracks(response.Result),
		returned: response.NumberReturned,
		total:    response.TotalMatches,
	}, nil
}

// parseDIDLTracks parses every <item> in a DIDL-Lite document. parseDIDLTrack
// handles a single item and cannot be reused: unmarshalling into one struct
// field keeps only the first, which on a queue means silently losing all but
// one track.
func parseDIDLTracks(raw string) []TrackMetadata {
	if raw == "" || raw == "NOT_IMPLEMENTED" {
		return nil
	}

	var doc struct {
		Items []struct {
			Title       string `xml:"title"`
			Creator     string `xml:"creator"`
			Album       string `xml:"album"`
			AlbumArtURI string `xml:"albumArtURI"`
			Res         struct {
				Duration string `xml:"duration,attr"`
				URI      string `xml:",chardata"`
			} `xml:"res"`
		} `xml:"item"`
	}
	// Same escaped-or-nested ambiguity as everywhere else: try the raw bytes
	// first, then exactly one unescape pass.
	if err := xml.Unmarshal([]byte(raw), &doc); err != nil {
		if err := xml.Unmarshal([]byte(html.UnescapeString(raw)), &doc); err != nil {
			return nil
		}
	}

	tracks := make([]TrackMetadata, 0, len(doc.Items))
	for _, item := range doc.Items {
		tracks = append(tracks, TrackMetadata{
			URI:         item.Res.URI,
			Duration:    item.Res.Duration,
			Title:       normalizeText(item.Title),
			Artist:      normalizeText(item.Creator),
			Album:       normalizeText(item.Album),
			AlbumArtURI: item.AlbumArtURI,
		})
	}
	return tracks
}

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

// Queue is a speaker's queue and the version the speaker gave for it, so a
// caller can tell a copy it read earlier from the one the speaker holds now.
type Queue struct {
	Items    []QueueItem
	UpdateID int
}

// GetQueue reads the speaker's queue, paging until it has everything.
//
// Browse returns at most RequestedCount items and reports how many it sent and
// how many exist, so a single call is only ever correct by luck on a short
// queue.
func (c *SonosClient) GetQueue() (Queue, error) {
	queue := Queue{}

	for {
		page, err := c.browseQueue(len(queue.Items), browsePageSize)
		if err != nil {
			return Queue{}, err
		}

		// The id of the first page is the queue this read began against. A
		// later page reporting another means the queue changed underneath the
		// paging, so the result is a mix of two queues rather than either.
		if len(queue.Items) == 0 {
			queue.UpdateID = page.updateID
		} else if page.updateID != queue.UpdateID {
			slog.Warn("the queue changed while it was being read",
				"began", queue.UpdateID, "now", page.updateID)
			queue.UpdateID = page.updateID
		}

		for _, item := range page.items {
			queue.Items = append(queue.Items, QueueItem{
				Position:      len(queue.Items) + 1,
				TrackMetadata: item,
			})
		}

		// Stop on a page that returned nothing, not only on reaching the
		// claimed total: a speaker reporting a total it will not serve would
		// otherwise loop forever.
		if page.returned == 0 || len(queue.Items) >= page.total {
			// The claim is only worth reporting when it is wrong. Returning it
			// alongside the items would be a second count that equals len in
			// every correct case and differs only during a fault — which is
			// precisely when a caller would reach for the wrong one.
			if len(queue.Items) != page.total {
				slog.Warn("speaker served a different number of queue items than it claimed",
					"served", len(queue.Items), "claimed", page.total)
			}
			return queue, nil
		}
	}
}

// RemoveTrackFromQueue takes one track out of the speaker's queue. position is
// 1-based, as QueueItem.Position counts.
//
// The queue is READ from ContentDirectory under /MediaServer and CHANGED
// through AVTransport under /MediaRenderer. Two services for one queue, and
// this is the second of them.
//
// updateID is the version the caller believes the queue is at. A speaker that
// checks it refuses a command aimed at a queue that has since changed, which
// is the difference between removing the track the reader saw and removing
// whatever moved into its place.
func (c *SonosClient) RemoveTrackFromQueue(position, updateID int) error {
	if position < 1 {
		return fmt.Errorf("removing from the queue: position %d is not a track", position)
	}

	_, err := c.avTransport("RemoveTrackFromQueue", removeTrackAction{
		XMLName:   xml.Name{Local: "u:RemoveTrackFromQueue"},
		Namespace: avtransportService,
		ObjectID:  fmt.Sprintf("%s/%d", queueObjectID, position),
		UpdateID:  updateID,
	})
	if err != nil {
		return fmt.Errorf("removing track %d from the queue: %w", position, err)
	}
	return nil
}

// MoveQueueTrack moves one track to another place in the queue. Both positions
// are 1-based, as QueueItem.Position counts, and to is where the reader wants
// the track to end up.
//
// THE OFF-BY-ONE. InsertBefore is read against the queue with the track still
// in it, so moving DOWN has to name one place further on than the destination:
// lifting the track out afterwards shifts everything below it up by one. Moving
// up needs no such correction. This is the convention the action is documented
// with and is NOT yet verified against a speaker.
func (c *SonosClient) MoveQueueTrack(from, to, updateID int) error {
	if from < 1 || to < 1 {
		return fmt.Errorf("moving in the queue: %d to %d is not a move between tracks", from, to)
	}
	if from == to {
		return nil
	}

	insertBefore := to
	if to > from {
		insertBefore = to + 1
	}

	_, err := c.avTransport("ReorderTracksInQueue", reorderTracksAction{
		XMLName:        xml.Name{Local: "u:ReorderTracksInQueue"},
		Namespace:      avtransportService,
		StartingIndex:  from,
		NumberOfTracks: 1,
		InsertBefore:   insertBefore,
		UpdateID:       updateID,
	})
	if err != nil {
		return fmt.Errorf("moving track %d to %d in the queue: %w", from, to, err)
	}
	return nil
}

// browsePage is one Browse response: the items it carried, and the counts that
// say whether more remain.
type browsePage struct {
	items    []TrackMetadata
	returned int
	total    int
	updateID int // the container's version at the moment it was browsed
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
				UpdateID       int    `xml:"UpdateID"`
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
		updateID: response.UpdateID,
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

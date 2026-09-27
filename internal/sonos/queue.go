package sonos

import (
	"context"
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

// The Queue service is the second place a speaker takes queue writes, and the
// only one that answers with the version it created. AVTransport's reorder and
// single-track removal return nothing, so a caller there cannot send a second
// write until a Browse or an event says what the version is now.
//
// Every speaker asked has it, from a 2018 Amp to a 2023 Era, but that sample
// holds no S1-era player. If one ever turns up it will refuse these actions,
// and AVTransport is the way back: its RemoveTrackRangeFromQueue returns the
// new version too, and only a reorder would have to re-read between writes.
// See SONOS-NOTES.md.
const (
	queueService = "urn:schemas-sonos-com:service:Queue:1"
	queuePath    = "/MediaRenderer/Queue/Control"
)

// localQueueID is the speaker's own queue, which the Queue service names with
// a number where ContentDirectory names it with the "Q:0" object id.
const localQueueID = 0

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
func (c *SonosClient) GetQueue(ctx context.Context) (Queue, error) {
	queue := Queue{}

	for {
		page, err := c.browseQueue(ctx, len(queue.Items), browsePageSize)
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

// RemoveTrackRange takes a run of tracks out of the queue and returns the
// version the speaker moved to.
//
// The version comes back so a caller removing several runs can carry it into
// the next call. Waiting for the ContentDirectory event instead is what makes
// a second removal arrive stale, and the speaker refuses a stale one.
//
// start is 1-based, as QueueItem.Position counts. A zero updateID is accepted
// by the speaker and means "do not check", which is never what a caller wants:
// the check is the only thing between removing the track the reader saw and
// removing whatever has since moved into its place.
func (c *SonosClient) RemoveTrackRange(ctx context.Context, start, count, updateID int) (int, error) {
	if start < 1 {
		return 0, fmt.Errorf("removing from the queue: position %d is not a track", start)
	}
	if count < 1 {
		return 0, fmt.Errorf("removing from the queue: %d tracks is not a run", count)
	}

	data, err := c.queueControl(ctx, "RemoveTrackRange", removeTrackRangeAction{
		XMLName:        xml.Name{Local: "u:RemoveTrackRange"},
		Namespace:      queueService,
		QueueID:        localQueueID,
		UpdateID:       updateID,
		StartingIndex:  start,
		NumberOfTracks: count,
	})
	if err != nil {
		return 0, fmt.Errorf("removing %d tracks from the queue at %d: %w", count, start, err)
	}
	return parseNewUpdateID(data)
}

// MoveQueueTrack moves one track to another place in the queue and returns the
// version the speaker moved to. Both positions are 1-based, as
// QueueItem.Position counts, and to is where the reader wants it to end up.
//
// THE OFF-BY-ONE. InsertBefore is read against the queue with the track still
// in it, so moving DOWN has to name one place further on than the destination:
// lifting the track out afterwards shifts everything below it up by one. Moving
// up needs no such correction. Verified against a speaker twice: a track sent
// from index 1 with InsertBefore 3 lands at position 2, and one sent with
// InsertBefore one PAST the last track lands at the very bottom rather than
// short of it -- see SONOS-NOTES.md.
func (c *SonosClient) MoveQueueTrack(ctx context.Context, from, to, updateID int) (int, error) {
	if from < 1 || to < 1 {
		return 0, fmt.Errorf("moving in the queue: %d to %d is not a move between tracks", from, to)
	}
	if from == to {
		return updateID, nil
	}

	insertBefore := to
	if to > from {
		insertBefore = to + 1
	}

	data, err := c.queueControl(ctx, "ReorderTracks", reorderTracksAction{
		XMLName:        xml.Name{Local: "u:ReorderTracks"},
		Namespace:      queueService,
		QueueID:        localQueueID,
		StartingIndex:  from,
		NumberOfTracks: 1,
		InsertBefore:   insertBefore,
		UpdateID:       updateID,
	})
	if err != nil {
		return 0, fmt.Errorf("moving track %d to %d in the queue: %w", from, to, err)
	}
	return parseNewUpdateID(data)
}

func (c *SonosClient) queueControl(ctx context.Context, action string, payload any) ([]byte, error) {
	return c.Do(ctx, queuePath, queueService, action, payload)
}

// parseNewUpdateID reads the version a Queue write answers with. The element
// sits inside a response named for the action, so the response element is
// matched by position rather than by a name that differs per call.
func parseNewUpdateID(data []byte) (int, error) {
	var envelope struct {
		Body struct {
			Response struct {
				NewUpdateID int `xml:"NewUpdateID"`
			} `xml:",any"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return 0, fmt.Errorf("parsing the new queue version: %w", err)
	}
	return envelope.Body.Response.NewUpdateID, nil
}

// browsePage is one Browse response: the items it carried, and the counts that
// say whether more remain.
type browsePage struct {
	items    []TrackMetadata
	returned int
	total    int
	updateID int // the container's version at the moment it was browsed
}

func (c *SonosClient) browseQueue(ctx context.Context, start, count int) (browsePage, error) {
	// ObjectID is a constant today, but it is the field that becomes a playlist
	// identifier later. Marshalling escapes it, where a format string would not.
	data, err := c.Do(ctx, contentDirectoryPath, contentDirectoryService, "Browse", browseAction{
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

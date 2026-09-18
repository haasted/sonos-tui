package sonos

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// ContainerUpdate names one container that changed, with the update id the
// speaker now holds for it. A reader that kept the previous id can tell a
// change it has already seen from one it has not.
type ContainerUpdate struct {
	ID       string
	UpdateID int
}

// ContentDirectoryEvent is what a ContentDirectory NOTIFY carries. Unlike the
// two MediaRenderer services it has no LastChange: each evented variable is a
// property of its own, so there is no second document to unescape.
type ContentDirectoryEvent struct {
	SystemUpdateID int  // rises on any change anywhere in the directory
	HasSystem      bool // false when the event carried no SystemUpdateID

	// Containers are the containers the event named, in the order given.
	// Empty on an event that reported only a system-wide change.
	Containers []ContainerUpdate
}

var _ slog.LogValuer = ContentDirectoryEvent{}

func (e ContentDirectoryEvent) LogValue() slog.Value {
	names := make([]string, 0, len(e.Containers))
	for _, c := range e.Containers {
		names = append(names, fmt.Sprintf("%s=%d", c.ID, c.UpdateID))
	}
	return slog.GroupValue(
		slog.Int("system", e.SystemUpdateID),
		slog.String("containers", strings.Join(names, " ")),
	)
}

// QueueUpdate reports the new update id of this speaker's own queue, and
// whether the event named it at all. An event about some other container says
// nothing about the queue and must not be read as saying it is unchanged.
func (e ContentDirectoryEvent) QueueUpdate() (int, bool) {
	for _, c := range e.Containers {
		if c.ID == queueObjectID {
			return c.UpdateID, true
		}
	}
	return 0, false
}

// ParseContentDirectoryEvent parses a GENA NOTIFY body from ContentDirectory's
// event service.
func ParseContentDirectoryEvent(body []byte) (ContentDirectoryEvent, error) {
	var envelope struct {
		Properties []struct {
			SystemUpdateID     *int    `xml:"SystemUpdateID"`
			ContainerUpdateIDs *string `xml:"ContainerUpdateIDs"`
		} `xml:"property"`
	}
	if err := xml.Unmarshal(body, &envelope); err != nil {
		return ContentDirectoryEvent{}, fmt.Errorf("parsing propertyset envelope: %w", err)
	}

	var event ContentDirectoryEvent
	for _, property := range envelope.Properties {
		if property.SystemUpdateID != nil {
			event.SystemUpdateID, event.HasSystem = *property.SystemUpdateID, true
		}
		if property.ContainerUpdateIDs != nil {
			event.Containers = append(event.Containers, parseContainerUpdates(*property.ContainerUpdateIDs)...)
		}
	}
	return event, nil
}

// parseContainerUpdates reads the comma-separated pairs of ContainerUpdateIDs,
// e.g. "Q:0,42,SQ:3,7". A container id holds a colon but never a comma, so the
// pairs are positional rather than delimited by anything of their own.
//
// A trailing or unreadable pair is dropped rather than failing the event: the
// pairs before it are still true, and they are the ones a caller acts on.
func parseContainerUpdates(raw string) []ContainerUpdate {
	fields := strings.Split(raw, ",")

	var updates []ContainerUpdate
	for i := 0; i+1 < len(fields); i += 2 {
		id := strings.TrimSpace(fields[i])
		updateID, err := strconv.Atoi(strings.TrimSpace(fields[i+1]))
		if id == "" || err != nil {
			slog.Debug("unreadable container update", "pair", fields[i]+","+fields[i+1])
			continue
		}
		updates = append(updates, ContainerUpdate{ID: id, UpdateID: updateID})
	}
	return updates
}

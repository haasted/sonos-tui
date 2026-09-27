package sonos

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
)

// ZoneGroupTopology says how the whole household is grouped. It is the fourth
// service this package speaks to, and the only one outside /MediaRenderer and
// /MediaServer.
const (
	topologyService = "urn:schemas-upnp-org:service:ZoneGroupTopology:1"
	topologyPath    = "/ZoneGroupTopology/Control"
)

// ZoneMember is one speaker as the topology reports it.
//
// Three fields of the twenty-nine a member carries. The rest describe its
// firmware, its wifi and its microphone, none of which is this package's
// business: a speaker's condition is not how the household is arranged.
type ZoneMember struct {
	// ID is the speaker's bare RINCON identifier, which is what every other
	// part of the topology refers to it by.
	ID string

	RoomName string
	Location string // its device_description.xml, which carries its address
}

// ZoneGroup is a set of speakers playing as one.
type ZoneGroup struct {
	// Coordinator is the ID of the speaker that owns the group: its queue is
	// the group's queue, and transport commands belong to it.
	Coordinator string

	// ID is what the speaker calls this group, "<coordinator>:<number>".
	// Nothing here keys on it -- the coordinator is the identity, verified by
	// two captures: a speaker joining another's group left that group's id
	// untouched. Kept because the number cannot be worked out again.
	ID string

	// Members are every speaker in the group, the coordinator among them.
	Members []ZoneMember
}

// Topology is how one household is grouped, as one of its speakers reported
// it. Any speaker answers for the whole household, so one read is enough.
//
// ONE household. A network can hold more than one Sonos system, and discovery
// finds speakers across all of them: a speaker this topology does not mention
// is not necessarily gone, it may simply belong to another household. Callers
// matching speakers to groups must allow for finding neither.
//
// Bonded sets -- a stereo pair, a surround set, a sub -- are OUT OF SCOPE.
// Several devices in one logical zone are marked somewhere in this document,
// and a command sent to a satellite misbehaves, but no capture of one exists
// to write against and a guessed field would read as tested.
type Topology struct {
	Groups []ZoneGroup
}

// GetZoneGroupState reads how the household is grouped.
func (c *SonosClient) GetZoneGroupState(ctx context.Context) (Topology, error) {
	data, err := c.Do(ctx, topologyPath, topologyService, "GetZoneGroupState",
		zoneGroupStateAction{
			XMLName:   xml.Name{Local: "u:GetZoneGroupState"},
			Namespace: topologyService,
		})
	if err != nil {
		return Topology{}, fmt.Errorf("reading the zone group topology: %w", err)
	}

	var envelope struct {
		Body struct {
			Response struct {
				State string `xml:"ZoneGroupState"`
			} `xml:"GetZoneGroupStateResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return Topology{}, fmt.Errorf("parsing the topology envelope: %w", err)
	}
	return parseZoneGroupState(envelope.Body.Response.State)
}

// parseZoneGroupState reads the document the service answers with.
func parseZoneGroupState(raw string) (Topology, error) {
	if raw == "" {
		return Topology{}, fmt.Errorf("the response carried no ZoneGroupState")
	}

	var doc struct {
		Groups []struct {
			Coordinator string `xml:"Coordinator,attr"`
			ID          string `xml:"ID,attr"`
			Members     []struct {
				UUID     string `xml:"UUID,attr"`
				ZoneName string `xml:"ZoneName,attr"`
				Location string `xml:"Location,attr"`
			} `xml:"ZoneGroupMember"`
		} `xml:"ZoneGroups>ZoneGroup"`
	}
	// Same escaped-or-nested ambiguity as everywhere else: try the raw bytes
	// first, then exactly one unescape pass.
	if err := xml.Unmarshal([]byte(raw), &doc); err != nil {
		if err := xml.Unmarshal([]byte(html.UnescapeString(raw)), &doc); err != nil {
			return Topology{}, fmt.Errorf("parsing the zone group state: %w", err)
		}
	}

	topology := Topology{Groups: make([]ZoneGroup, 0, len(doc.Groups))}
	for _, group := range doc.Groups {
		members := make([]ZoneMember, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, ZoneMember{
				ID: member.UUID, RoomName: member.ZoneName, Location: member.Location,
			})
		}
		topology.Groups = append(topology.Groups, ZoneGroup{
			Coordinator: group.Coordinator, ID: group.ID, Members: members,
		})
	}
	return topology, nil
}

// zoneGroupStateFrom pulls the grouping out of a NOTIFY body, and empty
// when that body carried none of it.
func zoneGroupStateFrom(body []byte) (string, error) {
	var propertyset struct {
		Properties []struct {
			State string `xml:"ZoneGroupState"`
		} `xml:"property"`
	}
	if err := xml.Unmarshal(body, &propertyset); err != nil {
		return "", fmt.Errorf("parsing a topology event: %w", err)
	}

	for _, property := range propertyset.Properties {
		if property.State != "" {
			return property.State, nil
		}
	}
	return "", nil
}

// GroupOf is the group a speaker is in, false when this household does not
// hold it -- it may be off, or it may belong to another Sonos system.
func (t Topology) GroupOf(id string) (ZoneGroup, bool) {
	for _, group := range t.Groups {
		for _, member := range group.Members {
			if member.ID == id {
				return group, true
			}
		}
	}
	return ZoneGroup{}, false
}

// CoordinatorOf is the speaker that owns the queue a speaker is playing from,
// which is itself when it is alone or coordinating.
func (t Topology) CoordinatorOf(id string) (string, bool) {
	group, found := t.GroupOf(id)
	if !found {
		return "", false
	}
	return group.Coordinator, true
}

// Grouped reports a group of more than one speaker, where a command has to
// choose between the coordinator and the speaker in front of the reader.
func (g ZoneGroup) Grouped() bool { return len(g.Members) > 1 }

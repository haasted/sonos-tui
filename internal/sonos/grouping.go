package sonos

import (
	"context"
	"encoding/xml"
	"fmt"
)

// groupURIScheme is how a speaker is told to follow another: its transport
// is pointed at the coordinator instead of at anything playable.
const groupURIScheme = "x-rincon:"

// JoinGroup makes this speaker play whatever the coordinator is playing.
//
// There is no create-group action. A group is made by pointing a speaker's
// transport at another speaker, and grouping three is two of these.
//
// IT THROWS THE JOINER'S STATE AWAY. The speaker abandons its own queue and
// whatever it was playing, adopts the coordinator's, and leaving gives none
// of it back. Nothing here warns anybody: a caller doing this on a reader's
// behalf should have said so first.
//
// It also returns before the household agrees. The speaker accepts the
// command at once and the topology event follows, so a caller that redraws
// from its own idea of the grouping is drawing ahead of the speakers.
func (c *SonosClient) JoinGroup(ctx context.Context, coordinator string) error {
	if coordinator == "" {
		return fmt.Errorf("joining a group: no coordinator named")
	}

	_, err := c.avTransport(ctx, "SetAVTransportURI", setAVTransportURIAction{
		XMLName:    xml.Name{Local: "u:SetAVTransportURI"},
		Namespace:  avtransportService,
		CurrentURI: groupURIScheme + coordinator,
	})
	if err != nil {
		return fmt.Errorf("joining %s: %w", coordinator, err)
	}
	return nil
}

// LeaveGroup takes this speaker out of whatever group it is in, leaving it
// standing alone with a queue of its own again.
//
// Sound for any member, the coordinator included: a group whose coordinator
// leaves is not dissolved, the speaker promotes one of the others and the
// rest play on. Which one is the speakers' business, not ours.
func (c *SonosClient) LeaveGroup(ctx context.Context) error {
	_, err := c.avTransport(ctx, "BecomeCoordinatorOfStandaloneGroup",
		becomeCoordinatorAction{
			XMLName:   xml.Name{Local: "u:BecomeCoordinatorOfStandaloneGroup"},
			Namespace: avtransportService,
		})
	if err != nil {
		return fmt.Errorf("leaving a group: %w", err)
	}
	return nil
}

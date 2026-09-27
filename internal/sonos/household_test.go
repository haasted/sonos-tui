package sonos_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: a report REPLACING what it overlaps rather than adding to it. A
// speaker describes its whole system, so an arrangement it leaves out has
// stopped being true -- appending would leave a speaker in two groups.
func TestHousehold_AReportReplacesWhatItOverlaps(t *testing.T) {
	var house sonos.Household
	house = house.Apply(reported(group("a", "a", "b"), group("c", "c")))

	// The pair split up.
	house = house.Apply(reported(group("a", "a"), group("b", "b"), group("c", "c")))

	for _, id := range []string{"a", "b", "c"} {
		found, ok := house.GroupOf(id)
		require.True(t, ok, "%s went missing", id)
		assert.Len(t, found.Members, 1, "%s is still in a group of more than itself", id)
	}
}

// Case: another Sonos system on the same network. Its report says nothing
// about the first, so the first must survive it -- the two are separate
// households and neither speaks for the other.
func TestHousehold_AnotherSystemLeavesTheFirstAlone(t *testing.T) {
	var house sonos.Household
	house = house.Apply(reported(group("a", "a", "b")))
	house = house.Apply(reported(group("z", "z")))

	coordinator, found := house.CoordinatorOf("b")
	require.True(t, found, "the first system was forgotten")
	assert.Equal(t, "a", coordinator)

	_, found = house.GroupOf("z")
	assert.True(t, found, "the second system was not taken in")
}

// Case: the same arrangement reported by two speakers, which is what one
// change produces. They disagree on order -- each lists itself first -- so
// folding both in must not leave the household holding it twice.
func TestHousehold_TwoReportsOfOneArrangementAreOneArrangement(t *testing.T) {
	var house sonos.Household
	house = house.Apply(reported(group("a", "a", "b")))
	house = house.Apply(reported(group("a", "b", "a"))) // as the other one writes it

	found, ok := house.GroupOf("b")
	require.True(t, ok)
	assert.Len(t, found.Members, 2)
	assert.Equal(t, "a", found.Coordinator)
	assert.Len(t, house.Drives("a"), 1, "the arrangement was taken in twice")
}

// Case: the question a caller actually has. Not whether a speaker is in a
// group, but whether its playback belongs to somebody else.
func TestHousehold_FollowsAnswersForTheDrivenOnly(t *testing.T) {
	house := sonos.Household{}.Apply(reported(group("a", "a", "b"), group("c", "c")))

	driver, follows := house.Follows("b")
	assert.True(t, follows)
	assert.Equal(t, "a", driver)

	_, follows = house.Follows("a")
	assert.False(t, follows, "the speaker in charge follows nobody")

	_, follows = house.Follows("c")
	assert.False(t, follows, "a speaker alone follows nobody")

	_, follows = house.Follows("unknown")
	assert.False(t, follows, "a speaker nobody mentioned cannot be said to follow")
}

// Case: who a speaker is driving, which is what its tab names underneath it.
func TestHousehold_DrivesNamesTheOthersOnly(t *testing.T) {
	house := sonos.Household{}.Apply(reported(group("a", "a", "b", "c")))

	driven := house.Drives("a")
	require.Len(t, driven, 2)
	for _, member := range driven {
		assert.NotEqual(t, "a", member.ID, "a speaker was listed as driving itself")
	}

	assert.Empty(t, house.Drives("b"), "a driven speaker drives nobody")
}

// Case: nothing heard yet, which must be told from a speaker standing
// alone. Every speaker reads as ungrouped either way, and only one of those
// is something we know.
func TestHousehold_KnowsWhetherItHasHeardAnything(t *testing.T) {
	var house sonos.Household
	assert.False(t, house.Known())

	assert.True(t, house.Apply(reported(group("a", "a"))).Known())
}

// group is one group: the coordinator first, then everyone in it by id.
func group(coordinator string, members ...string) sonos.ZoneGroup {
	zone := sonos.ZoneGroup{Coordinator: coordinator, ID: coordinator + ":1"}
	for _, id := range members {
		zone.Members = append(zone.Members, sonos.ZoneMember{ID: id, RoomName: id})
	}
	return zone
}

func reported(groups ...sonos.ZoneGroup) sonos.Topology {
	return sonos.Topology{Groups: groups}
}

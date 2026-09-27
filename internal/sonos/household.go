package sonos

import "slices"

// Household is what is known about how the speakers around here are grouped.
//
// Named for the one case that matters, though it may hold more than one: a
// network can carry several Sonos systems, each answering only for itself
// and each silent about the others. What one reports is folded in here so a
// caller can ask about any speaker it can see without first knowing which
// system that speaker belongs to.
//
// A value, copied rather than shared. Nothing here is written in place, so
// a copy taken while an event is being applied is a whole older answer
// rather than half of a newer one.
type Household struct {
	// systems holds each Sonos system's groups apart from the others'. Kept
	// apart rather than merged because the difference is real: speakers can
	// only be grouped with others of their own system, and a flat list
	// cannot say which those are.
	systems [][]ZoneGroup
}

// Apply folds in what one speaker reported.
//
// The report REPLACES the groups it overlaps rather than adding to them: the
// speaker has just described its whole system, so an arrangement of its own
// that the report leaves out has stopped being true. Groups it says nothing
// about belong to another system and are left alone.
func (h Household) Apply(reported Topology) Household {
	if len(reported.Groups) == 0 {
		return h
	}

	kept := make([][]ZoneGroup, 0, len(h.systems)+1)
	for _, system := range h.systems {
		if !sharesSpeaker(system, reported.Groups) {
			kept = append(kept, system)
		}
	}

	h.systems = append(kept, reported.Groups)
	return h
}

// Known reports that something has been heard at all, so a caller can tell
// a speaker that stands alone from one nobody has asked about yet.
func (h Household) Known() bool { return len(h.systems) > 0 }

// GroupOf is the group a speaker is in, false when nothing heard so far
// mentions it -- it may be off, or it may belong to a system that has not
// been asked.
func (h Household) GroupOf(id string) (ZoneGroup, bool) {
	for _, system := range h.systems {
		for _, group := range system {
			if holds(group, id) {
				return group, true
			}
		}
	}
	return ZoneGroup{}, false
}

// Companions is every OTHER speaker of the same Sonos system, whether or not
// it is already grouped with this one. They are the speakers this one can be
// grouped with: a system is the boundary, and one on another system is no
// more reachable than one that is switched off.
func (h Household) Companions(id string) []ZoneMember {
	system, found := h.systemOf(id)
	if !found {
		return nil
	}

	var others []ZoneMember
	for _, group := range system {
		for _, member := range group.Members {
			if member.ID != id {
				others = append(others, member)
			}
		}
	}
	return others
}

func (h Household) systemOf(id string) ([]ZoneGroup, bool) {
	for _, system := range h.systems {
		for _, group := range system {
			if holds(group, id) {
				return system, true
			}
		}
	}
	return nil, false
}

func holds(group ZoneGroup, id string) bool {
	return slices.ContainsFunc(group.Members, func(m ZoneMember) bool { return m.ID == id })
}

// CoordinatorOf is the speaker that owns the queue a speaker plays from,
// which is itself when it stands alone or coordinates.
func (h Household) CoordinatorOf(id string) (string, bool) {
	group, found := h.GroupOf(id)
	if !found {
		return "", false
	}
	return group.Coordinator, true
}

// Follows is the speaker playing through another one, and false for one that
// answers for itself. The question a caller actually has: not whether a
// speaker is in a group, but whether its playback belongs to somebody else.
func (h Household) Follows(id string) (string, bool) {
	coordinator, found := h.CoordinatorOf(id)
	if !found || coordinator == id {
		return "", false
	}
	return coordinator, true
}

// Drives is everyone a speaker is playing through to, and nothing for one
// that drives nobody. It never includes the speaker itself.
func (h Household) Drives(id string) []ZoneMember {
	group, found := h.GroupOf(id)
	if !found || group.Coordinator != id {
		return nil
	}

	driven := make([]ZoneMember, 0, len(group.Members))
	for _, member := range group.Members {
		if member.ID != id {
			driven = append(driven, member)
		}
	}
	return driven
}

// sharesSpeaker reports one system's groups holding a speaker that appears
// in another's, which is how a report is matched to the system it describes.
func sharesSpeaker(system, reported []ZoneGroup) bool {
	for _, group := range system {
		for _, other := range reported {
			for _, member := range other.Members {
				if holds(group, member.ID) {
					return true
				}
			}
		}
	}
	return false
}

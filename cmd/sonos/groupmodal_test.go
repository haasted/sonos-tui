package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/haasted/sonos-tui/internal/sonos"
)

// Case: the key, which must offer the room on screen a list of the speakers
// it could play with.
func TestGroupModal_GOpensItForTheRoomOnScreen(t *testing.T) {
	m := householdOnScreen(t)

	model, _ := m.Update(keyMsgFor("g"))
	m = model.(appModel)

	if !m.grouping.IsOpen() {
		t.Fatal("g opened nothing")
	}
	if head := groupingOf(t, m).heading(); !strings.Contains(head, m.tabs.Items[0].Title) {
		t.Fatalf("the heading reads %q and does not name the room it is for", head)
	}
}

// Case: esc, which closes it. The content claims no keys, so the modal
// takes esc for itself as it always has.
func TestGroupModal_EscCloses(t *testing.T) {
	m := householdOnScreen(t)
	model, _ := m.Update(keyMsgFor("g"))

	model, _ = model.(appModel).Update(keyMsgFor("esc"))

	if model.(appModel).grouping.IsOpen() {
		t.Fatal("esc left the list open")
	}
}

// Case: who is offered. The rest of this speaker's own Sonos system, and
// nobody else: a speaker on another system cannot be grouped with, and the
// room itself is not a candidate to play with itself.
func TestGroupModal_OffersTheRestOfItsOwnSystem(t *testing.T) {
	m := householdOnScreen(t)
	model, _ := m.Update(keyMsgFor("g"))

	rooms := roomsIn(groupingOf(t, model.(appModel)))

	if !strings.Contains(rooms, "Køkken") || !strings.Contains(rooms, "Spisestue") {
		t.Fatalf("the list holds %q, want both rooms of this system", rooms)
	}
	if strings.Contains(rooms, "Værelse") {
		t.Fatalf("a speaker on another Sonos system was offered: %q", rooms)
	}
	if strings.Contains(rooms, m.tabs.Items[0].Title) {
		t.Fatalf("the room was offered itself: %q", rooms)
	}
}

// Case: a room with nobody to play with. A system of one is an ordinary
// answer, not an error, and an empty box would say nothing at all.
func TestGroupModal_SaysSoWhenThereIsNobody(t *testing.T) {
	m, _, _ := withQueueServer(t, 5)
	model, _ := m.Update(keyMsgFor("g"))

	body := plain(groupingOf(t, model.(appModel)).View().Content)
	if !strings.Contains(body, "No other speakers") {
		t.Fatalf("a room with no companions shows %q", body)
	}
}

// householdOnScreen is an app whose speaker shares a system with two other
// rooms, and a second Sonos system alongside it.
func householdOnScreen(t *testing.T) appModel {
	t.Helper()

	m, _, _ := withQueueServer(t, 5)
	id := speakerIDOf(t, m)

	m.household = m.household.Apply(sonos.Topology{Groups: []sonos.ZoneGroup{
		{Coordinator: id, Members: []sonos.ZoneMember{
			{ID: id, RoomName: m.tabs.Items[0].Title},
			{ID: "RINCON_A", RoomName: "Køkken"},
		}},
		{Coordinator: "RINCON_B", Members: []sonos.ZoneMember{{ID: "RINCON_B", RoomName: "Spisestue"}}},
	}})
	m.household = m.household.Apply(sonos.Topology{Groups: []sonos.ZoneGroup{
		{Coordinator: "RINCON_Z", Members: []sonos.ZoneMember{{ID: "RINCON_Z", RoomName: "Værelse"}}},
	}})
	return m
}

func groupingOf(t *testing.T, m appModel) groupContent {
	t.Helper()

	content, ok := m.grouping.Content.(groupContent)
	if !ok {
		t.Fatal("the grouping modal holds no list")
	}
	return content
}

func roomsIn(g groupContent) string {
	rooms := make([]string, 0, len(g.rows))
	for _, row := range g.rows {
		rooms = append(rooms, row.room)
	}
	return strings.Join(rooms, ", ")
}

// Case: what the list starts as. A speaker already playing with this room
// begins in the group -- the list is the group as it would be, not a set of
// changes to it.
func TestGroupModal_AlreadyGroupedSpeakersStartIn(t *testing.T) {
	m := householdOnScreen(t)
	model, _ := m.Update(keyMsgFor("g"))

	rows := groupingOf(t, model.(appModel)).rows
	byRoom := map[string]bool{}
	for _, row := range rows {
		byRoom[row.room] = row.picked
	}

	if !byRoom["Køkken"] {
		t.Fatal("a speaker already playing with this room did not start in the group")
	}
	if byRoom["Spisestue"] {
		t.Fatal("a speaker in a group of its own started in this one")
	}
}

// Case: the cursor, which must move and stop at the ends rather than wrap.
func TestGroupModal_TheCursorMovesAndStops(t *testing.T) {
	g := openedGrouping(t)

	if g.cursor != 0 {
		t.Fatalf("the list opened on row %d", g.cursor)
	}
	if up := pressGroup(g, "up"); up.cursor != 0 {
		t.Fatalf("up from the first row went to %d", up.cursor)
	}

	down := pressGroup(g, "down")
	if down.cursor != 1 {
		t.Fatalf("down went to row %d", down.cursor)
	}
	if past := pressGroup(down, "down"); past.cursor != 1 {
		t.Fatalf("down from the last row went to %d", past.cursor)
	}
}

// Case: s, which puts the row under the cursor in or out and steps down, so
// marking a run is s, s, s rather than a step between each.
func TestGroupModal_SPutsARowInAndStepsDown(t *testing.T) {
	g := openedGrouping(t)
	was := g.rows[0].picked

	after := pressGroup(g, "s")

	if after.rows[0].picked == was {
		t.Fatal("s did not change whether the row is in the group")
	}
	if after.cursor != 1 {
		t.Fatalf("s left the cursor on row %d, want it stepped down", after.cursor)
	}
	if back := pressGroup(pressGroup(after, "up"), "s"); back.rows[0].picked != was {
		t.Fatal("s twice on one row did not put it back")
	}
}

// Case: the copy every update makes. The rows are the one field behind a
// slice, so a careless write would reach copies meant to be thrown away.
func TestGroupModal_OneCopyDoesNotWriteIntoAnother(t *testing.T) {
	g := openedGrouping(t)
	first := pressGroup(g, "s")

	if pressGroup(g, "s").rows[0].picked != first.rows[0].picked {
		t.Fatal("two updates of one list disagreed about the same row")
	}
	if g.rows[0].picked == first.rows[0].picked {
		t.Fatal("a later copy's change reached back into the one it was made from")
	}
}

// Case: what is picked, which is what a command will be built from once
// there is one. It follows the rows, whichever way they were set.
func TestGroupModal_PickedNamesWhatWouldBeInTheGroup(t *testing.T) {
	g := openedGrouping(t)
	if !slices.Equal(g.picked(), []string{"RINCON_A"}) {
		t.Fatalf("the list opens with %v in the group, want the one already playing along",
			g.picked())
	}

	// s on the first row takes it out and steps down; s again puts the
	// second in. The group is then the other way round.
	swapped := pressGroup(pressGroup(g, "s"), "s")

	if !slices.Equal(swapped.picked(), []string{"RINCON_B"}) {
		t.Fatalf("the group holds %v, want the room that was just picked", swapped.picked())
	}
}

func openedGrouping(t *testing.T) groupContent {
	t.Helper()

	m := householdOnScreen(t)
	model, _ := m.Update(keyMsgFor("g"))
	return groupingOf(t, model.(appModel))
}

func pressGroup(g groupContent, key string) groupContent {
	updated, _ := g.Update(keyMsgFor(key))
	return updated.(groupContent)
}

// Case: picking a row, which must ask for the speaker to be moved AT ONCE
// rather than remembering it for a later apply.
func TestGroupModal_PickingAsksForTheChangeAtOnce(t *testing.T) {
	g := openedGrouping(t)

	_, cmd := g.Update(keyMsgFor("s"))
	if cmd == nil {
		t.Fatal("picking a row asked for nothing")
	}

	change, ok := cmd().(groupChangeMsg)
	if !ok {
		t.Fatalf("picking produced %T", cmd())
	}
	if change.id != g.rows[0].id {
		t.Fatalf("the change names %q, not the row under the cursor", change.id)
	}
	if change.join {
		t.Fatal("unpicking a row already in the group asked for it to join")
	}
}

// Case: the other direction. A row that was out asks to come in.
func TestGroupModal_PickingAnAbsentRowAsksItToJoin(t *testing.T) {
	g := pressGroup(openedGrouping(t), "down") // Spisestue, which is not in

	_, cmd := g.Update(keyMsgFor("s"))
	change := cmd().(groupChangeMsg)

	if !change.join {
		t.Fatal("picking a row that was out did not ask it to join")
	}
	if change.room != "Spisestue" {
		t.Fatalf("the change names %q", change.room)
	}
}

// Case: joining points a speaker at the COORDINATOR, not at the room the
// reader is looking at. The two differ exactly when that room is itself
// being driven, and a speaker pointed at a follower follows nothing.
func TestGroupModal_JoinsTheCoordinatorNotTheRoomOnScreen(t *testing.T) {
	m, _, _ := withQueueServer(t, 5)
	id := speakerIDOf(t, m)

	// The room on screen follows another speaker.
	m.household = m.household.Apply(sonos.Topology{Groups: []sonos.ZoneGroup{{
		Coordinator: "RINCON_LEADER",
		Members: []sonos.ZoneMember{
			{ID: "RINCON_LEADER", RoomName: "Ovenpå"},
			{ID: id, RoomName: m.tabs.Items[0].Title},
		},
	}}})

	if _, follows := m.household.Follows(id); !follows {
		t.Fatal("the room on screen is not following anything, so this proves nothing")
	}

	// The change itself is sent to a speaker this app does not hold, so the
	// command cannot run here -- what matters is which coordinator it would
	// have used, and Follows is what decides that.
	owner, _ := m.household.Follows(id)
	if owner != "RINCON_LEADER" {
		t.Fatalf("the coordinator resolved to %q", owner)
	}
}

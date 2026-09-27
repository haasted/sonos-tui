package main

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: a speaker another one is playing through, which must read as
// belonging to it rather than standing on its own.
func TestGrouping_AFollowerIsMarkedInTheTabs(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	id := speakerIDOf(t, m)

	m.household = m.household.Apply(groupOf("RINCON_COORDINATOR", id))
	m = m.syncPlaying()

	if !m.tabs.Items[0].Following {
		t.Fatal("a speaker being played through is not marked as following")
	}
}

// Case: the speaker doing the driving. It is in a group, but it is the one
// in charge, so nothing about it is dimmed.
func TestGrouping_ACoordinatorIsNotMarked(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	id := speakerIDOf(t, m)

	m.household = m.household.Apply(groupOf(id, "RINCON_FOLLOWER"))
	m = m.syncPlaying()

	if m.tabs.Items[0].Following {
		t.Fatal("the coordinator of a group was marked as following it")
	}
}

// Case: a speaker on its own. A group of one in the protocol, and not a
// group at all to the reader.
func TestGrouping_AStandaloneSpeakerIsNotMarked(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	id := speakerIDOf(t, m)

	m.household = m.household.Apply(groupOf(id))
	m = m.syncPlaying()

	if m.tabs.Items[0].Following {
		t.Fatal("a speaker alone was marked as following something")
	}
}

// Case: a speaker the topology never mentions. Discovery crosses Sonos
// systems and a topology does not, so this one may answer to another
// household -- unknown, which is not the same as standing alone, and must
// not be drawn as though we had been told anything.
func TestGrouping_ASpeakerFromAnotherHouseholdIsNotMarked(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)

	m.household = m.household.Apply(groupOf("RINCON_SOMEONE", "RINCON_ELSE"))
	m = m.syncPlaying()

	if m.tabs.Items[0].Following {
		t.Fatal("a speaker this household never mentioned was marked as following")
	}
}

// Case: before anything has been read. Nothing is known, so nothing may be
// claimed -- every speaker would otherwise read as standing alone.
func TestGrouping_NothingIsMarkedBeforeTheTopologyIsRead(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)

	m = m.syncPlaying()

	if m.household.Known() {
		t.Fatal("the grouping counts as known before anything asked for it")
	}
	if m.tabs.Items[0].Following {
		t.Fatal("a speaker was marked as following before anything was read")
	}
}

// groupOf is a household of one group: the first id coordinates, the rest
// follow it.
func groupOf(coordinator string, followers ...string) sonos.Topology {
	members := []sonos.ZoneMember{{ID: coordinator, RoomName: "Coordinator"}}
	for _, id := range followers {
		members = append(members, sonos.ZoneMember{ID: id, RoomName: "Follower"})
	}
	return sonos.Topology{Groups: []sonos.ZoneGroup{{
		Coordinator: coordinator, ID: coordinator + ":1", Members: members,
	}}}
}

func speakerIDOf(t *testing.T, m appModel) string {
	t.Helper()

	speaker, ok := m.activeSpeaker()
	if !ok {
		t.Fatal("the model holds no speaker")
	}
	return speaker.device.Speaker.ID()
}

// Case: the coordinator's own pane, which names the rooms it is driving so
// the dimming in the tab bar has somewhere to be explained.
func TestGrouping_TheCoordinatorNamesWhatItDrives(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	id := speakerIDOf(t, m)

	m.household = m.household.Apply(sonos.Topology{Groups: []sonos.ZoneGroup{{
		Coordinator: id,
		Members: []sonos.ZoneMember{
			{ID: id, RoomName: "Ovenpå"},
			{ID: "RINCON_A", RoomName: "Køkken"},
			{ID: "RINCON_B", RoomName: "Spisestue"},
		},
	}}})
	m = m.syncPlaying()

	notes := m.tabs.Items[0].Notes
	if !slices.Equal(notes, []string{"Køkken", "Spisestue"}) {
		t.Fatalf("the tab carries %v, want the rooms it drives", notes)
	}

	// And they reach the screen, on their own rows under the tab row rather
	// than beside the tabs.
	lines := strings.Split(plain(m.View().Content), "\n")
	beneath := strings.Join(lines[len(lines)-2:], "\n")
	for _, room := range []string{"Køkken", "Spisestue"} {
		if !strings.Contains(beneath, room) {
			t.Fatalf("the rows under the tabs do not name %q:\n%s", room, beneath)
		}
	}
	if strings.Contains(beneath, "Ovenpå") {
		t.Fatalf("the driving room was repeated beneath itself:\n%s", beneath)
	}
}

// Case: a speaker driving nobody. There is nothing to say, and a line
// saying so would sit under every pane in the house.
func TestGrouping_AStandaloneSpeakerNamesNothing(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	id := speakerIDOf(t, m)

	m.household = m.household.Apply(groupOf(id))
	m = m.syncPlaying()

	if notes := m.tabs.Items[0].Notes; len(notes) != 0 {
		t.Fatalf("a speaker alone names %v under its tab", notes)
	}
}

// Case: the follower's own pane, which says nothing about the group for now
// -- mirroring what it is playing belongs to the task that routes commands
// through a coordinator.
func TestGrouping_AFollowerNamesNothing(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	id := speakerIDOf(t, m)

	m.household = m.household.Apply(groupOf("RINCON_COORDINATOR", id))
	m = m.syncPlaying()

	if notes := m.tabs.Items[0].Notes; len(notes) != 0 {
		t.Fatalf("a follower claims to be driving %v", notes)
	}
}

// Case: the tab row itself, which must not change. A speaker being driven
// keeps its place, its selectability and its number: only the rows beneath
// the row are new.
func TestGrouping_TheTabRowKeepsEverySpeaker(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	id := speakerIDOf(t, m)
	before := len(m.tabs.Items)

	m.household = m.household.Apply(groupOf("RINCON_COORDINATOR", id))
	m = m.syncPlaying()

	if got := len(m.tabs.Items); got != before {
		t.Fatalf("the row holds %d tabs after grouping, want the same %d", got, before)
	}
	if _, ok := m.activeSpeaker(); !ok {
		t.Fatal("the grouped speaker stopped being selectable")
	}
}

// Case: where a name under a tab sits. It must start in the same column as
// the name above it, whichever tab that is -- the status glyph is three
// bytes wide and one column, so this is measured in columns.
func TestGrouping_ANameSitsUnderTheNameItBelongsTo(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	model, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	m = model.(appModel)

	for at := range m.tabs.Items {
		speaker, ok := m.tabs.Items[at].Content.(speakerModel)
		if !ok {
			continue
		}

		id := speaker.device.Speaker.ID()
		m.household = m.household.Apply(sonos.Topology{Groups: []sonos.ZoneGroup{{
			Coordinator: id,
			Members: []sonos.ZoneMember{
				{ID: id, RoomName: m.tabs.Items[at].Title},
				{ID: "RINCON_A", RoomName: "Køkken"},
			},
		}}})
		m = m.syncPlaying()

		lines := strings.Split(plain(m.View().Content), "\n")
		row, note := lines[len(lines)-2], lines[len(lines)-1]

		want := columnOf(row, m.tabs.Items[at].Title)
		if got := columnOf(note, "Køkken"); got != want {
			t.Fatalf("tab %d: the name under it starts at column %d, want %d to match the name above\n%s\n%s",
				at, got, want, row, note)
		}
	}
}

// columnOf is where text starts on a line, counted in columns rather than
// bytes: the status glyph is one column and three bytes.
func columnOf(line, text string) int {
	at := strings.Index(line, text)
	if at < 0 {
		return -1
	}
	return len([]rune(line[:at]))
}

// Case: a household is asked once, not once per speaker in it. The first
// answer accounts for every member, so nobody else in it has anything to add.
func TestGrouping_AHouseholdIsAskedOnce(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)

	// Every speaker on screen belongs to the answer that just arrived.
	var members []sonos.ZoneMember
	for _, tab := range m.tabs.Items {
		speaker, ok := tab.Content.(speakerModel)
		if !ok {
			continue
		}
		members = append(members, sonos.ZoneMember{ID: speaker.device.Speaker.ID()})
	}
	m.household = m.household.Apply(sonos.Topology{Groups: []sonos.ZoneGroup{{
		Coordinator: members[0].ID, Members: members,
	}}})

	if _, ask := m.readUncovered(); ask != nil {
		t.Fatal("a speaker already accounted for was asked again")
	}
}

// Case: a speaker no answer has mentioned, which is what a second Sonos
// system on the same network looks like. It gets asked, because nothing so
// far can speak for it.
func TestGrouping_ASpeakerNoAnswerMentionedIsAsked(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)

	m.household = m.household.Apply(sonos.Topology{Groups: []sonos.ZoneGroup{{
		Coordinator: "RINCON_ELSEWHERE",
		Members:     []sonos.ZoneMember{{ID: "RINCON_ELSEWHERE"}},
	}}})

	if _, ask := m.readUncovered(); ask == nil {
		t.Fatal("a speaker from another household was never asked about itself")
	}
}

// Case: speakers arriving one after another before any answer comes back.
// Only one is asked: the first answer accounts for every other speaker in
// its household, so asking each as it appears sends a burst to learn one
// thing.
func TestGrouping_OnlyOneIsAskedAtATime(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)

	m, first := m.readUncovered()
	if first == nil {
		t.Fatal("nothing was asked to begin with")
	}

	if _, again := m.readUncovered(); again != nil {
		t.Fatal("a second read went out before the first had answered")
	}
}

// Case: an answer that leaves a speaker out, which is what another Sonos
// system on the network looks like. The next one is asked, and the one that
// already answered is not asked again.
func TestGrouping_TheNextIsAskedOnceTheFirstAnswers(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	m, _ = m.readUncovered()

	// An answer mentioning nobody on screen: everyone is still uncovered,
	// and handling it is what asks the next one.
	model, ask := m.Update(topologyLoadedMsg{topology: sonos.Topology{}})
	m = model.(appModel)

	if ask == nil {
		t.Fatal("nobody was asked after an answer that left everyone out")
	}
	if len(m.askedTopology) != 2 {
		t.Fatalf("%d speakers have been asked, want the first and one more",
			len(m.askedTopology))
	}
	if _, again := m.readUncovered(); again != nil {
		t.Fatal("a third read went out before the second had answered")
	}
}

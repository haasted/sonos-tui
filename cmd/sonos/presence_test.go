package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: a cached speaker whose check failed while the searches still run.
// They may yet find it, so its tab must stay.
func TestPresence_AFailedCheckAloneRemovesNothing(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")

	m = deliver(m, cachedCheckedMsg{id: "B", err: errors.New("no answer")})

	assert.True(t, m.hasSpeaker("B"), "the tab went before the searches ended")
}

// Case: a failed check, then the searches end without finding the speaker.
// Only now is it gone, and the reader is told.
func TestPresence_AFailedCheckAndNoSightingRemovesTheTab(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")
	m = deliver(m, cachedCheckedMsg{id: "B", err: errors.New("no answer")})

	model, cmd := m.Update(discoveryDoneMsg{})
	m = model.(appModel)

	assert.False(t, m.hasSpeaker("B"))
	assert.Len(t, m.tabs.Items, 2)
	assert.NotNil(t, cmd, "the removal must be announced")
}

// Case: the searches end first and the check fails after. There is nothing
// left to wait for, so the tab goes at once.
func TestPresence_ACheckFailingAfterTheSearchesRemovesTheTab(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")
	m = deliver(m, discoveryDoneMsg{})

	m = deliver(m, cachedCheckedMsg{id: "B", err: errors.New("no answer")})

	assert.False(t, m.hasSpeaker("B"))
}

// Case: a failed check, then a search finds the speaker at its address. The
// sighting proves it is there: it loads, and the end of the search keeps it.
func TestPresence_ASightingOverridesAFailedCheck(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")
	m = deliver(m, cachedCheckedMsg{id: "B", err: errors.New("no answer")})

	model, cmd := m.Update(speakerFoundMsg{speaker: cachedSpeakers()[1]})
	m = deliver(model.(appModel), discoveryDoneMsg{})

	assert.NotNil(t, cmd, "a speaker a search found must load")
	assert.True(t, m.hasSpeaker("B"), "a speaker a search found was removed")
}

// Case: a search finds a cached speaker before its check answers. It loads at
// once, and the check's later answer must not load it a second time, which
// would subscribe to its events twice.
func TestPresence_TheFirstAnswerLoadsAndTheSecondDoesNot(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")

	model, found := m.Update(speakerFoundMsg{speaker: cachedSpeakers()[0]})
	_, checked := model.(appModel).Update(cachedCheckedMsg{id: "A"})

	assert.NotNil(t, found, "the first answer must load the speaker")
	assert.Nil(t, checked, "the second answer loaded the speaker again")
}

// Case: a check that answers first. It loads, and a search answering later
// must not load it again.
func TestPresence_ACheckThatAnswersFirstLoadsOnce(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")

	model, checked := m.Update(cachedCheckedMsg{id: "A"})
	_, found := model.(appModel).Update(speakerFoundMsg{speaker: cachedSpeakers()[0]})

	assert.NotNil(t, checked)
	assert.Nil(t, found)
}

// Case: a cached speaker found at another address. Its tab must stay one tab,
// now driven by a Device at the new address.
func TestPresence_ASpeakerAtANewAddressKeepsItsTab(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")
	moved := sonos.NewSpeaker("192.168.1.42", "http://192.168.1.42:1400/xml/d.xml", "uuid:B",
		sonos.DeviceDescription{RoomName: "Bedroom"})

	model, cmd := m.Update(speakerFoundMsg{speaker: moved})
	require.NotNil(t, cmd, "the moved speaker must be loaded from its new address")
	m = deliver(model.(appModel), speakerReadyMsg{
		title: "Bedroom", id: "B", speaker: newSpeakerModel(context.Background(), sonos.NewDevice(moved)),
	})

	assert.Len(t, m.tabs.Items, 3, "the moved speaker got a second tab")
	device, ok := m.deviceFor("B")
	require.True(t, ok)
	assert.Equal(t, "192.168.1.42", device.IP)
}

// Case: a new speaker whose id sorts between two cached ones. It must go in
// by id, and the selection must stay on the speaker it was on.
func TestPresence_ANewSpeakerGoesInByIDAndTheSelectionStays(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "uuid:C")
	newcomer := sonos.NewSpeaker("192.168.1.5", "http://192.168.1.5:1400/xml/d.xml", "uuid:AB",
		sonos.DeviceDescription{RoomName: "Attic"})

	m = deliver(m, speakerReadyMsg{
		title: "Attic", id: "AB", speaker: newSpeakerModel(context.Background(), sonos.NewDevice(newcomer)),
	})

	var titles []string
	for _, tab := range m.tabs.Items {
		titles = append(titles, tab.Title)
	}
	assert.Equal(t, []string{"Kitchen", "Attic", "Bedroom", "Office"}, titles)
	assert.Equal(t, "Office", m.tabs.Items[m.tabs.Active].Title, "the selection moved off its speaker")
}

// removeCached removes a cached speaker the way the program does: its check
// fails and the searches end without it.
func removeCached(t *testing.T, m appModel, id string) appModel {
	t.Helper()
	m = deliver(m, cachedCheckedMsg{id: id, err: errors.New("no answer")})
	m = deliver(m, discoveryDoneMsg{})
	if m.hasSpeaker(id) {
		t.Fatalf("%s was not removed", id)
	}
	return m
}

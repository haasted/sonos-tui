package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: the next track starts while the queue is open, because the last one
// ended or the phone skipped. The speaker says so in an AVTransport event and
// the queue itself does not change, so nothing reads it again. The marker
// must still move to the new track, and the cursor must stay where it is.
func TestQueuePlaying_TheMarkerFollowsTheNextTrack(t *testing.T) {
	m, device, _ := withQueueServer(t, 20)
	playQueueTrack(t, device, 3)
	m = pressKey(m, "enter")
	require.Equal(t, 3, queueOf(t, m).playing, "the queue did not open on the track playing")
	m = pressKey(pressKey(m, "down"), "down")
	cursor := queueOf(t, m).cursor

	playQueueTrack(t, device, 4)
	m = deliver(m, sonos.Event{Kind: sonos.EventAVTransport, Speaker: device.Speaker})

	assert.Equal(t, 4, queueOf(t, m).playing, "the marker stayed on the track that finished")
	assert.Equal(t, cursor, queueOf(t, m).cursor, "the cursor moved with the marker")
}

// Case: the same on a speaker another one drives. Its queue is the
// coordinator's, and the event comes from the coordinator, so the open queue
// must follow the coordinator's events.
func TestQueuePlaying_AFollowerFollowsItsCoordinator(t *testing.T) {
	m, follower, leader := groupedQueue(t)
	playQueueTrack(t, leader, 3)
	m = pressKey(m, "enter")
	speaker, _ := m.activeSpeaker()
	require.Equal(t, follower, speaker.device, "the follower is not the tab on screen")
	m = deliver(m, queueLoadedMsg{device: leader, room: follower.RoomName(), items: queueItems(20), playing: 3})

	playQueueTrack(t, leader, 4)
	m = deliver(m, sonos.Event{Kind: sonos.EventAVTransport, Speaker: leader.Speaker})

	assert.Equal(t, 4, queueOf(t, m).playing, "the follower's queue kept marking the old track")
}

// Case: an event from a speaker in another room, while this room's queue is
// open. It says nothing about what plays here, so the marker stays.
func TestQueuePlaying_AnotherRoomsEventLeavesTheMarker(t *testing.T) {
	m, follower, leader := groupedQueue(t)
	m.household = sonos.Household{} // the two play on their own
	m = m.syncPlaying()
	playQueueTrack(t, follower, 3)
	m = pressKey(m, "enter")

	playQueueTrack(t, leader, 7)
	m = deliver(m, sonos.Event{Kind: sonos.EventAVTransport, Speaker: leader.Speaker})

	assert.Equal(t, 3, queueOf(t, m).playing)
}

// groupedQueue is a model with two speakers, the one on screen driven by the
// other. Both answer Browse, so either can be asked for its queue.
func groupedQueue(t *testing.T) (appModel, *sonos.Device, *sonos.Device) {
	t.Helper()
	m, follower, _ := withQueueServer(t, 20)
	follower.Speaker.USN = "uuid:RINCON_FOLLOWER::urn:schemas-upnp-org:device:ZonePlayer:1"
	m.tabs.Items[0].Content = newSpeakerModel(context.Background(), follower)

	leader, _ := queueDevice(t, 20)
	leader.Speaker.USN = "uuid:RINCON_LEADER::urn:schemas-upnp-org:device:ZonePlayer:1"
	leader.Speaker.IP = "192.168.1.11"
	m = m.insertSpeaker(speakerKey(leader.Speaker), "Leader", newSpeakerModel(context.Background(), leader))
	for i, tab := range m.tabs.Items {
		if speaker, ok := tab.Content.(speakerModel); ok && speaker.device == follower {
			m.tabs.Active = i
		}
	}

	m.household = m.household.Apply(groupOf("RINCON_LEADER", "RINCON_FOLLOWER"))
	return m.syncPlaying(), follower, leader
}

// playQueueTrack has a speaker report that it plays track n of its queue, as
// an AVTransport event does when a track ends or another is chosen.
func playQueueTrack(t *testing.T, device *sonos.Device, n int) {
	t.Helper()
	body := fmt.Sprintf(`<?xml version="1.0"?><e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0">`+
		`<e:property><LastChange>`+
		`&lt;Event xmlns="urn:schemas-upnp-org:metadata-1-0/AVT/"&gt;&lt;InstanceID val="0"&gt;`+
		`&lt;TransportState val="PLAYING"/&gt;&lt;RelativeTimePosition val="0:00:01"/&gt;`+
		`&lt;CurrentTrack val="%d"/&gt;&lt;AVTransportURI val="x-rincon-queue:RINCON_TEST#0"/&gt;`+
		`&lt;/InstanceID&gt;&lt;/Event&gt;</LastChange></e:property></e:propertyset>`, n)
	require.NoError(t, device.HandleAVTransportEvent(context.Background(), []byte(body)))
	<-device.Events()
}

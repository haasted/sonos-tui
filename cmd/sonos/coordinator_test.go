package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: a transport command pressed on a speaker another one drives. The
// follower has no transport of its own, so the command belongs to the
// coordinator and must arrive there instead.
func TestRouting_ATransportCommandGoesToTheCoordinator(t *testing.T) {
	follower, heardByFollower := recordingDevice(t)
	leader, heardByLeader := recordingDevice(t)

	m := speakerModel{ctx: context.Background(), device: follower, leader: leader}
	m.deviceCommand(actionPlay, (*sonos.Device).Play)()

	if got := heardByFollower(); len(got) != 0 {
		t.Fatalf("the follower was sent %v, and owns none of it", got)
	}
	if got := heardByLeader(); len(got) != 1 || !strings.HasSuffix(got[0], "#Play") {
		t.Fatalf("the coordinator heard %v, want one Play", got)
	}
}

// Case: volume on a driven speaker. Every room is as loud as its own
// listener wants, so this one command must NOT follow the group.
func TestRouting_VolumeStaysOnTheSpeakerInView(t *testing.T) {
	follower, heardByFollower := recordingDevice(t)
	leader, heardByLeader := recordingDevice(t)

	m := speakerModel{ctx: context.Background(), device: follower, leader: leader}
	m.adjustVolume(volumeStepUp)()

	if got := heardByLeader(); len(got) != 0 {
		t.Fatalf("the coordinator was sent %v, and nobody asked it to be louder", got)
	}
	if got := heardByFollower(); len(got) == 0 {
		t.Fatal("the speaker in view was never told to change its volume")
	}
}

// Case: mute on a driven speaker. Unlike volume, a mute is an instruction
// about the music, so it reaches the whole group through the coordinator.
func TestRouting_MuteReachesTheWholeGroup(t *testing.T) {
	follower, heardByFollower := recordingDevice(t)
	leader, heardByLeader := recordingDevice(t)

	m := speakerModel{ctx: context.Background(), device: follower, leader: leader}
	m.toggleMute()()

	if got := heardByFollower(); len(got) != 0 {
		t.Fatalf("the follower was muted alone, hearing %v", got)
	}
	got := heardByLeader()
	if len(got) != 2 || !strings.HasSuffix(got[0], "#GetGroupMute") || !strings.HasSuffix(got[1], "#SetGroupMute") {
		t.Fatalf("the coordinator heard %v, want GetGroupMute then SetGroupMute", got)
	}
}

// Case: a speaker that drives itself. Nothing may be rerouted, which is the
// state every speaker is in until it joins a group.
func TestRouting_AnUngroupedSpeakerKeepsItsOwnCommands(t *testing.T) {
	device, heard := recordingDevice(t)

	m := speakerModel{ctx: context.Background(), device: device}
	m.deviceCommand(actionNext, (*sonos.Device).Next)()

	if got := heard(); len(got) != 1 || !strings.HasSuffix(got[0], "#Next") {
		t.Fatalf("the speaker heard %v, want one Next", got)
	}
}

// Case: a coordinator the program does not hold, which is what a group
// spanning another Sonos system looks like. The command still has to go
// somewhere, and the speaker in view is where it went before any of this.
func TestRouting_AnUnknownCoordinatorFallsBackToTheSpeakerInView(t *testing.T) {
	m, _, _ := withQueueServer(t, 5)
	id := speakerIDOf(t, m)

	m.household = m.household.Apply(groupOf("RINCON_NOT_ON_SCREEN", id))
	m = m.syncPlaying()

	speaker, ok := m.tabs.Items[0].Content.(speakerModel)
	if !ok {
		t.Fatal("the tab holds no speaker")
	}
	if speaker.leader != nil {
		t.Fatal("a coordinator we do not hold was routed to anyway")
	}
	if speaker.transport() != speaker.device {
		t.Fatal("the fallback is not the speaker in view")
	}
}

// Case: what a driven speaker's pane draws. The room really is playing the
// coordinator's track, so saying otherwise contradicts what is being heard --
// but the volume on it is still its own.
func TestRouting_AFollowerShowsTheGroupsTrackAndItsOwnVolume(t *testing.T) {
	follower, _ := recordingDevice(t)
	leader, _ := recordingDevice(t)

	playTrack(t, leader, "Autobahn")
	setVolume(t, follower, 15)

	m := speakerModel{ctx: context.Background(), device: follower, leader: leader}
	shown := m.shown()

	if shown.Current.Title != "Autobahn" {
		t.Fatalf("the pane shows %q, want the track the room is actually playing", shown.Current.Title)
	}
	if !shown.Playing() {
		t.Fatal("the pane reads as stopped while the group plays through it")
	}
	if shown.Volume != 15 {
		t.Fatalf("the pane shows volume %d, want the speaker's own 15", shown.Volume)
	}
}

// recordingDevice is a speaker that accepts every command and reports which
// SOAP actions reached it, so a test can say where a command went.
func recordingDevice(t *testing.T) (*sonos.Device, func() []string) {
	t.Helper()

	var mu sync.Mutex
	var actions []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		actions = append(actions, strings.Trim(r.Header.Get("SOAPACTION"), `"`))
		mu.Unlock()

		action := lastSegment(r.Header.Get("SOAPACTION"))
		fmt.Fprintf(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
			`<s:Body><u:%sResponse xmlns:u="urn:x"><CurrentMute>0</CurrentMute>`+
			`<CurrentVolume>15</CurrentVolume></u:%sResponse></s:Body></s:Envelope>`, action, action)
	}))
	t.Cleanup(server.Close)

	device := sonos.NewDeviceWithClient(
		sonos.Speaker{IP: server.Listener.Addr().String()},
		&sonos.SonosClient{BaseURL: server.URL, HTTPClient: server.Client()},
	)
	return device, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), actions...)
	}
}

func lastSegment(soapAction string) string {
	_, action, _ := strings.Cut(strings.Trim(soapAction, `"`), "#")
	return action
}

// playTrack puts a speaker into PLAYING with a named track, the way an
// arriving GENA notify would.
func playTrack(t *testing.T, device *sonos.Device, title string) {
	t.Helper()

	if err := device.HandleAVTransportEvent(context.Background(), playingBody(title)); err != nil {
		t.Fatalf("handling event: %v", err)
	}
	<-device.Events()
}

// setVolume puts a level on a speaker without going near the network, so a
// test can tell one speaker's volume from another's.
func setVolume(t *testing.T, device *sonos.Device, level int) {
	t.Helper()

	if err := device.HandleRenderingControlEvent(volumeBody(level)); err != nil {
		t.Fatalf("handling volume event: %v", err)
	}
	<-device.Events()
}

func volumeBody(level int) []byte {
	return []byte(`<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0"><e:property><LastChange>` +
		`&lt;Event xmlns="urn:schemas-upnp-org:metadata-1-0/RCS/"&gt;&lt;InstanceID val="0"&gt;` +
		fmt.Sprintf(`&lt;Volume channel="Master" val="%d"/&gt;`, level) +
		`&lt;/InstanceID&gt;&lt;/Event&gt;</LastChange></e:property></e:propertyset>`)
}

// Case: son-bqf.6, the glyph in the tab row. A speaker being driven by a
// paused group is silent, and the row said otherwise because it read that
// speaker's own transport. Which of two causes made its own state wrong no
// longer matters: nothing asks it any more.
func TestRouting_ADrivenSpeakerIsNotPlayingWhenTheGroupIsPaused(t *testing.T) {
	m, kitchen := modelPlaying(t, "Kitchen Song")
	bedroom, ok := m.deviceFor("B")
	if !ok {
		t.Fatal("no device for the second cached speaker")
	}

	// The bedroom is driven by the kitchen, and still believes it is playing
	// -- which is exactly the stale state the bug reported.
	playTrack(t, bedroom, "Kitchen Song")
	m.household = m.household.Apply(sonos.Topology{Groups: []sonos.ZoneGroup{{
		Coordinator: kitchen.Speaker.ID(),
		Members: []sonos.ZoneMember{
			{ID: kitchen.Speaker.ID(), RoomName: "Kitchen"},
			{ID: bedroom.Speaker.ID(), RoomName: "Bedroom"},
		},
	}}})
	m = m.syncPlaying()
	if !m.tabs.Items[1].Playing {
		t.Fatal("the driven speaker does not read as playing before the group pauses")
	}

	pause(t, kitchen)
	m = m.syncPlaying()

	if m.tabs.Items[1].Playing {
		t.Fatal("the driven speaker still shows the playing glyph while the group is paused")
	}
	if m.tabs.Items[0].Playing {
		t.Fatal("the coordinator itself still reads as playing")
	}
}

// pause puts a speaker into PAUSED_PLAYBACK the way an arriving notify would.
func pause(t *testing.T, device *sonos.Device) {
	t.Helper()

	body := []byte(`<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0"><e:property><LastChange>` +
		`&lt;Event xmlns="urn:schemas-upnp-org:metadata-1-0/AVT/"&gt;&lt;InstanceID val="0"&gt;` +
		`&lt;TransportState val="PAUSED_PLAYBACK"/&gt;&lt;RelativeTimePosition val="0:00:12"/&gt;` +
		`&lt;/InstanceID&gt;&lt;/Event&gt;</LastChange></e:property></e:propertyset>`)
	if err := device.HandleAVTransportEvent(context.Background(), body); err != nil {
		t.Fatalf("handling pause event: %v", err)
	}
	<-device.Events()
}

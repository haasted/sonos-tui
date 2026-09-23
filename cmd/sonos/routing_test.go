package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/nowplaying"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: a playback command whose result comes back after the user has moved to
// another tab, which must not repaint that tab with the first speaker's track.
func TestCommandResultReachesItsOwnSpeaker(t *testing.T) {
	m, kitchen := modelPlaying(t, "Kitchen Song")
	result := speakerModelFor(t, m, kitchen).to(
		nowplaying.TrackMsg{Track: trackFromState(kitchen.State())})

	m = switchRight(t, m, "Bedroom")
	m = deliver(m, result)

	check(t, !strings.Contains(render(m, 1), "Kitchen Song"),
		"Bedroom must not show Kitchen's track")
	check(t, strings.Contains(render(m, 0), "Kitchen Song"),
		"Kitchen must still show its own track")
}

// Case: the same race for volume, where a misdirected readout would invite the
// user to adjust the wrong speaker.
func TestVolumeResultReachesItsOwnSpeaker(t *testing.T) {
	m, kitchen := modelPlaying(t, "Kitchen Song")
	result := speakerModelFor(t, m, kitchen).to(
		nowplaying.VolumeMsg{Volume: 42, Muted: false, Show: true})

	m = switchRight(t, m, "Bedroom")
	m = deliver(m, result)

	check(t, !strings.Contains(render(m, 1), "42"), "Bedroom must not show Kitchen's volume")
}

// Case: a failed command, which must name its room and surface app-wide —
// the user may well be looking at a different tab when it fails.
func TestFailedCommandRaisesANamedNotice(t *testing.T) {
	m, kitchen := modelPlaying(t, "Kitchen Song")

	msg := speakerModelFor(t, m, kitchen).failed(errors.New("kitchen exploded"))

	notice, ok := msg.(panes.NoticeMsg)
	if !ok {
		t.Fatalf("expected an app-wide notice, got %T", msg)
	}
	check(t, notice.Level == panes.NoticeError, "a failed command is an error, not a warning")
	check(t, strings.Contains(notice.Text, "Kitchen"), "the notice must name the room that failed")
	check(t, strings.Contains(notice.Text, "kitchen exploded"), "the notice must carry the cause")
}

// Case: a result for a speaker whose tab was removed mid-flight, which must be
// dropped rather than falling through to the active tab.
func TestResultForARemovedSpeakerIsDropped(t *testing.T) {
	m, kitchen := modelPlaying(t, "Kitchen Song")
	result := speakerModelFor(t, m, kitchen).to(
		nowplaying.TrackMsg{Track: trackFromState(kitchen.State())})

	model, _ := m.Update(speakerGoneMsg{ip: "192.168.1.10"})
	m = deliver(model.(appModel), result)

	for i := range m.tabs.Items {
		check(t, !strings.Contains(render(m, i), "Kitchen Song"),
			"no surviving tab may inherit the removed speaker's track")
	}
}

// modelPlaying builds the cached three-speaker model with Kitchen playing the
// named track, and returns Kitchen's device.
func modelPlaying(t *testing.T, title string) (appModel, *sonos.Device) {
	t.Helper()

	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")
	device, ok := m.deviceFor("192.168.1.10")
	if !ok {
		t.Fatal("no device for the first cached speaker")
	}
	if err := device.HandleAVTransportEvent(context.Background(), playingBody(title)); err != nil {
		t.Fatalf("handling event: %v", err)
	}
	<-device.Events()
	return m, device
}

// speakerModelFor finds the tab model driving a device, so a test can build the
// exact message that device's own commands would produce.
func speakerModelFor(t *testing.T, m appModel, device *sonos.Device) speakerModel {
	t.Helper()

	for _, tab := range m.tabs.Items {
		if speaker, ok := tab.Content.(speakerModel); ok && speaker.device == device {
			return speaker
		}
	}
	t.Fatal("no tab for the device")
	return speakerModel{}
}

func switchRight(t *testing.T, m appModel, expect string) appModel {
	t.Helper()

	model, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = model.(appModel)
	if got := m.tabs.Items[m.tabs.Active].Title; got != expect {
		t.Fatalf("expected to land on %q, got %q", expect, got)
	}
	return m
}

func deliver(m appModel, msg tea.Msg) appModel {
	model, _ := m.Update(msg)
	return model.(appModel)
}

func render(m appModel, tab int) string {
	return m.tabs.Items[tab].Content.View().Content
}

func check(t *testing.T, ok bool, msg string) {
	t.Helper()
	if !ok {
		t.Fatal(msg)
	}
}

// playingBody is a GENA notify carrying its own position, so handling it never
// reaches for the network to fill one in.
func playingBody(title string) []byte {
	return []byte(`<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0"><e:property><LastChange>` +
		`&lt;Event xmlns="urn:schemas-upnp-org:metadata-1-0/AVT/"&gt;&lt;InstanceID val="0"&gt;` +
		`&lt;TransportState val="PLAYING"/&gt;&lt;RelativeTimePosition val="0:00:12"/&gt;` +
		`&lt;CurrentTrackMetaData val="&amp;lt;DIDL-Lite&amp;gt;&amp;lt;item&amp;gt;&amp;lt;dc:title&amp;gt;` +
		title + `&amp;lt;/dc:title&amp;gt;&amp;lt;/item&amp;gt;&amp;lt;/DIDL-Lite&amp;gt;"/&gt;` +
		`&lt;/InstanceID&gt;&lt;/Event&gt;</LastChange></e:property></e:propertyset>`)
}

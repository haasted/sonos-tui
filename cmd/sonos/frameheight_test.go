package main

import (
	"context"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/nowplaying"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: every state the player passes through on the way up. The inline frame
// must be the same height in all of them — a frame that grows scrolls the
// terminal, and that scroll is unrecoverable.
func TestView_HeightIsConstantAcrossStates(t *testing.T) {
	for name, view := range playerStates(t) {
		if got := lipgloss.Height(view); got != playerHeightRows {
			t.Errorf("%s renders %d rows, want %d — a changing height scrolls the terminal",
				name, got, playerHeightRows)
		}
	}
}

// Case: the real content behind the padding, which must fit the reserved
// height. If it ever outgrows it, padToHeight stops helping and the frame
// starts scrolling again, silently.
func TestView_NeverExceedsThePlayerHeight(t *testing.T) {
	m := loadedModel(t)
	m.tabs.Width = 40 // force the ‹ › overflow windowing too

	if got := lipgloss.Height(m.tabs.View()); got > playerHeightRows {
		t.Fatalf("player content is %d rows, over the %d reserved: raise playerHeightRows",
			got, playerHeightRows)
	}
}

// Case: a notice, which lives in the divider precisely so it costs no rows.
// This is the guard against someone later making it a line of its own.
func TestView_ANoticeDoesNotChangeTheHeight(t *testing.T) {
	m := loadedModel(t)
	before := lipgloss.Height(m.View().Content)

	model, _ := m.Update(panes.NoticeMsg{Level: panes.NoticeWarn, Text: "Køkken: no live updates"})

	if got := lipgloss.Height(model.(appModel).View().Content); got != before {
		t.Fatalf("a notice changed the frame from %d rows to %d", before, got)
	}
}

func playerStates(t *testing.T) map[string]string {
	t.Helper()

	empty := newAppModel(context.Background(), nil, nil, "")
	discovered, _ := empty.Update(discoveryDoneMsg{})

	return map[string]string{
		"discovering": empty.View().Content,
		"none found":  discovered.(appModel).View().Content,
		"cached tabs": newAppModel(context.Background(), nil, cachedSpeakers(), "").View().Content,
		"playing":     loadedModel(t).View().Content,
	}
}

// loadedModel is the steady state: cached tabs with a track playing.
func loadedModel(t *testing.T) appModel {
	t.Helper()

	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")
	device, ok := m.deviceFor("192.168.1.10")
	if !ok {
		t.Fatal("no device for the first cached speaker")
	}
	if err := device.HandleAVTransportEvent(context.Background(), playingBody("A Fairly Long Track Title Indeed")); err != nil {
		t.Fatalf("handling event: %v", err)
	}
	<-device.Events()

	speaker := speakerModelFor(t, m, device)
	updated, _ := speaker.Update(nowplaying.TrackMsg{Track: trackFromState(device.State())})
	m.tabs.Items[0].Content = updated
	return m
}

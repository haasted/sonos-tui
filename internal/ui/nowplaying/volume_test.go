package nowplaying

import (
	"strings"
	"testing"
	"time"
)

// Case: a mute after a stretch of playback, which used to rewind the clock —
// a full TrackMsg carried a stale RelTime along with the volume.
func TestVolumeMsgLeavesTheClockAlone(t *testing.T) {
	m := playingModel(90 * time.Second)
	before := m.elapsed().Round(time.Second)

	m, _ = m.Update(VolumeMsg{Volume: 40, Muted: true})
	if got := m.elapsed().Round(time.Second); got != before {
		t.Fatalf("muting moved the clock: %v then %v", before, got)
	}

	m, _ = m.Update(VolumeMsg{Volume: 46})
	if got := m.elapsed().Round(time.Second); got != before {
		t.Fatalf("a volume change moved the clock: %v then %v", before, got)
	}
}

// Case: arriving at a speaker's tab, where the level has not changed but
// should still be shown.
func TestVolumeMsgShowSurfacesAnUnchangedLevel(t *testing.T) {
	m := playingModel(0)
	if readoutVisible(m) {
		t.Fatal("a freshly loaded model should show the clock, not a readout")
	}

	m, _ = m.Update(VolumeMsg{Volume: 40, Show: true})
	if !readoutVisible(m) {
		t.Fatal("Show should surface the readout even though the level is unchanged")
	}
}

// Case: the routine updates that must stay quiet, or the readout would appear
// constantly during normal playback.
func TestVolumeMsgWithoutShowIgnoresAnUnchangedLevel(t *testing.T) {
	m := playingModel(0)

	m, _ = m.Update(VolumeMsg{Volume: 40})
	if readoutVisible(m) {
		t.Fatal("an unchanged volume without Show must not surface the readout")
	}
}

// Case: an external change arriving by event, which must still be announced.
func TestVolumeMsgSurfacesAChangedLevel(t *testing.T) {
	m := playingModel(0)

	m, _ = m.Update(VolumeMsg{Volume: 55})
	if !readoutVisible(m) {
		t.Fatal("a changed volume should surface the readout")
	}
	if !m.volumeRising {
		t.Fatal("40 -> 55 should read as rising")
	}
}

// Case: mute arriving by VolumeMsg, which must still drive the glyph flash.
func TestVolumeMsgDrivesTheMuteFlash(t *testing.T) {
	m := playingModel(0)

	m, _ = m.Update(VolumeMsg{Volume: 40, Muted: true})
	if m.muteFlash() <= 0 {
		t.Fatal("muting via VolumeMsg should start the glyph flash")
	}

	m, _ = m.Update(VolumeMsg{Volume: 40})
	if m.muteFlash() != 0 {
		t.Fatal("unmuting should clear the flash")
	}
}

func playingModel(playedFor time.Duration) Model {
	m := New(Track{Title: "So What", Album: "Kind of Blue", Playing: true,
		Volume: 40, Elapsed: 60 * time.Second, Duration: 300 * time.Second})
	m.receivedAt = time.Now().Add(-playedFor)
	return m
}

// readoutVisible reports whether the readout is on screen. It checks the flash
// that gates rendering rather than looking for an arrow, which arrival omits.
func readoutVisible(m Model) bool {
	return m.volumeFlash() > 0
}

// Case: arriving at a speaker, where nothing moved and an arrow would imply a
// change that did not happen.
func TestVolumeReadoutOmitsTheArrowOnArrival(t *testing.T) {
	m := playingModel(0)

	m, _ = m.Update(VolumeMsg{Volume: 40, Show: true})
	if got := m.volumeReadout(); got != "40" {
		t.Fatalf("arrival readout was %q, want a bare level", got)
	}
}

// Case: arriving after an earlier adjustment, whose direction must not linger.
func TestVolumeReadoutDropsAStaleArrowOnArrival(t *testing.T) {
	m := playingModel(0)

	m, _ = m.Update(VolumeMsg{Volume: 46})
	if !strings.HasPrefix(m.volumeReadout(), volumeUpGlyph) {
		t.Fatalf("a raise should show the up arrow, got %q", m.volumeReadout())
	}

	m, _ = m.Update(VolumeMsg{Volume: 46, Show: true})
	if got := m.volumeReadout(); got != "46" {
		t.Fatalf("arriving after a change kept a stale arrow: %q", got)
	}
}

// Case: real changes in both directions, which must keep their arrows.
func TestVolumeReadoutKeepsArrowsOnRealChanges(t *testing.T) {
	m := playingModel(0)

	m, _ = m.Update(VolumeMsg{Volume: 46})
	if got := m.volumeReadout(); got != volumeUpGlyph+" 46" {
		t.Fatalf("got %q, want an up arrow", got)
	}

	m, _ = m.Update(VolumeMsg{Volume: 42})
	if got := m.volumeReadout(); got != volumeDownGlyph+" 42" {
		t.Fatalf("got %q, want a down arrow", got)
	}
}

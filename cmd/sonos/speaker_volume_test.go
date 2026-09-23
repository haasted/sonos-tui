package main

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: a held up-arrow, where key repeat delivers far more presses than one
// window should ever apply.
func TestSpeaker_VolumeUpIsCappedPerWindow(t *testing.T) {
	m := newTestSpeaker()
	for range 20 {
		m = press(m, "up")
	}

	if m.pendingVolume != volumeUpMaxStep {
		t.Fatalf("holding up accumulated %d; it must be capped at %d so a held key can't jump the volume",
			m.pendingVolume, volumeUpMaxStep)
	}
}

// Case: repeated down presses, which accumulate freely so down stays a fast escape.
func TestSpeaker_VolumeDownIsNotCapped(t *testing.T) {
	m := newTestSpeaker()
	for range 5 {
		m = press(m, "down")
	}

	want := 5 * volumeStepDown
	if m.pendingVolume != want {
		t.Fatalf("holding down accumulated %d, want %d — down is deliberately uncapped so it stays a fast escape",
			m.pendingVolume, want)
	}
}

// Case: several presses inside one window, which must yield a single SOAP call.
func TestSpeaker_VolumePressesCoalesceIntoOneCommit(t *testing.T) {
	m := newTestSpeaker()
	m = press(m, "down")
	m = press(m, "down")
	m = press(m, "down")

	if m.pendingVolume != 3*volumeStepDown {
		t.Fatalf("presses should accumulate, got %d", m.pendingVolume)
	}

	m, cmd := commit(m)

	if cmd == nil {
		t.Fatal("a matching commit should issue the volume command")
	}
	if m.pendingVolume != 0 {
		t.Fatalf("pending delta should reset after commit, got %d", m.pendingVolume)
	}
}

// Case: a commit tick arriving after its window closed and a new one opened.
func TestSpeaker_StaleCommitFromAnEarlierWindowIsIgnored(t *testing.T) {
	m := newTestSpeaker()
	m = press(m, "up")
	stale := m.volumeGen

	m, _ = commit(m)

	m = press(m, "up")
	if m.volumeGen == stale {
		t.Fatal("a new window should carry a new generation")
	}

	updated, cmd := m.Update(volumeCommitMsg{gen: stale})
	m = updated.(speakerModel)

	if cmd != nil {
		t.Fatal("a commit tick from a closed window must not fire — that would send the volume twice")
	}
	if m.pendingVolume == 0 {
		t.Fatal("a stale commit must not consume the delta the open window still needs")
	}
}

// Case: a key held across window boundaries. Regression — a trailing debounce
// once committed nothing until release, then applied the whole delta at once.
func TestSpeaker_HeldKeyCommitsWithoutWaitingForRelease(t *testing.T) {
	m := newTestSpeaker()

	m, first := pressCmd(m, "down")
	if first == nil {
		t.Fatal("the first press should open a commit window")
	}

	m, again := pressCmd(m, "down")
	if again != nil {
		t.Fatal("a press inside an open window must not open a second one, or a held key double-commits")
	}
	if m.pendingVolume != 2*volumeStepDown {
		t.Fatalf("presses inside the window should still accumulate, got %d", m.pendingVolume)
	}

	m, cmd := commit(m)
	if cmd == nil {
		t.Fatal("the window must commit on its own, without waiting for the key to be released")
	}

	m, reopened := pressCmd(m, "down")
	if reopened == nil {
		t.Fatal("a still-held key should open the next window, so volume keeps moving while held")
	}
	if m.pendingVolume != volumeStepDown {
		t.Fatalf("the new window should start from a cleared delta, got %d", m.pendingVolume)
	}
}

// Case: a commit tick with no accumulated delta, which must not reach the speaker.
func TestSpeaker_CommitWithNothingPendingDoesNothing(t *testing.T) {
	m := newTestSpeaker()

	_, cmd := m.Update(volumeCommitMsg{gen: m.volumeGen})
	if cmd != nil {
		t.Fatal("a commit with no accumulated delta should not issue a SOAP call")
	}
}

// Case: two speakers each with a window open, whose ticks must not cross-fire.
func TestSpeaker_GenerationsAreUniqueAcrossSpeakers(t *testing.T) {
	a := press(newTestSpeaker(), "up")
	b := press(newTestSpeaker(), "up")

	if a.volumeGen == b.volumeGen {
		t.Fatal("generations must be globally unique, or one speaker's commit tick can fire another speaker's pending volume")
	}
}

// newTestSpeaker builds a speakerModel around a device pointed at a dead address;
// these tests exercise debounce bookkeeping, never real playback.
func newTestSpeaker() speakerModel {
	return newSpeakerModel(context.Background(), sonos.NewDevice(sonos.Speaker{IP: "127.0.0.1"}))
}

// press sends one key to the model and discards the resulting command.
func press(m speakerModel, key string) speakerModel {
	updated, _ := pressCmd(m, key)
	return updated
}

// pressCmd sends one key and keeps the command, for asserting whether a
// press opened a new commit window.
func pressCmd(m speakerModel, key string) (speakerModel, tea.Cmd) {
	updated, cmd := m.Update(tea.KeyPressMsg{Code: keyCode(key), Text: key})
	return updated.(speakerModel), cmd
}

// commit closes the model's current volume window, as the debounce tick would.
func commit(m speakerModel) (speakerModel, tea.Cmd) {
	updated, cmd := m.Update(volumeCommitMsg{gen: m.volumeGen})
	return updated.(speakerModel), cmd
}

// keyCode maps a key name to the rune Bubbletea reports for it.
func keyCode(key string) rune {
	switch key {
	case "up":
		return tea.KeyUp
	case "down":
		return tea.KeyDown
	default:
		return rune(key[0])
	}
}

// Case: a single press, which must move the volume by exactly one. Nothing
// asserted the magnitude before — every other test here references the
// constants symbolically, so they would pass at any step size.
func TestSpeaker_SinglePressMovesTheVolumeByOne(t *testing.T) {
	if got := press(newTestSpeaker(), "up").pendingVolume; got != 1 {
		t.Fatalf("one up press queued %+d, want +1", got)
	}
	if got := press(newTestSpeaker(), "down").pendingVolume; got != -1 {
		t.Fatalf("one down press queued %+d, want -1", got)
	}
}

// Case: both keys held. Up is capped and down is not, so the step size decides
// whether they travel comparably: at a step of 2 a held down key moved -16 per
// window against a capped +6, which is the asymmetry that prompted this change.
func TestSpeaker_HeldKeysTravelSymmetrically(t *testing.T) {
	up, down := newTestSpeaker(), newTestSpeaker()
	for range 8 {
		up = press(up, "up")
		down = press(down, "down")
	}

	if up.pendingVolume != volumeUpMaxStep {
		t.Fatalf("held up queued %+d, want the cap of %+d", up.pendingVolume, volumeUpMaxStep)
	}
	if gap := -down.pendingVolume - up.pendingVolume; gap > 2 {
		t.Fatalf("held down ran away from up: %+d against %+d", down.pendingVolume, up.pendingVolume)
	}
}

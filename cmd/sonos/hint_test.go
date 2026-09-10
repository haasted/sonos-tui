package main

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: the whole point — nobody has pressed anything, so the hint appears.
func TestHint_ShowsWhenNoKeyHasBeenPressed(t *testing.T) {
	msg, ok := hintNotice(newHintModel())
	if !ok {
		t.Fatal("an untouched program should offer the hint")
	}
	if msg.Text != hintText {
		t.Fatalf("hinted %q, want %q", msg.Text, hintText)
	}
	if msg.Entrance != panes.NoticeBloom {
		t.Fatalf("hint arrived as entrance %v, want bloom — it has to be noticed", msg.Entrance)
	}
	if msg.Level != panes.NoticeInfo {
		t.Fatalf("hint arrived at level %v, want info — it is not a problem", msg.Level)
	}
}

// Case: a user who already knows the keys. Offering help to someone who has
// been driving the program for four seconds is noise.
func TestHint_AKeypressRetiresIt(t *testing.T) {
	m := pressApp(newHintModel(), "right")

	if _, ok := hintNotice(m); ok {
		t.Fatal("the hint fired after the user had already pressed a key")
	}
}

// Case: opening help by hand, which is the one keypress that would make the
// hint absurd — it points at a modal already on screen.
func TestHint_OpeningHelpRetiresIt(t *testing.T) {
	m := pressApp(newHintModel(), "?")
	if !m.help.IsOpen() {
		t.Fatal("? should have opened the help modal")
	}

	if _, ok := hintNotice(m); ok {
		t.Fatal("the hint fired while the help it points at was already open")
	}
}

// Case: a degraded speaker warning lands first. Startup is exactly when one
// appears, and notices are latest-wins — so a hint firing at four seconds
// would bury the only message that mattered.
func TestHint_YieldsToARealNotice(t *testing.T) {
	m := newHintModel()
	updated, _ := m.Update(panes.NoticeMsg{Level: panes.NoticeWarn, Text: "Køkken did not answer"})
	m = updated.(appModel)

	if _, ok := hintNotice(m); ok {
		t.Fatal("the hint displaced a warning that was already on the rule")
	}
}

// Case: a second tick, however it arrives. The hint is an invitation to a
// first-time user, not a recurring nag.
func TestHint_ShowsAtMostOncePerRun(t *testing.T) {
	m := newHintModel()

	updated, cmd := m.Update(hintTickMsg{})
	m = updated.(appModel)
	if cmd == nil {
		t.Fatal("the first tick should raise the hint")
	}

	if _, ok := hintNotice(m); ok {
		t.Fatal("the hint fired twice in one run")
	}
}

// Case: a resize, which says nothing about whether the user has found the
// keyboard — dragging a window is not a reason to withhold help.
func TestHint_ResizingDoesNotRetireIt(t *testing.T) {
	updated, _ := newHintModel().Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	if _, ok := hintNotice(updated.(appModel)); !ok {
		t.Fatal("a resize retired the hint; only a keypress should")
	}
}

func newHintModel() appModel {
	return newAppModel(context.Background(), nil, nil, "")
}

// hintNotice runs the hint's timer and reports the notice it raised, if any.
func hintNotice(m appModel) (panes.NoticeMsg, bool) {
	_, cmd := m.Update(hintTickMsg{})
	if cmd == nil {
		return panes.NoticeMsg{}, false
	}
	msg, ok := cmd().(panes.NoticeMsg)
	return msg, ok
}

// pressApp sends one key to the app model, as the terminal would.
func pressApp(m appModel, key string) appModel {
	updated, _ := m.Update(tea.KeyPressMsg{Code: keyCode(key), Text: key})
	return updated.(appModel)
}

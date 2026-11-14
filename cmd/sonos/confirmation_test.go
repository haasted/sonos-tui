package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// Case: every playback command, which must have something to say once it has
// run. A command with no entry answers with silence, which is what this whole
// feature exists to remove.
func TestConfirmation_EveryPlaybackCommandSaysSomething(t *testing.T) {
	for _, action := range []string{actionPlay, actionPause, actionNext, actionPrevious} {
		if confirmations[action] == "" {
			t.Errorf("the command %q confirms nothing", action)
		}
	}
}

// Case: the wording. A confirmation reports what happened, where the command
// name orders it to happen.
//
// Play and pause are where the two forms differ. A skip reads the same either
// way, so only these two can prove the distinction is being made.
func TestConfirmation_ReadsAsTheOutcomeNotTheCommand(t *testing.T) {
	if got := confirmations[actionPlay]; got == actionPlay {
		t.Errorf("play confirms with %q, which is the order and not the outcome", got)
	}
	if got := confirmations[actionPause]; got == actionPause {
		t.Errorf("pause confirms with %q, which is the order and not the outcome", got)
	}
}

// Case: mute, which is the one command whose outcome is not fixed by the key.
// The same key mutes and unmutes, so the state read back decides the words.
func TestConfirmation_MuteSaysWhichWayItWent(t *testing.T) {
	if muteConfirmation(true) == muteConfirmation(false) {
		t.Fatal("mute and unmute confirm with the same words")
	}
}

// Case: a command that ran. It sends two messages: its own result for the
// speaker, and a notice for the whole program.
func TestConfirmation_ACommandSendsItsResultAndANotice(t *testing.T) {
	result := speakerMsg{}

	notice, ok := noticeIn(t, withConfirmation(result, "paused"))
	if !ok {
		t.Fatal("a command that ran sent no notice")
	}
	if notice.Text != "paused" {
		t.Fatalf("the notice says %q", notice.Text)
	}
	if !notice.Confirmation {
		t.Fatal("the notice does not yield, so it could bury a refusal")
	}
	if notice.Entrance != panes.NoticeAppear {
		t.Fatal("a confirmation must not perform: it appears on every keypress")
	}
}

// Case: a command with nothing to say, which must not send an empty notice.
// Volume is the one in the program today.
func TestConfirmation_ACommandWithNoWordsSendsOnlyItsResult(t *testing.T) {
	result := speakerMsg{}

	got := withConfirmation(result, "")
	if _, isBatch := got.(tea.BatchMsg); isBatch {
		t.Fatal("a command with no confirmation still sent two messages")
	}
}

// noticeIn runs a batched message and returns the notice it carries.
func noticeIn(t *testing.T, msg tea.Msg) (panes.NoticeMsg, bool) {
	t.Helper()

	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return panes.NoticeMsg{}, false
	}
	for _, cmd := range batch {
		if notice, ok := cmd().(panes.NoticeMsg); ok {
			return notice, true
		}
	}
	return panes.NoticeMsg{}, false
}

package main

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// Case: , on the player, signed in. The settings open on the cloud's section,
// saying when the session runs out, with "Sign out" to act on; , closes them.
func TestSettings_CommaOpensAndCloses(t *testing.T) {
	ends := time.Date(2026, 12, 1, 12, 0, 0, 0, time.Local)
	m := pressKey(searchModel(t, &fakeCloud{signedIn: true, ends: ends}), ",")

	require.True(t, m.settings.IsOpen())
	view := plain(m.View().Content)
	assert.Contains(t, view, fmt.Sprintf(settingsEndsFmt, "1 December"))
	assert.Contains(t, view, listCursorGlyph+" "+settingsSignOut)

	m = pressKey(m, ",")
	assert.False(t, m.settings.IsOpen())
}

// Case: the cloud signed out, and no cloud at all. Signed out offers to sign
// in; with no cloud there is nothing to act on.
func TestSettings_WhatTheCloudSectionSays(t *testing.T) {
	signedOut := plain(pressKey(searchModel(t, &fakeCloud{}), ",").View().Content)
	assert.Contains(t, signedOut, settingsSignedOut)
	assert.Contains(t, signedOut, listCursorGlyph+" "+settingsSignIn)

	noCloud := plain(pressKey(sizedModel(t), ",").View().Content)
	assert.Contains(t, noCloud, settingsNoCloud)
	assert.NotContains(t, noCloud, listCursorGlyph)
}

// Case: enter once on "Sign out". It asks to be confirmed and signs nothing
// out; the second enter asks for the sign-out.
func TestSettings_SigningOutTakesTwoEnters(t *testing.T) {
	s := settingsContent{cloud: cloudState{available: true, signedIn: true}}

	model, cmd := s.Update(keyMsgFor("enter"))
	s = model.(settingsContent)
	assert.Nil(t, cmd, "one enter asked for a sign-out")
	assert.Contains(t, plain(s.View().Content), settingsConfirm)

	_, cmd = s.Update(keyMsgFor("enter"))
	require.NotNil(t, cmd)
	assert.Equal(t, settingsSignOutMsg{}, cmd())
}

// Case: another key, esc among them, between the two enters. The session is
// kept, the row is "Sign out" again, and esc did not close the settings.
func TestSettings_AnyOtherKeyKeepsTheSession(t *testing.T) {
	for _, key := range []string{"esc", "down", "x"} {
		m := pressKey(pressKey(searchModel(t, &fakeCloud{signedIn: true}), ","), "enter")

		model, cmd := m.Update(keyMsgFor(key))
		m = model.(appModel)

		assert.True(t, m.settings.IsOpen(), "%s closed the settings", key)
		require.NotNil(t, cmd, key)
		assert.Equal(t, settingsKeptMsg{}, cmd(), key)
		assert.Contains(t, plain(m.View().Content), listCursorGlyph+" "+settingsSignOut, key)
	}
}

// Case: the sign-out asked for, and done. The border says it is signing
// out meanwhile; then the settings offer to sign in, the search's results
// are gone and the border confirms it.
func TestSettings_SigningOut(t *testing.T) {
	cloud := &fakeCloud{signedIn: true}
	m := pressKey(searchModel(t, cloud), ",")
	m.search.Content = searchContent{asked: "bowie"}

	m, cmd := deliverCmd(m, settingsSignOutMsg{})
	assert.True(t, m.settings.NoticeShowing(), "the border does not say it is signing out")
	for _, msg := range flatten(cmd) {
		m = deliver(m, msg)
	}

	assert.Equal(t, 1, cloud.signOuts)
	require.True(t, m.settings.IsOpen())
	assert.True(t, m.settings.NoticeShowing())
	view := plain(m.View().Content)
	assert.Contains(t, view, settingsSignedOut)
	assert.Contains(t, view, listCursorGlyph+" "+settingsSignIn)
	assert.Empty(t, searchOf(m).asked, "the search kept the old session's results")
}

// Case: each way a sign-out can end, and what the border says. Sonos not
// told is a warning; a session left on the machine is an error.
func TestSettings_WhatEachSignOutSays(t *testing.T) {
	for name, tc := range map[string]struct {
		err   error
		text  string
		level panes.NoticeLevel
	}{
		"done":        {nil, settingsSignedOutOK, panes.NoticeInfo},
		"not told":    {fmt.Errorf("%w: offline", sonoscloud.ErrServerNotTold), settingsNotTold, panes.NoticeWarn},
		"not deleted": {errors.Join(fmt.Errorf("%w: disk", sonoscloud.ErrNotDeleted), sonoscloud.ErrServerNotTold), settingsNotDeleted, panes.NoticeError},
	} {
		notice := signedOutNotice(tc.err)
		assert.Equal(t, tc.text, notice.Text, name)
		assert.Equal(t, tc.level, notice.Level, name)
	}
}

// Case: "Sign in" in the settings, then a sign-in. The sign-in opens over the
// settings, and once signed in the settings show the session, not the search.
func TestSettings_SignInReturnsToTheSettings(t *testing.T) {
	cloud := &fakeCloud{}
	m := pressKey(searchModel(t, cloud), ",")

	m = deliver(m, settingsSignInMsg{})
	require.True(t, m.signIn.IsOpen())

	m, cmd := deliverCmd(m, signInAskMsg{username: "a@b", password: "pw"})
	for _, msg := range flatten(cmd) {
		m = deliver(m, msg)
	}

	assert.False(t, m.signIn.IsOpen())
	assert.False(t, m.search.IsOpen(), "the search opened after a sign-in from the settings")
	require.True(t, m.settings.IsOpen())
	assert.Contains(t, plain(m.View().Content), listCursorGlyph+" "+settingsSignOut)
}

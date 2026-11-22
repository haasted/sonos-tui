package main

import (
	"errors"
	"net"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// Case: an email and a password typed, q and ? among them. They are text:
// the app neither quits nor shows the help.
func TestSignIn_TheFieldsTakeEveryKey(t *testing.T) {
	m := pressKey(searchModel(t, &fakeCloud{}), "s")

	m = typeSearch(m, strings.Split("q?@x", "")...)
	m = typeSearch(pressKey(m, "tab"), "?", "q")

	assert.False(t, m.quitting, "q quit the app")
	assert.False(t, m.help.IsOpen(), "? opened the help")
	s := signInOf(m)
	assert.Equal(t, "q?@x", s.email.Value())
	assert.Equal(t, "?q", s.password.Value())
}

// Case: a password pasted, as a password manager does. It lands in the field
// the cursor is in.
func TestSignIn_APasteReachesTheField(t *testing.T) {
	m := pressKey(pressKey(searchModel(t, &fakeCloud{}), "s"), "tab")

	m = deliver(m, tea.PasteMsg{Content: "pasted-pw"})

	assert.Equal(t, "pasted-pw", signInOf(m).password.Value())
}

// Case: enter with a field empty. Nothing is sent; the cursor goes to the
// empty field.
func TestSignIn_EnterWithAFieldEmptySendsNothing(t *testing.T) {
	m := pressKey(searchModel(t, &fakeCloud{}), "s")

	// The command enter answers with is the cursor's blink in the field it
	// moves to, not a sign-in: the modal does not start one.
	m = pressKey(m, "enter")
	assert.False(t, signInOf(m).signingIn)
	assert.True(t, signInOf(m).email.Focused())

	m = pressKey(typeSearch(m, "a", "@", "b"), "enter")
	assert.False(t, signInOf(m).signingIn)
	assert.True(t, signInOf(m).password.Focused(), "the cursor did not go to the empty password")
}

// Case: enter with both fields filled. A sign-in is asked for with what was
// typed, and the password field is emptied at once.
func TestSignIn_EnterAsksForASignIn(t *testing.T) {
	m := typeSearch(pressKey(searchModel(t, &fakeCloud{}), "s"), " ", "a", "@", "b", " ")
	m = typeSearch(pressKey(m, "tab"), "p", "w")

	m, cmd := pressKeyCmd(m, "enter")

	require.NotNil(t, cmd)
	ask := cmd().(signInAskMsg)
	assert.Equal(t, "a@b", ask.username, "the username was not trimmed")
	assert.Equal(t, "pw", ask.password)
	assert.Empty(t, signInOf(m).password.Value(), "the password stayed in the field")
}

// Case: a sign-in that works, in the app. The cloud is asked, the border says
// it is signing in meanwhile, and then the sign-in closes and the search
// opens.
func TestSignIn_ASignInOpensTheSearch(t *testing.T) {
	cloud := &fakeCloud{}
	m := pressKey(searchModel(t, cloud), "s")

	m, cmd := deliverCmd(m, signInAskMsg{username: "a@b", password: "pw"})
	assert.True(t, m.signIn.NoticeShowing(), "the border does not say it is signing in")
	for _, msg := range flatten(cmd) {
		m = deliver(m, msg)
	}

	assert.Equal(t, []string{"a@b"}, cloud.signIns)
	assert.False(t, m.signIn.IsOpen(), "the sign-in stayed open")
	assert.True(t, m.search.IsOpen(), "the search did not open")
}

// Case: a sign-in that fails. The sign-in stays, the border says why, and the
// cursor waits in the password field for the next try.
func TestSignIn_AFailureIsSaidAndCanBeTriedAgain(t *testing.T) {
	m := typeSearch(pressKey(searchModel(t, &fakeCloud{signInErr: sonoscloud.ErrBadCredentials}), "s"), "a")
	m = typeSearch(pressKey(m, "tab"), "p")
	m, cmd := pressKeyCmd(m, "enter")
	m, signing := deliverCmd(m, cmd())

	for _, msg := range flatten(signing) {
		m = deliver(m, msg)
	}

	require.True(t, m.signIn.IsOpen())
	assert.True(t, m.signIn.NoticeShowing())
	assert.True(t, signInOf(m).password.Focused())
	assert.False(t, signInOf(m).signingIn, "the keys still wait")
}

// Case: each way a sign-in can fail, and what the border says. Nothing of
// what was typed is in it.
func TestSignIn_WhatEachFailureSays(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"bad credentials": {sonoscloud.ErrBadCredentials, signInIncorrect},
		"flow changed":    {sonoscloud.ErrFlowChanged, signInChanged},
		"no network":      {&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, signInOffline},
		"anything else":   {errors.New("okta answered 500"), signInFailed},
	} {
		assert.Equal(t, tc.want, signInProblem(tc.err), name)
	}
}

// Case: esc on the sign-in. It closes, and the search does not open: the app
// goes on without the cloud.
func TestSignIn_EscClosesWithoutTheSearch(t *testing.T) {
	m := pressKey(pressKey(searchModel(t, &fakeCloud{}), "s"), "esc")

	assert.False(t, m.signIn.IsOpen())
	assert.False(t, m.search.IsOpen())
}

// Case: a search the server refuses as signed out, then a sign-in. The
// sign-in opens over the search, and once signed in the query runs again.
func TestSignIn_ARefusedSearchRunsAgainAfterTheSignIn(t *testing.T) {
	cloud := &fakeCloud{signedIn: true, err: sonoscloud.ErrSignedOut}
	m := searchFor(t, searchModel(t, cloud), "bowie")
	require.True(t, m.signIn.IsOpen(), "a refused session did not open the sign-in")

	cloud.err, cloud.results = nil, sampleResults()
	m, cmd := deliverCmd(m, signInAskMsg{username: "a@b", password: "pw"})
	for _, msg := range flatten(cmd) {
		var next []tea.Msg
		m, next = deliverAll(m, msg)
		for _, answer := range next {
			m = deliver(m, answer)
		}
	}

	assert.Equal(t, []string{"bowie", "bowie"}, cloud.queries)
	assert.Equal(t, []string{"Low", "Heroes"}, resultNames(searchOf(m)))
	assert.False(t, searchOf(m).filtering, "the keys did not go to the results")
}

// Case: the session runs out while the search is open, and a new query is
// asked. Once signed in, that query runs, not the one before it.
func TestSignIn_TheQueryAskedSignedOutRunsAfterTheSignIn(t *testing.T) {
	cloud := &fakeCloud{signedIn: true}
	m := pressKey(searchModel(t, cloud), "s")
	m.search.Content = searchContent{asked: "bowie"}
	cloud.signedIn = false

	m = deliver(m, searchAskMsg{query: "stones"})
	require.True(t, m.signIn.IsOpen())
	cloud.signedIn = true
	m = deliver(m, signInDoneMsg{})

	assert.Equal(t, "stones", searchOf(m).asked)
	assert.True(t, searchOf(m).running, "the query did not run")
}

// Case: the sign-in left alone for the hint's time, and one with a key
// pressed first. Only the first shows the hint in its border.
func TestSignIn_TheHintComesAfterAWait(t *testing.T) {
	m := pressKey(searchModel(t, &fakeCloud{}), "s")
	waited := deliver(m, signInHintDueMsg{gen: m.signInGen})
	assert.True(t, waited.signIn.NoticeShowing())

	typed := pressKey(m, "a")
	typed = deliver(typed, signInHintDueMsg{gen: m.signInGen})
	assert.False(t, typed.signIn.NoticeShowing(), "the hint came though a key was pressed")
}

func signInOf(m appModel) signInContent {
	s, _ := m.signIn.Content.(signInContent)
	return s
}

func pressKeyCmd(m appModel, key string) (appModel, tea.Cmd) {
	model, cmd := m.Update(keyMsgFor(key))
	return model.(appModel), cmd
}

// deliverCmd delivers msg and gives the command the app answers with.
func deliverCmd(m appModel, msg tea.Msg) (appModel, tea.Cmd) {
	model, cmd := m.Update(msg)
	return model.(appModel), cmd
}

// deliverAll delivers msg and gives every message its command answers with.
func deliverAll(m appModel, msg tea.Msg) (appModel, []tea.Msg) {
	model, cmd := m.Update(msg)
	return model.(appModel), flatten(cmd)
}

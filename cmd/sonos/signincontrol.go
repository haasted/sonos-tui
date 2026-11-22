// How the application drives the sign-in modal: when it opens, signing in,
// what its border says, and what follows a sign-in. signin.go holds the modal
// itself.
package main

import (
	"errors"
	"log/slog"
	"net"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// What the sign-in's bottom border says. The hint comes after listHintDelay
// with no key, as in the list modals.
const (
	signInHint      = "enter signs in · esc closes"
	signInSigning   = "signing in…"
	signInIncorrect = "incorrect email or password"
	signInOffline   = "cannot reach Sonos; check the network"
	signInChanged   = "Sonos sign-in changed; cannot sign in"
	signInFailed    = "could not sign in"
)

// signInFollow is what shows once a sign-in has worked: what asked for it.
type signInFollow int

const (
	followSearch   signInFollow = iota // the search opens, and a stopped query runs again
	followSettings                     // the settings show the new session
)

type (
	// signInDoneMsg is how a sign-in went.
	signInDoneMsg struct{ err error }

	// signInHintDueMsg says the sign-in has waited listHintDelay for a key.
	// signInTickMsg is one frame of a notice in its border. gen tells them
	// from those of an earlier notice or opening.
	signInHintDueMsg struct{ gen int }
	signInTickMsg    struct{ gen int }
)

// openSignIn opens the sign-in, empty, over whatever is on screen, and
// remembers what shows once it has worked.
func (m appModel) openSignIn(then signInFollow) (appModel, tea.Cmd) {
	m.signInThen = then
	content, blink := newSignIn()
	m.signIn = m.signIn.Open()
	m.signIn.Content = content
	m.signInGen = nextGen()
	gen := m.signInGen
	return m, tea.Batch(blink, tea.Tick(listHintDelay, func(time.Time) tea.Msg { return signInHintDueMsg{gen: gen} }))
}

// updateSignIn hands a key to the sign-in. A key before the hint shows means
// the reader has found the keys; a notice already in the border plays out.
func (m appModel) updateSignIn(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, key := msg.(tea.KeyMsg); key && !m.signIn.NoticeShowing() {
		m.signInGen = nextGen()
	}
	modal, cmd := m.signIn.Update(msg)
	m.signIn = modal
	if !modal.IsOpen() {
		m.signInGen = nextGen() // the chains die with the modal
	}
	return m, cmd
}

// applySignInAsk signs in, in the background, and says so in the border.
func (m appModel) applySignInAsk(msg signInAskMsg) (tea.Model, tea.Cmd) {
	if m.cloud == nil {
		return m.signInFailedWith(signInFailed)
	}
	m, notice := m.signInNotice(panes.NoticeInfo, signInSigning)
	ctx, cloud := m.ctx, m.cloud
	return m, tea.Batch(notice, func() tea.Msg {
		return signInDoneMsg{err: cloud.SignIn(ctx, msg.username, msg.password)}
	})
}

// applySignInDone closes the sign-in and shows the search, or says in the
// border what went wrong and lets the reader try again.
func (m appModel) applySignInDone(msg signInDoneMsg) (tea.Model, tea.Cmd) {
	if !m.signIn.IsOpen() {
		return m, nil
	}
	if msg.err != nil {
		return m.signInFailedWith(signInProblem(msg.err))
	}
	m.signIn = m.signIn.Close()
	m.signInGen = nextGen()
	if m.signInThen == followSettings {
		return m.settingsSignedIn()
	}
	return m.afterSignIn()
}

// signInFailedWith puts the cursor back in the password field and says why.
func (m appModel) signInFailedWith(problem string) (tea.Model, tea.Cmd) {
	content, ok := m.signIn.Content.(signInContent)
	if !ok {
		return m, nil
	}
	content, focus := content.failed()
	m.signIn.Content = content
	m, notice := m.signInNotice(panes.NoticeError, problem)
	return m, tea.Batch(focus, notice)
}

// afterSignIn shows the search. A query the signed-out session stopped runs
// again, so the reader gets what they asked for.
func (m appModel) afterSignIn() (tea.Model, tea.Cmd) {
	content, _ := m.search.Content.(searchContent)
	rerun := content.problem == searchSignedOut && content.asked != ""
	model, cmd := m.openSearch()
	m = model.(appModel)
	if !rerun {
		return m, cmd
	}
	model, ask := m.applySearchAsk(searchAskMsg{query: content.asked})
	m = model.(appModel)
	if s, ok := m.search.Content.(searchContent); ok {
		s.filtering = false // the query runs; the keys go to its results
		m.search.Content = s
	}
	return m, tea.Batch(cmd, ask)
}

// signInNotice puts a notice in the sign-in's bottom border, in place of
// whatever is there, and starts the frames that draw it.
func (m appModel) signInNotice(level panes.NoticeLevel, text string) (appModel, tea.Cmd) {
	return m.signInNotify(panes.NoticeMsg{Level: level, Text: text})
}

func (m appModel) signInNotify(notice panes.NoticeMsg) (appModel, tea.Cmd) {
	m.signInGen = nextGen()
	m.signIn = m.signIn.Notify(notice)
	return m, signInTick(m.signInGen, m.signIn.NoticeInterval())
}

// applySignInHintDue blooms the hint into the border, unless a key or a
// notice has come since.
func (m appModel) applySignInHintDue(msg signInHintDueMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.signInGen || !m.signIn.IsOpen() {
		return m, nil
	}
	return m.signInNotify(panes.NoticeMsg{Level: panes.NoticeInfo, Text: signInHint, Entrance: panes.NoticeBloom})
}

// applySignInTick repaints a notice until it has gone from the border.
func (m appModel) applySignInTick(msg signInTickMsg) (tea.Model, tea.Cmd) {
	if msg.gen == m.signInGen && m.signIn.IsOpen() && m.signIn.NoticeShowing() {
		return m, signInTick(msg.gen, m.signIn.NoticeInterval())
	}
	return m, nil
}

func signInTick(gen int, every time.Duration) tea.Cmd {
	return tea.Tick(every, func(time.Time) tea.Msg { return signInTickMsg{gen: gen} })
}

// signInProblem is what the border says about a failed sign-in. Nothing of
// what the reader typed is in it, or in the log.
func signInProblem(err error) string {
	var netErr net.Error
	switch {
	case errors.Is(err, sonoscloud.ErrBadCredentials):
		return signInIncorrect
	case errors.Is(err, sonoscloud.ErrFlowChanged):
		return signInChanged
	case errors.As(err, &netErr):
		return signInOffline
	default:
		slog.Warn("signing in to the Sonos cloud", "err", err)
		return signInFailed
	}
}

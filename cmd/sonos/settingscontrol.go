// How the application drives the settings modal: opening it with the cloud
// as it is, what its border says, and what its action asks for. settings.go
// holds the modal itself.
package main

import (
	"errors"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// What the settings' bottom border says. The hint comes after listHintDelay
// with no key, as in the list modals.
const (
	settingsHint        = "enter chooses · esc closes"
	settingsKept        = "still signed in"
	settingsSignedInOK  = "signed in"
	settingsSigningOut  = "signing out…"
	settingsSignedOutOK = "signed out"
	settingsNotTold     = "signed out here; could not reach Sonos"
	settingsNotDeleted  = "could not delete the saved session"
)

type (
	// settingsHintDueMsg says the settings have waited listHintDelay for a
	// key. settingsTickMsg is one frame of a notice in their border. gen
	// tells them from those of an earlier notice or opening.
	settingsHintDueMsg struct{ gen int }
	settingsTickMsg    struct{ gen int }

	// settingsSignedOutMsg is how a sign-out went. The local data is gone
	// unless err says it could not be deleted.
	settingsSignedOutMsg struct{ err error }
)

// openSettings opens the settings with the cloud as it is now.
func (m appModel) openSettings() (appModel, tea.Cmd) {
	content, _ := m.settings.Content.(settingsContent)
	m.settings = m.settings.Open()
	m.settings.Content = content.showing(m.cloudState())
	m.settingsGen = nextGen()
	gen := m.settingsGen
	return m, tea.Tick(listHintDelay, func(time.Time) tea.Msg { return settingsHintDueMsg{gen: gen} })
}

// cloudState is the cloud as the settings show it, read without a request.
func (m appModel) cloudState() cloudState {
	if m.cloud == nil {
		return cloudState{}
	}
	return cloudState{available: true, signedIn: m.cloud.SignedIn(), ends: m.cloud.SessionEnds()}
}

// updateSettings hands a key to the settings. , closes them as it opened
// them. A key before the hint shows means the reader has found the keys.
func (m appModel) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.settings.NoticeShowing() {
		m.settingsGen = nextGen()
	}
	if content, _ := m.settings.Content.(settingsContent); msg.String() == "," && !content.confirming {
		m.settings = m.settings.Close()
		return m, nil
	}
	modal, cmd := m.settings.Update(msg)
	m.settings = modal
	if !modal.IsOpen() {
		m.settingsGen = nextGen() // the chains die with the modal
	}
	return m, cmd
}

// applySettingsSignOut signs out in the background and says so in the
// border. The settings take no keys until it is done.
func (m appModel) applySettingsSignOut(settingsSignOutMsg) (tea.Model, tea.Cmd) {
	if m.cloud == nil {
		return m, nil
	}
	m, notice := m.settingsNotify(panes.NoticeMsg{Level: panes.NoticeInfo, Text: settingsSigningOut})
	ctx, cloud := m.ctx, m.cloud
	return m, tea.Batch(notice, func() tea.Msg { return settingsSignedOutMsg{err: cloud.SignOut(ctx)} })
}

// applySettingsSignedOut drops what the app holds from the cloud, shows the
// settings signed out and says how the sign-out went.
func (m appModel) applySettingsSignedOut(msg settingsSignedOutMsg) (tea.Model, tea.Cmd) {
	if m.searchCancel != nil {
		m.searchCancel()
		m.searchCancel = nil
	}
	m.searchRun = nextGen() // an answer still on its way is for the old session
	m.search.Content = searchContent{}
	if !m.settings.IsOpen() {
		return m, nil
	}
	content, _ := m.settings.Content.(settingsContent)
	m.settings.Content = content.showing(m.cloudState())
	return m.settingsNotify(signedOutNotice(msg.err))
}

// signedOutNotice is what the border says about a sign-out. A failure to
// tell Sonos is a warning: nothing of the session is left here.
func signedOutNotice(err error) panes.NoticeMsg {
	switch {
	case err == nil:
		return panes.NoticeMsg{Level: panes.NoticeInfo, Text: settingsSignedOutOK, Confirmation: true}
	case errors.Is(err, sonoscloud.ErrNotDeleted):
		slog.Error("signing out of the Sonos cloud", "err", err)
		return panes.NoticeMsg{Level: panes.NoticeError, Text: settingsNotDeleted}
	}
	slog.Warn("telling Sonos about the sign-out", "err", err)
	return panes.NoticeMsg{Level: panes.NoticeWarn, Text: settingsNotTold}
}

// applySettingsKept says the sign-out asked for once was not done.
func (m appModel) applySettingsKept(settingsKeptMsg) (tea.Model, tea.Cmd) {
	return m.settingsNotify(panes.NoticeMsg{Level: panes.NoticeInfo, Text: settingsKept})
}

// applySettingsSignIn opens the sign-in over the settings, which show the
// new session once it has signed in.
func (m appModel) applySettingsSignIn(settingsSignInMsg) (tea.Model, tea.Cmd) {
	return m.openSignIn(followSettings)
}

// settingsSignedIn shows the settings again after the sign-in they asked for.
func (m appModel) settingsSignedIn() (tea.Model, tea.Cmd) {
	if !m.settings.IsOpen() {
		return m, nil
	}
	content, _ := m.settings.Content.(settingsContent)
	m.settings.Content = content.showing(m.cloudState())
	return m.settingsNotify(panes.NoticeMsg{Level: panes.NoticeInfo, Text: settingsSignedInOK, Confirmation: true})
}

// settingsNotify puts a notice in the settings' bottom border, in place of
// whatever is there, and starts the frames that draw it.
func (m appModel) settingsNotify(notice panes.NoticeMsg) (appModel, tea.Cmd) {
	m.settingsGen = nextGen()
	m.settings = m.settings.Notify(notice)
	return m, settingsTick(m.settingsGen, m.settings.NoticeInterval())
}

// applySettingsHintDue blooms the hint into the border, unless a key or a
// notice has come since.
func (m appModel) applySettingsHintDue(msg settingsHintDueMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.settingsGen || !m.settings.IsOpen() {
		return m, nil
	}
	return m.settingsNotify(panes.NoticeMsg{Level: panes.NoticeInfo, Text: settingsHint, Entrance: panes.NoticeBloom})
}

// applySettingsTick repaints a notice until it has gone from the border.
func (m appModel) applySettingsTick(msg settingsTickMsg) (tea.Model, tea.Cmd) {
	if msg.gen == m.settingsGen && m.settings.IsOpen() && m.settings.NoticeShowing() {
		return m, settingsTick(msg.gen, m.settings.NoticeInterval())
	}
	return m, nil
}

func settingsTick(gen int, every time.Duration) tea.Cmd {
	return tea.Tick(every, func(time.Time) tea.Msg { return settingsTickMsg{gen: gen} })
}

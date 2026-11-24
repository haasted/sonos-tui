package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/ui/panes"
	"github.com/haasted/sonos-tui/internal/ui/theme"
)

// What the settings say. The first section is the Sonos cloud.
const (
	settingsTitle      = "Settings"
	settingsCloud      = "Sonos cloud services"
	settingsSignedIn   = "Signed in"
	settingsEndsFmt    = "Signed in · the session ends on %s"
	settingsSignedOut  = "Not signed in"
	settingsNoCloud    = "Not available"
	settingsSignOut    = "Sign out"
	settingsConfirm    = "Sign out? enter again to confirm"
	settingsSignIn     = "Sign in"
	settingsRuleLength = 46
)

type (
	// settingsSignOutMsg asks for a sign-out: enter was pressed twice.
	settingsSignOutMsg struct{}

	// settingsSignInMsg asks for the sign-in modal.
	settingsSignInMsg struct{}

	// settingsKeptMsg says a sign-out was asked for once and then not.
	settingsKeptMsg struct{}
)

// cloudState is the cloud as the settings show it.
type cloudState struct {
	available bool      // there is a cloud client at all
	signedIn  bool      // a saved session has not run out
	ends      time.Time // when the session runs out; zero when its cookie does not say
}

// settingsContent is the settings modal's content. Today it holds one
// section, the Sonos cloud, with one action: sign out, or sign in.
type settingsContent struct {
	cloud      cloudState
	confirming bool // enter was pressed once on "Sign out"
	signingOut bool // a sign-out is on its way, so the keys wait
}

var (
	_ tea.Model        = settingsContent{}
	_ panes.EscClaimer = settingsContent{}
)

func (settingsContent) Init() tea.Cmd { return nil }

// ClaimsEsc keeps esc from closing the modal while a sign-out waits for its
// second enter: esc then keeps the session, as any other key does.
func (s settingsContent) ClaimsEsc() bool { return s.confirming }

// Update acts on the one action. A sign-out takes enter twice, so it is not
// done by accident; any other key between the two keeps the session.
func (s settingsContent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok || s.signingOut {
		return s, nil
	}
	if key.String() != "enter" {
		if s.confirming {
			s.confirming = false
			return s, func() tea.Msg { return settingsKeptMsg{} }
		}
		return s, nil
	}
	switch {
	case !s.cloud.available:
		return s, nil
	case !s.cloud.signedIn:
		return s, func() tea.Msg { return settingsSignInMsg{} }
	case !s.confirming:
		s.confirming = true
		return s, nil
	}
	s.confirming, s.signingOut = false, true
	return s, func() tea.Msg { return settingsSignOutMsg{} }
}

// showing is the settings with the cloud as it is now, and nothing waiting.
func (s settingsContent) showing(cloud cloudState) settingsContent {
	s.cloud, s.confirming, s.signingOut = cloud, false, false
	return s
}

func (s settingsContent) View() tea.View {
	var b strings.Builder
	b.WriteString(settingsTitleStyle().Render(settingsTitle) + "\n")
	b.WriteString(listRuleStyle().Render(strings.Repeat("─", settingsRuleLength)) + "\n")
	b.WriteString(searchHeadingStyle().Render(settingsCloud) + "\n")
	b.WriteString("  " + settingsQuietStyle().Render(s.cloudLine()))
	if action := s.action(); action != "" {
		style := lipgloss.NewStyle().Bold(true)
		if s.confirming {
			style = style.Foreground(settingsWarnColor())
		}
		b.WriteString("\n" + listCursorGlyph + " " + style.Render(action))
	}
	return tea.NewView(b.String())
}

// cloudLine says how the cloud is: signed in, and until when, or not.
func (s settingsContent) cloudLine() string {
	switch {
	case !s.cloud.available:
		return settingsNoCloud
	case !s.cloud.signedIn:
		return settingsSignedOut
	case s.cloud.ends.IsZero():
		return settingsSignedIn
	}
	return fmt.Sprintf(settingsEndsFmt, s.cloud.ends.Local().Format("2 January"))
}

// action is the row enter acts on, or empty with no cloud to act on.
func (s settingsContent) action() string {
	switch {
	case !s.cloud.available:
		return ""
	case !s.cloud.signedIn:
		return settingsSignIn
	case s.confirming:
		return settingsConfirm
	}
	return settingsSignOut
}

// settingsBorderColor is a quiet slate: the settings are about the client
// itself, not about music, so they take none of the lists' colours.
func settingsBorderColor() color.Color {
	return theme.LightDark(lipgloss.Color("#475569"), lipgloss.Color("#94A3B8"))
}

func settingsTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(settingsBorderColor())
}

func settingsQuietStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.LightDark(lipgloss.Color("#6B7280"), lipgloss.Color("#9CA3AF")))
}

// settingsWarnColor marks a sign-out waiting for its second enter.
func settingsWarnColor() color.Color {
	return theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24"))
}

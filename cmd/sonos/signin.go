package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/ui/theme"
)

const (
	signInFieldWidth = 32
	signInLabelWidth = 10
)

// signInTitle names the feature that asked for the sign-in, so the reader
// knows why they are asked. Everything else the modal says goes into its
// bottom border.
const signInTitle = "Sign in to Sonos cloud services to use search"

// signInAskMsg asks for a sign-in with what the reader typed. The password
// is in it only on its way to the sign-in.
type signInAskMsg struct{ username, password string }

// signInContent is the sign-in modal's content: the title and two fields.
type signInContent struct {
	email, password textinput.Model
	signingIn       bool // a sign-in is on its way, so the keys wait
}

var _ tea.Model = signInContent{}

// newSignIn is an empty sign-in, the cursor in the email field. Neither field
// is filled from before: the username is not kept.
func newSignIn() (signInContent, tea.Cmd) {
	email := textinput.New()
	email.Prompt, email.Placeholder = "", "you@example.com"
	email.SetWidth(signInFieldWidth)
	blink := email.Focus()

	password := textinput.New()
	password.Prompt = ""
	password.EchoMode, password.EchoCharacter = textinput.EchoPassword, '•'
	password.SetWidth(signInFieldWidth)

	return signInContent{email: email, password: password}, blink
}

func (signInContent) Init() tea.Cmd { return nil }

// Update moves between the fields, types into the one with the cursor, and
// asks for a sign-in on enter. Every key that types text is text here, q and
// ? too.
func (s signInContent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		if s.signingIn {
			return s, nil
		}
		switch key.String() {
		case "tab", "down", "shift+tab", "up":
			return s.switchField()
		case "enter":
			return s.submit()
		}
	}
	var cmd tea.Cmd
	if s.email.Focused() {
		s.email, cmd = s.email.Update(msg)
	} else {
		s.password, cmd = s.password.Update(msg)
	}
	return s, cmd
}

// switchField moves the cursor to the other field.
func (s signInContent) switchField() (signInContent, tea.Cmd) {
	if s.email.Focused() {
		s.email.Blur()
		return s, s.password.Focus()
	}
	s.password.Blur()
	return s, s.email.Focus()
}

// submit asks for a sign-in when both fields hold something, and otherwise
// moves to the empty one. The password field is emptied at once: the
// password is not held here a moment longer than it takes to send it.
func (s signInContent) submit() (signInContent, tea.Cmd) {
	username := strings.TrimSpace(s.email.Value())
	switch {
	case username == "":
		s.password.Blur()
		return s, s.email.Focus()
	case s.password.Value() == "":
		s.email.Blur()
		return s, s.password.Focus()
	}
	password := s.password.Value()
	s.password.SetValue("")
	s.signingIn = true
	return s, func() tea.Msg { return signInAskMsg{username: username, password: password} }
}

// failed lets the reader try again, the cursor in the emptied password field.
func (s signInContent) failed() (signInContent, tea.Cmd) {
	s.signingIn = false
	s.email.Blur()
	return s, s.password.Focus()
}

func (s signInContent) View() tea.View {
	return tea.NewView(signInTitleStyle().Render(signInTitle) + "\n\n" +
		s.field("Email", s.email) + "\n" +
		s.field("Password", s.password))
}

// field is one labelled field. The label of the field with the cursor is lit.
func (s signInContent) field(label string, input textinput.Model) string {
	style := lipgloss.NewStyle().Foreground(theme.LightDark(lipgloss.Color("#6B7280"), lipgloss.Color("#9CA3AF")))
	if input.Focused() {
		style = signInTitleStyle()
	}
	return style.Render(fmt.Sprintf("%-*s", signInLabelWidth, label)) + input.View()
}

// signInTitleStyle is the search's teal: the sign-in is for the cloud, as the
// search is.
func signInTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(searchBorderColor())
}

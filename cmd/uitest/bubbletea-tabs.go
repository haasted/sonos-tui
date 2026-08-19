package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
	"github.com/haasted/sonos-sandbox/pkg/ui/spinners"
)

// tabsRootModel is the outermost Bubbletea model for the "tabs" layout —
// wraps panes.Tabs with a q/ctrl+c quit binding so the playground has
// something runnable. It also drives a spinner next to the first tab, to
// demo a tab whose title updates independently of which tab is active.
type tabsRootModel struct {
	tabs         panes.Tabs
	spinner      spinner.Model
	spinnerTab   int  // always 0 (the first tab), kept as a field rather than a literal so it stays in one place
	spinnerStyle int  // index into spinnerStyles, cycled with "d"
	quitting     bool // View renders nothing once true, so the inline UI erases itself instead of leaving its last frame behind
}

// spinnerStyles are every named spinner style to cycle through with "d":
// bubbles/spinner's built-ins, plus our own from pkg/ui/spinners.
var spinnerStyles = []struct {
	name string
	spin spinner.Spinner
}{
	{"Dot", spinner.Dot},
	{"Equalizer", spinners.Equalizer},
	{"Bars", spinners.Bars},
	{"Jitter", spinners.Jitter},
	{"Line", spinner.Line},
	{"MiniDot", spinner.MiniDot},
	{"Jump", spinner.Jump},
	{"Pulse", spinner.Pulse},
	{"Points", spinner.Points},
	{"Globe", spinner.Globe},
	{"Moon", spinner.Moon},
	{"Monkey", spinner.Monkey},
	{"Meter", spinner.Meter},
	{"Hamburger", spinner.Hamburger},
	{"Ellipsis", spinner.Ellipsis},
}

var roomNames = []string{
	"Kitchen",
	"Living Room",
	"Bedroom",
	"Bathroom",
	"Office",
	"Dining Room",
	"Garage",
	"Patio",
}

func newTabsRootModel(pos panes.Position) tabsRootModel {
	tabs := make([]panes.Tab, len(roomNames))
	for i, room := range roomNames {
		content := fmt.Sprintf("Nothing playing yet in the %s.", room)
		tabs[i] = panes.Tab{Title: room, Content: panes.StaticContent(content)}
	}
	t := panes.NewTabs(tabs)
	t.Position = pos

	s := spinner.New()
	s.Spinner = spinnerStyles[0].spin

	m := tabsRootModel{tabs: t, spinner: s, spinnerTab: 0}
	m.updateSpinnerTabTitle()
	return m
}

// updateSpinnerTabTitle stamps the current spinner frame and style name onto
// the chosen tab's title, so it animates regardless of which tab is active
// and the style is identifiable while cycling with "d".
func (m *tabsRootModel) updateSpinnerTabTitle() {
	name := spinnerStyles[m.spinnerStyle].name
	m.tabs.Tabs[m.spinnerTab].Title = fmt.Sprintf("%s %s (%s)", roomNames[m.spinnerTab], m.spinner.View(), name)
}

func (m tabsRootModel) Init() tea.Cmd {
	return tea.Batch(m.tabs.Init(), m.spinner.Tick)
}

func (m tabsRootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "d":
			m.spinnerStyle = (m.spinnerStyle + 1) % len(spinnerStyles)
			m.spinner.Spinner = spinnerStyles[m.spinnerStyle].spin
			m.updateSpinnerTabTitle()
			return m, nil
		}
	}

	if _, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.updateSpinnerTabTitle()
		return m, cmd
	}

	tabs, cmd := m.tabs.Update(msg)
	m.tabs = tabs
	return m, cmd
}

func (m tabsRootModel) View() string {
	if m.quitting {
		return ""
	}
	return m.tabs.View() + "\n\ntab/shift+tab or ←/→ to switch tabs · 1-8 to jump, 0 for last · d to cycle spinner style · q to quit\n"
}

func layoutTabs() {
	runTabs(panes.Top)
}

// layoutTabsBottom is the same component with Position: Bottom, to compare side by side with layoutTabs.
func layoutTabsBottom() {
	runTabs(panes.Bottom)
}

func runTabs(pos panes.Position) {
	// No WithAltScreen: render inline, starting at the current cursor row,
	// so scrollback above is left untouched. View() returning "" once
	// quitting is set (see above) makes Bubbletea's renderer erase the UI's
	// own lines on the way out instead of leaving the last frame behind.
	if _, err := tea.NewProgram(newTabsRootModel(pos)).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

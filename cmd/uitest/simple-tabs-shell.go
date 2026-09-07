package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// simpleTabsModel is the minimal Bubbletea root model every plain
// Tabs-wrapped layout in this playground needs: q/ctrl+c to quit, inline
// (no alt-screen) rendering that erases itself on the way out, and
// WindowSizeMsg forwarded so Tabs can window its header row. Pulled out
// once "interface" mockups started needing the exact same shell "tabs" and
// "effects" already had, copy-pasted twice.
type simpleTabsModel struct {
	tabs     panes.Tabs
	quitting bool

	modal    altScreenModal
	hasModal bool // opt-in: layouts that never ask for one behave exactly as before
	width    int
	height   int
}

func newSimpleTabsModel(tabs panes.Tabs) simpleTabsModel {
	return simpleTabsModel{tabs: tabs}
}

// withModal enables the son-911 overlay prototype, opened with "?". Opt-in so
// the four other layouts sharing this shell are unaffected.
func (m simpleTabsModel) withModal() simpleTabsModel {
	m.hasModal = true
	return m
}

func (m simpleTabsModel) Init() tea.Cmd {
	return m.tabs.Init()
}

func (m simpleTabsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.tabs.Width = sizeMsg.Width
		m.width, m.height = sizeMsg.Width, sizeMsg.Height
		m.modal.width, m.modal.height = sizeMsg.Width, sizeMsg.Height
		if m.modal.open {
			m.modal.resizes++
		}
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		key := keyMsg.String()

		// While open the modal owns the keyboard, so the tabs underneath
		// cannot be navigated behind it — the same rule son-quf will need.
		if m.modal.open {
			switch key {
			case "?", "esc", "q":
				m.modal = m.modal.toggle()
			case "m":
				m.modal = m.modal.cycle()
			}
			return m, nil
		}

		switch key {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "?":
			if m.hasModal {
				m.modal = m.modal.toggle()
				return m, nil
			}
		}
	}

	tabs, cmd := m.tabs.Update(msg)
	m.tabs = tabs
	return m, cmd
}

func (m simpleTabsModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	// AltScreen is a per-frame property in Bubbletea v2, not a command: the
	// renderer switches whenever it differs from the previous frame.
	if m.modal.open {
		view := tea.NewView(m.modal.ViewOver(m.appView()))
		view.AltScreen = m.modal.altScreen()
		return view
	}
	return tea.NewView(m.appView())
}

// appView is the layout itself, rendered the same whether it stands alone
// inline or serves as the backdrop the modal floats over.
func (m simpleTabsModel) appView() string {
	help := "tab/shift+tab or ←/→ to switch tabs · q to quit"
	if m.hasModal {
		help = "tab/shift+tab or ←/→ to switch tabs · ? for the modal · q to quit"
	}
	return m.tabs.View() + "\n\n" + help + "\n"
}

func runSimpleTabs(tabs panes.Tabs) {
	runShell(newSimpleTabsModel(tabs))
}

// runSimpleTabsWithModal is runSimpleTabs with the son-911 overlay enabled.
func runSimpleTabsWithModal(tabs panes.Tabs) {
	runShell(newSimpleTabsModel(tabs).withModal())
}

func runShell(model simpleTabsModel) {
	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

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

	modal    panes.Modal
	hasModal bool // opt-in: layouts that never ask for one behave exactly as before
	resizes  int  // resizes seen while the modal is open, shown in the demo content
	width    int
	height   int
}

func newSimpleTabsModel(tabs panes.Tabs) simpleTabsModel {
	return simpleTabsModel{tabs: tabs}
}

// withModal enables the overlay prototype, opened with "?". Opt-in so
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
		m.modal = m.modal.SetSize(sizeMsg.Width, sizeMsg.Height)
		if m.modal.IsOpen() {
			m.resizes++
		}
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		key := keyMsg.String()

		// An open modal owns the keyboard, so the tabs underneath cannot be
		// navigated behind it. panes.Modal handles esc itself; the rest of
		// these are this playground's own bindings.
		if m.modal.IsOpen() {
			switch key {
			case "?", "q":
				m.modal = m.modal.Close()
			case "m":
				// The two this demo is about, named rather than counted: a
				// third mode existing does not belong in its cycle.
				if m.modal.Mode == panes.ModalFullScreen {
					m.modal.Mode = panes.ModalHover
				} else {
					m.modal.Mode = panes.ModalFullScreen
				}
			default:
				modal, cmd := m.modal.Update(msg)
				m.modal = modal
				return m, cmd
			}
			return m, nil
		}

		switch key {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "?":
			if m.hasModal {
				m.modal = m.modal.Open()
				m.resizes = 0
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
	if m.modal.IsOpen() {
		m.modal.Content = m.demoContent()
		view := tea.NewView(m.modal.ViewOver(m.appView()))
		view.AltScreen = m.modal.AltScreen()
		return view
	}
	return tea.NewView(m.appView())
}

// demoContent rebuilds the throwaway modal body with the sizes the component
// has settled on, so the overlay reports what it actually did.
func (m simpleTabsModel) demoContent() modalDemo {
	w, h := m.modal.BoxSize()
	return modalDemo{
		mode:     m.modal.Mode,
		terminal: [2]int{m.width, m.height},
		box:      [2]int{w, h},
		resizes:  m.resizes,
	}
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

// runSimpleTabsWithModal is runSimpleTabs with the overlay prototype enabled.
func runSimpleTabsWithModal(tabs panes.Tabs) {
	runShell(newSimpleTabsModel(tabs).withModal())
}

func runShell(model simpleTabsModel) {
	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

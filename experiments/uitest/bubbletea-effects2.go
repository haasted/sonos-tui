package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// effects2RootModel mirrors effectsRootModel, but each tab demonstrates a
// piece of the vendored movy graphics core (internal/ui/movy) instead of a
// standalone lipgloss/go-colorful effect.
type effects2RootModel struct {
	tabs     panes.Tabs
	quitting bool
}

func newEffects2RootModel() effects2RootModel {
	t := panes.NewTabs([]panes.Tab{
		{Title: "Text Reveal", Content: newTextRevealEffect()},
		{Title: "Glow Ring", Content: newGlowRingEffect()},
		{Title: "CRT Grid", Content: newCRTGridEffect()},
		{Title: "Sprite Walk", Content: newSpriteWalkEffect()},
	})
	return effects2RootModel{tabs: t}
}

func (m effects2RootModel) Init() tea.Cmd {
	return m.tabs.Init()
}

func (m effects2RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sizeMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.tabs.Width = sizeMsg.Width
		return m, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}

	tabs, cmd := m.tabs.Update(msg)
	m.tabs = tabs
	return m, cmd
}

func (m effects2RootModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	return tea.NewView(m.tabs.View() + "\n\ntab/shift+tab or ←/→ to switch tabs · q to quit\n")
}

func layoutEffects2() {
	if _, err := tea.NewProgram(newEffects2RootModel()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// bubbleteaModel is the smallest possible Bubbletea program: static view, quit on q/ctrl+c.
type bubbleteaModel struct{}

func (m bubbleteaModel) Init() tea.Cmd { return nil }

func (m bubbleteaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m bubbleteaModel) View() string {
	return "Sonos UI — bubbletea test layout\n\nPress q to quit.\n"
}

// layoutBubbletea runs the model above as a full Bubbletea program.
func layoutBubbletea() {
	if _, err := tea.NewProgram(bubbleteaModel{}).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

package panes

import tea "charm.land/bubbletea/v2"

// StaticContent is a tea.Model that renders fixed text unconditionally —
// handy as Tab content in tests/demos, or as a placeholder for panes not
// built yet.
type StaticContent string

func (c StaticContent) Init() tea.Cmd                       { return nil }
func (c StaticContent) Update(tea.Msg) (tea.Model, tea.Cmd) { return c, nil }
func (c StaticContent) View() tea.View                      { return tea.NewView(string(c)) }

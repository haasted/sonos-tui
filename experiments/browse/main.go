// Command browse is a mock-up of the client's favourites modal, to try out
// browsing into a favourite before the real one changes. The modal's code
// is copied from cmd/sonos; the favourites are made up, and nothing plays.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/store"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// mockModel holds the favourites modal over an empty screen, and says on the
// last line what a key would have done in the client.
type mockModel struct {
	modal         panes.Modal
	pulse         int    // the generation of the live repaint chain
	said          string // what the last action would have done
	width, height int
}

func main() {
	if _, err := tea.NewProgram(newMock()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newMock() mockModel {
	modal := panes.NewModal(favouritesContent{})
	modal.Mode = panes.ModalFixed
	modal.Color = favouritesBorderColor()
	modal = modal.Open()
	modal.Content = favouritesContent{listView: listView[store.Favourite]{openedAt: time.Now()}}.loaded(
		favouritesLoadedMsg{favourites: mockFavourites(), kind: store.FavouriteAlbum})
	return mockModel{modal: modal, said: "esc or ctrl+c quits"}
}

func (m mockModel) Init() tea.Cmd { return m.restartChain() }

func (m mockModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.modal = m.modal.SetSize(msg.Width, msg.Height)
		m.modal.Content = m.content().resized(m.modal.ContentRows(), m.modal.ContentColumns())
		return m, nil
	case listTickMsg:
		if msg.gen == m.pulse && m.content().animating() {
			return m, listTick(msg.gen)
		}
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m.forward(msg)
	case tea.MouseWheelMsg:
		return m.forward(msg)
	case favouriteActionMsg:
		m.said = fmt.Sprintf("would %s %q", actionName(msg.action), msg.favourite.Title)
	case favouriteRemoveMsg:
		m.said = fmt.Sprintf("would remove %q", msg.favourite.Title)
	case favouriteRestoreMsg:
		m.said = fmt.Sprintf("would restore %q", msg.favourite.Title)
	case favouriteFadedMsg:
		content, cmd := m.content().faded(msg.gen)
		m.modal.Content = content
		return m, cmd
	case favouriteArrivedMsg:
		m.modal.Content = m.content().arrived(msg.gen)
	}
	return m, nil
}

// forward hands a key or the wheel to the modal, and starts a fresh repaint
// chain so a bump it lights is drawn. Closing the modal ends the mock-up.
func (m mockModel) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.modal, cmd = m.modal.Update(msg)
	if !m.modal.IsOpen() {
		return m, tea.Quit
	}
	return m, tea.Batch(cmd, m.restartChain())
}

func (m *mockModel) restartChain() tea.Cmd {
	m.pulse = nextGen()
	return listTick(m.pulse)
}

func (m mockModel) content() favouritesContent { return m.modal.Content.(favouritesContent) }

func (m mockModel) View() tea.View {
	background := strings.Repeat("\n", max(m.height-1, 0)) + lipgloss.NewStyle().Faint(true).Render(m.said)
	modal := m.modal
	modal.Light = m.content().borderLight()
	return tea.NewView(modal.ViewOver(background))
}

func actionName(a favouriteAction) string {
	switch a {
	case favouritePlayNext:
		return "play next"
	case favouriteAddToQueue:
		return "add to the end of the queue"
	case favouritePlayShuffled:
		return "play shuffled"
	}
	return "play"
}

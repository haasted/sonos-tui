// The modal that shows which speakers a room can be grouped with. Nothing
// here changes a grouping yet: it lists the candidates and closes again.
package main

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/sonos"
	"github.com/haasted/sonos-tui/internal/ui/theme"
)

// groupRow is one speaker the room could be grouped with, and whether it is
// to be. Picked means IN THE GROUP, so a row starts picked when the speaker
// is already playing with this room: the list is the group as it would be,
// not a set of changes to it.
type groupRow struct {
	id     string
	room   string
	picked bool
}

// groupContent lists the speakers one room can be grouped with: the rest of
// its own Sonos system, whether or not they are already playing with it.
//
// It holds rows rather than reading the household as it draws, for the
// reason queueContent holds items: what is on screen should be what the
// reader was shown when it opened, not whatever has arrived since.
type groupContent struct {
	room   string
	rows   []groupRow
	cursor int
	width  int
}

var _ tea.Model = groupContent{}

func (groupContent) Init() tea.Cmd { return nil }

// Update moves the cursor and picks rows out. esc is not handled here, so
// it reaches the modal and closes it, as it does for the help popup.
func (g groupContent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return g, nil
	}

	switch key.String() {
	case "up", "k":
		g.cursor = max(g.cursor-1, 0)
	case "down", "j":
		g.cursor = min(g.cursor+1, g.lastRow())
	case "home", "g":
		g.cursor = 0
	case "end", "G":
		g.cursor = g.lastRow()
	case "s", "right":
		return g.toggle()
	}
	return g, nil
}

// toggle puts the row under the cursor in the group or takes it out, and
// steps down: marking a run is then s, s, s rather than a step between each,
// as it is in the queue.
//
// The speaker is told AT ONCE. There is no confirming and no applying on
// close: the row changing is the reader saying what they want, and the only
// thing left is for the speakers to agree. The row is drawn its new way
// before they have, because the alternative is a key that does nothing for
// a round trip; the topology event that follows is what makes it true.
func (g groupContent) toggle() (groupContent, tea.Cmd) {
	if g.cursor >= len(g.rows) {
		return g, nil
	}

	// Copied rather than written to. Every copy of this content shares the
	// array behind the rows, so a write would reach copies meant to be
	// thrown away -- the reason queueContent clones its own.
	rows := slices.Clone(g.rows)
	row := &rows[g.cursor]
	row.picked = !row.picked
	g.rows = rows

	change := groupChangeMsg{id: row.id, room: row.room, join: row.picked}
	g.cursor = min(g.cursor+1, g.lastRow())
	return g, func() tea.Msg { return change }
}

func (g groupContent) lastRow() int { return max(len(g.rows)-1, 0) }

// picked is every speaker the reader has left in the group, which is what a
// command will be built from once there is one.
func (g groupContent) picked() []string {
	var ids []string
	for _, row := range g.rows {
		if row.picked {
			ids = append(ids, row.id)
		}
	}
	return ids
}

func (g groupContent) View() tea.View {
	var b strings.Builder
	b.WriteString(groupHeadingStyle.Render(g.heading()) + "\n")
	b.WriteString(listRuleStyle().Render(strings.Repeat("─", g.ruleWidth())) + "\n")

	if len(g.rows) == 0 {
		b.WriteString(listNoteStyle().Render(g.note()))
		return tea.NewView(b.String())
	}

	width := g.ruleWidth()
	for i, row := range g.rows {
		mark := " "
		if i == g.cursor {
			mark = listCursorGlyph
		}

		style := groupRowStyle
		if row.picked {
			// The same background the queue marks a picked track with: a row
			// here means the same thing, chosen rather than merely under the
			// cursor.
			style = style.Background(themedSelectedBackground())
		}
		b.WriteString(style.Render(padRight(mark+" "+row.room, width)) + "\n")
	}
	return tea.NewView(strings.TrimRight(b.String(), "\n"))
}

func (g groupContent) heading() string {
	if len(g.rows) == 0 {
		return g.room
	}
	return fmt.Sprintf("%s · group with", g.room)
}

// note stands in for the list when there is nobody to offer. A speaker on
// its own system has no companions, which is not the same as an error.
func (g groupContent) note() string {
	return "No other speakers to group " + g.room + " with."
}

func (g groupContent) ruleWidth() int {
	widest := lipgloss.Width(g.heading())
	for _, row := range g.rows {
		widest = max(widest, lipgloss.Width(row.room)+listMarkWidth)
	}
	return max(widest, lipgloss.Width(g.note()))
}

// groupingFor builds the list for one speaker, in the order the household
// reports them.
func groupingFor(room string, companions []sonos.ZoneMember, playingWith map[string]bool) groupContent {
	rows := make([]groupRow, 0, len(companions))
	for _, member := range companions {
		rows = append(rows, groupRow{
			id: member.ID, room: member.RoomName, picked: playingWith[member.ID],
		})
	}
	return groupContent{room: room, rows: rows}
}

var (
	groupHeadingStyle = lipgloss.NewStyle().Bold(true).
				Foreground(theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24")))

	groupRowStyle = lipgloss.NewStyle().
			Foreground(theme.LightDark(lipgloss.Color("#1F2937"), lipgloss.Color("#E5E7EB")))
)

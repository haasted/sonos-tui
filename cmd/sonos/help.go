package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/ui/theme"
)

// helpKey is one row of the help popup: the keys that do a thing, and the
// thing. Deliberately a literal table rather than generated from the
// bindings, which live across speakerModel, appModel and panes.Tabs — see
// TestHelp_DocumentsEveryKeyTheAppHandles for the guard against it drifting.
type helpKey struct {
	keys string
	what string
}

const (
	helpKeyColumn = 11 // width of the key column, so descriptions line up
	helpColumnGap = 4
)

var (
	helpSectionStyle = lipgloss.NewStyle().Bold(true).
				Foreground(theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24")))

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(theme.LightDark(lipgloss.Color("#1F2937"), lipgloss.Color("#E5E7EB")))

	helpWhatStyle = lipgloss.NewStyle().
			Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))
)

// helpSection is one titled group of keys.
type helpSection struct {
	title string
	keys  []helpKey
}

// helpTable is a whole popup, as columns of sections. A popup that clips is
// worse than useless, so a table wide enough to need two columns says so here
// rather than being wrapped at render time.
type helpTable [][]helpSection

// playerHelp is what "?" shows over the player. It holds no queue keys: those
// mean nothing until the queue is open, and the queue carries its own table.
var playerHelp = helpTable{
	{
		{"Playback", []helpKey{
			{"p  k", "play / pause"},
			{"j", "previous track"},
			{"l", "next track"},
			{"enter", "show the queue"},
			{"g", "group with"},
			{"f", "favourites"},
			{"a", "mark as a favourite"},
		}},
		{"Sound", []helpKey{
			{"↑  ↓", "volume"},
			{"m", "mute"},
		}},
	},
	{
		{"Rooms", []helpKey{
			{"→  tab", "next room"},
			{"←  ⇧tab", "previous room"},
			{"1 … 9", "jump to room"},
			{"0", "last room"},
		}},
		{"App", []helpKey{
			{"esc", "close"},
			{"q", "quit"},
		}},
	},
}

// queueHelp is what "?" shows over the queue. One column: the list holds one
// kind of key, and a second column would only spread six rows thinner.
var queueHelp = helpTable{
	{
		{"Queue", []helpKey{
			{"↑ ↓  k j", "move a row"},
			{"pgup  F", "page up"},
			{"pgdn f spc", "page down"},
			{"^u  ^d", "half a page"},
			{"home  g", "to the top"},
			{"end  G", "to the end"},
			{"c", "to what is playing"},
			{"x  delete", "remove a track"},
			{"n", "play next"},
			{"N", "send to the bottom"},
			{"enter", "play from here"},
			{"a", "mark track as a favourite"},
			{"/", "filter the list"},
			{"s  →", "select / deselect"},
			{"S", "select none"},
			{"*", "invert the selection"},
			{"J K ⇧↑ ⇧↓", "move a track"},
			{"esc", "close"},
		}},
	},
}

// favouritesHelp is what "?" shows over the favourites. The list moves with
// the queue's keys, and ← → switch between the kinds of favourite.
var favouritesHelp = helpTable{
	{
		{"Favourites", []helpKey{
			{"↑ ↓  k j", "move a row"},
			{"pgup  F", "page up"},
			{"pgdn f spc", "page down"},
			{"^u  ^d", "half a page"},
			{"home  g", "to the top"},
			{"end  G", "to the end"},
			{"←  →", "tracks, albums, playlists, stations"},
			{"enter", "play, replacing the queue"},
			{"⇧enter", "play shuffled, replacing the queue"},
			{"n", "play next"},
			{"N", "add to the end of the queue"},
			{"x  delete", "remove a favourite"},
			{"X", "restore the last favourite removed"},
			{"/", "filter the list"},
			{"esc", "close"},
		}},
	},
}

// helpContent renders one shortcut table as modal content. Which table it
// holds is the caller's choice, so the popup shows the keys that work where
// the reader is rather than every key the program has.
type helpContent struct {
	table helpTable
}

var _ tea.Model = helpContent{}

func (helpContent) Init() tea.Cmd { return nil }

func (h helpContent) Update(tea.Msg) (tea.Model, tea.Cmd) { return h, nil }

func (h helpContent) View() tea.View {
	columns := make([]string, 0, len(h.table))
	for _, sections := range h.table {
		var b strings.Builder
		for j, section := range sections {
			if j > 0 {
				b.WriteString("\n")
			}
			b.WriteString(helpSectionStyle.Render(section.title) + "\n")
			for _, key := range section.keys {
				b.WriteString(helpKeyStyle.Render(padRight(key.keys, helpKeyColumn)) +
					helpWhatStyle.Render(key.what) + "\n")
			}
		}
		columns = append(columns, strings.TrimRight(b.String(), "\n"))
	}

	// Interleaved rather than joined with a separator: JoinHorizontal lays each
	// argument out as its own block, and one pre-joined string would lose that.
	gap := strings.Repeat(" ", helpColumnGap)
	blocks := make([]string, 0, 2*len(columns))
	for i, column := range columns {
		if i > 0 {
			blocks = append(blocks, gap)
		}
		blocks = append(blocks, column)
	}
	return tea.NewView(lipgloss.JoinHorizontal(lipgloss.Top, blocks...))
}

// padRight pads to exactly a display width, so a column lines up whatever it
// holds. A string already at or past the width is returned untouched: adding a
// space to it would push that one row out of line with every other.
// lipgloss.Width rather than len, because the arrow glyphs are multi-byte.
func padRight(s string, width int) string {
	if pad := width - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

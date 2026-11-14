// The small modal that asks what a mark in the queue is for: the track under
// the cursor, or the playlist or album the queue was started from.
package main

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/sonos"
	"github.com/haasted/sonos-tui/internal/store"
)

// markChooserHeading names the chooser.
const markChooserHeading = "Mark as a favourite"

type (
	// favouriteChooseMsg asks which of the track and the queue's playlist or
	// album to mark.
	favouriteChooseMsg struct {
		track  sonos.TrackMetadata
		source sonos.QueueSource
	}

	// favouriteChosenMsg is the reader's choice, ready to store.
	favouriteChosenMsg struct{ fav store.Favourite }

	// favouriteStoredMsg says which of the chooser's rows are favourites
	// already, so the chooser opens only when it has something to ask.
	favouriteStoredMsg struct {
		rows   []markRow
		stored []bool
	}
)

// markRow is one thing the chooser offers to mark.
type markRow struct {
	label string // what it is and its name, such as "Track: Enter Sandman"
	fav   store.Favourite
}

// markContent lists what can be marked, the track first. It holds the rows
// it was opened with, so what is offered does not change under the reader.
type markContent struct {
	rows   []markRow
	cursor int
}

var _ tea.Model = markContent{}

func (markContent) Init() tea.Cmd { return nil }

// Update moves the cursor and chooses with enter. esc is not handled here, so
// it reaches the modal and closes it with nothing marked.
func (c markContent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil
	}
	switch key.String() {
	case "up", "k":
		c.cursor = max(c.cursor-1, 0)
	case "down", "j":
		c.cursor = min(c.cursor+1, len(c.rows)-1)
	case "enter":
		if c.cursor < len(c.rows) {
			fav := c.rows[c.cursor].fav
			return c, func() tea.Msg { return favouriteChosenMsg{fav: fav} }
		}
	}
	return c, nil
}

func (c markContent) View() tea.View {
	width := lipgloss.Width(markChooserHeading)
	for _, row := range c.rows {
		width = max(width, lipgloss.Width(row.label)+listMarkWidth)
	}

	var b strings.Builder
	b.WriteString(favouritesHeadingStyle().Render(markChooserHeading) + "\n")
	b.WriteString(listRuleStyle().Render(strings.Repeat("─", width)) + "\n")
	for i, row := range c.rows {
		mark := " "
		if i == c.cursor {
			mark = listCursorGlyph
		}
		b.WriteString(groupRowStyle.Render(padRight(mark+" "+row.label, width)) + "\n")
	}
	return tea.NewView(strings.TrimRight(b.String(), "\n"))
}

// markRowsFor is what the chooser offers, in this order: the track, then the
// album or the playlist the queue was started from. Only what can be marked
// is offered.
func markRowsFor(track sonos.TrackMetadata, source sonos.QueueSource, now time.Time) []markRow {
	var rows []markRow
	if fav, err := trackFavourite(track, now); err == nil {
		rows = append(rows, markRow{label: "Track: " + track.Title, fav: fav})
	}
	if fav, ok := sourceFavourite(source, now); ok {
		label := "Playlist: "
		if fav.Kind == store.FavouriteAlbum {
			label = "Album: "
		}
		rows = append(rows, markRow{label: label + source.Title, fav: fav})
	}
	return rows
}

// Browsing from the search: stepping into an artist to see its albums, and
// back out to the results as they were. search.go holds the modal itself.
package main

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
	"github.com/haasted/sonos-tui/internal/ui/theme"
)

// Rows a level shows that are not results. They are not types the content
// API sends.
const (
	rowBack    sonoscloud.ResultType = "back"    // the first row of a level, which goes back
	rowHeading sonoscloud.ResultType = "heading" // a group heading, which the cursor passes over
)

const searchOpeningFmt = "Opening %s…"

// backRow is the row at the top of a level. It matches every filter, so it is
// always there.
var backRow = sonoscloud.Result{Type: rowBack, Name: "← back"}

// artistGroups are the groups an artist's albums are shown in, in this order.
var artistGroups = []struct {
	name  string
	kinds []sonoscloud.AlbumKind
}{
	{"Albums", []sonoscloud.AlbumKind{sonoscloud.KindAlbum}},
	{"Singles & EPs", []sonoscloud.AlbumKind{sonoscloud.KindSingle, sonoscloud.KindEP}},
}

// artistFilter matches an album by its title. The back row matches every
// filter and the headings none, so a filter flattens the groups.
var artistFilter = listFilter[sonoscloud.Result]{
	match: func(r sonoscloud.Result, text string) bool {
		switch r.Type {
		case rowBack:
			return true
		case rowHeading:
			return false
		}
		return anyContains(text, r.Name)
	},
	same: func(a, b sonoscloud.Result) bool {
		return a.Type == b.Type && a.ID.ObjectID == b.ID.ObjectID && a.Name == b.Name
	},
}

type (
	// searchOpenMsg asks for an artist to be read from the cloud, for the
	// level opened with gen.
	searchOpenMsg struct {
		result sonoscloud.Result
		gen    int
	}

	// artistOpenedMsg is an artist read from the cloud, for the level opened
	// with gen.
	artistOpenedMsg struct {
		gen     int
		localID string
		artist  sonoscloud.Artist
		err     error
	}
)

func isHeading(r sonoscloud.Result) bool { return r.Type == rowHeading }

// open steps into the artist under the cursor and asks for its albums. On
// anything else it lights the right edge, as pressing against the end of a
// list does.
func (s searchContent) open() (searchContent, tea.Cmd) {
	if s.cursor >= len(s.items) || s.items[s.cursor].Type != sonoscloud.TypeArtist {
		s.listView = s.bump(bumpRight)
		return s, nil
	}
	artist := s.items[s.cursor]
	gen := nextGen()
	s.levels = append(slices.Clone(s.levels), browseLevel[sonoscloud.Result]{name: artist.Name, saved: s.listView, gen: gen})
	s.items, s.all, s.filter, s.filtering = []sonoscloud.Result{backRow}, nil, "", false
	s.cursor, s.offset, s.skip = 0, 0, isHeading
	s.opening, s.levelProblem = true, ""
	return s, func() tea.Msg { return searchOpenMsg{result: artist, gen: gen} }
}

// opened shows an artist's albums, if the level on top is still the one they
// were asked for. The cursor starts on the first album, below the back row.
func (s searchContent) opened(gen int, artist sonoscloud.Artist) searchContent {
	if !s.waitingFor(gen) {
		return s
	}
	s.opening = false
	s.items, s.all = artistRows(artist), nil
	s.cursor, s.offset = s.landing(min(1, len(s.items)-1), 1), 0
	s.listView = s.follow()
	return s
}

// openFailed says why the level on top has nothing to show, if it is still
// the one that was asked for.
func (s searchContent) openFailed(gen int, problem string) searchContent {
	if !s.waitingFor(gen) {
		return s
	}
	s.opening, s.levelProblem = false, problem
	return s
}

// waitingFor reports the level on top as the one opened with gen, still
// waiting for what it holds.
func (s searchContent) waitingFor(gen int) bool {
	return len(s.levels) > 0 && s.levels[len(s.levels)-1].gen == gen && s.opening
}

// back goes up one level. At the top it lights the left edge.
func (s searchContent) back() searchContent {
	if len(s.levels) == 0 {
		s.listView = s.bump(bumpLeft)
		return s
	}
	return s.leave()
}

// leave goes up one level, as it was left, with no light.
func (s searchContent) leave() searchContent {
	if len(s.levels) == 0 {
		return s
	}
	saved := s.levels[len(s.levels)-1].saved
	saved.rows, saved.width = s.rows, s.width // the box may have been resized meanwhile
	s.listView, s.levels = saved, s.levels[:len(s.levels)-1]
	s.opening, s.levelProblem = false, ""
	return s
}

// leaveAll goes back to the results.
func (s searchContent) leaveAll() searchContent {
	for len(s.levels) > 0 {
		s = s.leave()
	}
	return s
}

// ClaimsEsc keeps esc from closing the modal while it has a filter to clear
// or a level to go back from.
func (s searchContent) ClaimsEsc() bool {
	return s.listView.ClaimsEsc() || len(s.levels) > 0
}

// onBackRow reports the cursor on the row that goes back.
func (s searchContent) onBackRow() bool {
	return s.cursor < len(s.items) && s.items[s.cursor].Type == rowBack
}

// artistRows is an artist's albums for the list: the back row, then each
// group that has albums under its heading. An album is a result, so the keys
// that act on a search result act on it.
func artistRows(artist sonoscloud.Artist) []sonoscloud.Result {
	rows := []sonoscloud.Result{backRow}
	for _, group := range artistGroups {
		var albums []sonoscloud.Result
		for _, entry := range artist.Albums {
			if slices.Contains(group.kinds, entry.Kind) {
				albums = append(albums, sonoscloud.Result{
					ID: entry.ID, Type: sonoscloud.TypeAlbum, Name: entry.Name,
					Artists: []sonoscloud.Ref{{Name: entry.Artist}}, Summary: entry.Summary,
					ImageURL: entry.ImageURL, Playable: entry.Playable,
				})
			}
		}
		if len(albums) > 0 {
			rows = append(rows, sonoscloud.Result{Type: rowHeading, Name: group.name})
			rows = append(rows, albums...)
		}
	}
	return rows
}

// shownName is a row's name as the list draws it. On an artist's page an
// album's " - Single" or " - EP" is left out: its group already says it.
func (s searchContent) shownName(r sonoscloud.Result) string {
	if len(s.levels) == 0 || r.Type != sonoscloud.TypeAlbum {
		return r.Name
	}
	return strings.TrimSuffix(strings.TrimSuffix(r.Name, " - Single"), " - EP")
}

// shownDetail is a row's second column: on an artist's page an album's year,
// which is all that differs from one album to the next, and otherwise the
// search's own detail.
func (s searchContent) shownDetail(r sonoscloud.Result) string {
	if len(s.levels) == 0 {
		return resultDetail(r)
	}
	_, year, _ := r.GenreYear()
	return year
}

// levelNote is the body of a level with no rows to draw yet, and empty
// otherwise.
func (s searchContent) levelNote() string {
	switch {
	case s.opening:
		return fmt.Sprintf(searchOpeningFmt, s.levels[len(s.levels)-1].name)
	default:
		return s.levelProblem
	}
}

// drawHeading draws a group heading as a rule across the row, with the
// group's name set into it, so it reads as a break between groups and not as
// one more entry.
func (s searchContent) drawHeading(name string, widths [2]int, arrow string) string {
	full := listMarkWidth + widths[0] + listGap + widths[1]
	lead := "── "
	tail := max(full-lipgloss.Width(lead)-lipgloss.Width(name)-1, 0)
	line := groupRuleStyle().Render(lead) + searchKindStyle().UnsetUnderline().Render(name) +
		groupRuleStyle().Render(" "+strings.Repeat("─", tail))
	if !s.overflows() {
		return line
	}
	return line + strings.Repeat(" ", listGap) + listArrowStyle(s.openedAt).Render(arrow)
}

// groupRuleStyle is stronger than the rule under the heading, which only
// frames the list.
func groupRuleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.LightDark(lipgloss.Color("#A3A3A3"), lipgloss.Color("#7A7A7A")))
}

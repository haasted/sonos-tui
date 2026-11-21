// Browsing from the search: stepping into an artist to see its albums, or an
// album to see its tracks, and back out to the list it was opened from as it
// was. search.go holds the modal itself.
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

// levelFilter matches an album or a track by its name. The back row matches
// every filter and the headings none, so a filter flattens the groups.
var levelFilter = listFilter[sonoscloud.Result]{
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

	// albumOpenedMsg is an album read from the cloud, for the level opened
	// with gen.
	albumOpenedMsg struct {
		gen     int
		localID string
		album   sonoscloud.Album
		err     error
	}
)

// albumStart is an album to play from one of its tracks.
type albumStart struct {
	album    sonoscloud.Result
	position int // the track's place on the album, counted from 1
}

func isHeading(r sonoscloud.Result) bool { return r.Type == rowHeading }

// open steps into the artist or album under the cursor and asks for what it
// holds. On anything else it lights the right edge, as pressing against the
// end of a list does.
func (s searchContent) open() (searchContent, tea.Cmd) {
	if s.cursor >= len(s.items) || !openable(s.items[s.cursor].Type) {
		s.listView = s.bump(bumpRight)
		return s, nil
	}
	row := s.items[s.cursor]
	gen := nextGen()
	s.levels = append(slices.Clone(s.levels), browseLevel[sonoscloud.Result]{name: row.Name, item: row, saved: s.listView, gen: gen})
	s.items, s.all, s.filter, s.filtering = []sonoscloud.Result{backRow}, nil, "", false
	s.cursor, s.offset, s.skip = 0, 0, isHeading
	s.opening, s.levelProblem, s.levelTotal = true, "", 0
	return s, func() tea.Msg { return searchOpenMsg{result: row, gen: gen} }
}

// openable reports the types that hold something to show.
func openable(t sonoscloud.ResultType) bool {
	return t == sonoscloud.TypeArtist || t == sonoscloud.TypeAlbum
}

// openedAlbum shows an album's tracks, if the level on top is still the one
// they were asked for. The album's own artist line is kept, so a track's
// artist shows only where it differs.
func (s searchContent) openedAlbum(gen int, album sonoscloud.Album) searchContent {
	if !s.waitingFor(gen) {
		return s
	}
	top := &s.levels[len(s.levels)-1]
	top.item.Artists = []sonoscloud.Ref{{Name: album.Artist}}
	s.opening, s.levelTotal = false, album.Total
	s.items, s.all = albumRows(album), nil
	s.cursor, s.offset = min(1, len(s.items)-1), 0
	s.listView = s.follow()
	return s
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
	s.opening, s.levelProblem, s.levelTotal = false, "", 0
	return s
}

// leaveAll goes back to the results.
func (s searchContent) leaveAll() searchContent {
	for len(s.levels) > 0 {
		s = s.leave()
	}
	return s
}

// ClaimsEsc keeps esc from closing the modal while it has a level to go back
// from, a query in the open box, or a filter to clear.
func (s searchContent) ClaimsEsc() bool {
	switch {
	case len(s.levels) > 0:
		return true
	case s.filtering:
		return s.query != ""
	}
	return s.listView.ClaimsEsc()
}

// onBackRow reports the cursor on the row that goes back.
func (s searchContent) onBackRow() bool {
	return s.cursor < len(s.items) && s.items[s.cursor].Type == rowBack
}

// artistRows is an artist's albums for the list: the back row, then each
// group that has albums under its heading, newest first. An album is a
// result, so the keys that act on a search result act on it.
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
		slices.SortStableFunc(albums, newestFirst)
		if len(albums) > 0 {
			rows = append(rows, sonoscloud.Result{Type: rowHeading, Name: group.name})
			rows = append(rows, albums...)
		}
	}
	return rows
}

// albumRows is an album's tracks for the list, after the back row. A track is
// a result, so the keys that act on a search result act on it.
func albumRows(album sonoscloud.Album) []sonoscloud.Result {
	rows := []sonoscloud.Result{backRow}
	for _, track := range album.Tracks {
		rows = append(rows, sonoscloud.Result{
			ID: track.ID, Type: sonoscloud.TypeTrack, Name: track.Name, Artists: track.Artists,
			Album: &sonoscloud.Ref{ID: album.ID, Name: album.Name}, Playable: track.Playable,
			Ordinal: track.Ordinal, Duration: track.Duration, MimeType: track.MimeType,
		})
	}
	return rows
}

// openedType is the type of what the level on top opened, or empty at the
// results.
func (s searchContent) openedType() sonoscloud.ResultType {
	if len(s.levels) == 0 {
		return ""
	}
	return s.levels[len(s.levels)-1].item.Type
}

// trackPosition is a track's place on the album opened, counted from 1, found
// in the whole list, so a filter does not change it. 0 when it is not there.
func (s searchContent) trackPosition(r sonoscloud.Result) int {
	for i, row := range s.everything() {
		if row.Type == sonoscloud.TypeTrack && levelFilter.same(row, r) {
			return i // the back row is 0, so the first track is 1
		}
	}
	return 0
}

// newestFirst orders albums by year, the newest first and those with no year
// last. Albums of one year compare equal, so a stable sort keeps the API's
// order among them: the API gives the year, not the date.
func newestFirst(a, b sonoscloud.Result) int {
	_, yearA, okA := a.GenreYear()
	_, yearB, okB := b.GenreYear()
	switch {
	case okA != okB && okA:
		return -1
	case okA != okB:
		return 1
	}
	return strings.Compare(yearB, yearA) // four digits each, so text order is number order
}

// shownName is a row's name as the list draws it. On an artist's page an
// album's " - Single" or " - EP" is left out: its group already says it. On
// an album's page a track has its number in front.
func (s searchContent) shownName(r sonoscloud.Result) string {
	switch {
	case s.openedType() == sonoscloud.TypeArtist && r.Type == sonoscloud.TypeAlbum:
		return strings.TrimSuffix(strings.TrimSuffix(r.Name, " - Single"), " - EP")
	case s.openedType() == sonoscloud.TypeAlbum && r.Type == sonoscloud.TypeTrack:
		return fmt.Sprintf("%2d  %s", r.Ordinal, r.Name)
	}
	return r.Name
}

// shownDetail is a row's second column: on an artist's page an album's year,
// which is all that differs from one album to the next; on an album's page a
// track's artist, only where it is not the album's; and otherwise the
// search's own detail.
func (s searchContent) shownDetail(r sonoscloud.Result) string {
	switch s.openedType() {
	case sonoscloud.TypeArtist:
		_, year, _ := r.GenreYear()
		return year
	case sonoscloud.TypeAlbum:
		if artist := artistNames(r); artist != artistNames(s.levels[len(s.levels)-1].item) {
			return artist
		}
		return ""
	}
	return resultDetail(r)
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

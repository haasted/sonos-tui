package main

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
	"github.com/haasted/sonos-tui/internal/ui/theme"
)

// searchTypes is the order of the type row, and of tab and → through it.
// Playlists are left out: the search has never sent one.
var searchTypes = []sonoscloud.ResultType{
	sonoscloud.TypeArtist, sonoscloud.TypeAlbum, sonoscloud.TypeTrack, sonoscloud.TypeProgram,
}

// searchTypeNames is what the type row calls each type.
var searchTypeNames = map[sonoscloud.ResultType]string{
	sonoscloud.TypeArtist:  "Artists",
	sonoscloud.TypeAlbum:   "Albums",
	sonoscloud.TypeTrack:   "Tracks",
	sonoscloud.TypeProgram: "Stations",
}

const (
	searchTitle      = "Search Apple Music"
	searchTypeGap    = "   "
	searchBoxRows    = 1 // the search box, above the list's own heading
	searchAskText    = "Type what to look for, then press enter."
	searchRunningFmt = "Searching for “%s”…"
	searchNothingFmt = "Nothing found for “%s”."
)

// searchAskMsg asks the application to run a search for query.
type searchAskMsg struct{ query string }

// searchContent is the search modal: a box for the query, a row of the result
// types, and the results of the type in view. listView's filtering flag says
// the box has the keys; the list's own filter is not used.
type searchContent struct {
	listView[sonoscloud.Result]

	query   string                                        // what the box holds
	asked   string                                        // the query the results are for
	byType  map[sonoscloud.ResultType][]sonoscloud.Result // the results, by type
	kind    sonoscloud.ResultType                         // the type in view; empty with no results
	running bool                                          // a search is on its way
	problem string                                        // why there are no results, said in the body
}

var _ listContent = searchContent{}

func (searchContent) Init() tea.Cmd { return nil }

// Update types into the box while it has the keys, and otherwise moves
// through the results, switches type, and goes back to the box with /.
func (s searchContent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		var delta int
		if s.listView, delta = s.wheelStep(msg); delta != 0 {
			s.filtering = false
			s.listView = s.wheelMove(delta)
		}
		return s, nil
	case listClickMsg:
		msg.line -= searchBoxRows
		s.listView, _, _, _ = s.click(msg.line, msg.at)
		return s, nil
	case tea.KeyMsg:
		if s.filtering {
			return s.boxKey(msg)
		}
		return s.listKey(msg)
	}
	return s, nil
}

// boxKey is one key while the box has the keys. Every key that types text is
// text here, q and ? too.
func (s searchContent) boxKey(msg tea.KeyMsg) (searchContent, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		s.filtering = false
	case "enter":
		query := strings.TrimSpace(s.query)
		if query == "" {
			return s, nil
		}
		s.filtering = false
		return s, func() tea.Msg { return searchAskMsg{query: query} }
	case "down":
		s.filtering = len(s.items) == 0
	case "backspace":
		if runes := []rune(s.query); len(runes) > 0 {
			s.query = string(runes[:len(runes)-1])
		}
	case "space":
		s.query += " "
	default:
		if text := msg.Key().Text; text != "" {
			s.query += text
		}
	}
	return s, nil
}

// listKey is one key while the list has the keys.
func (s searchContent) listKey(msg tea.KeyMsg) (searchContent, tea.Cmd) {
	if list, moved := s.navigate(msg.String()); moved {
		s.listView = list
		return s, nil
	}
	switch msg.String() {
	case "tab", "right":
		s = s.switchKind(1)
	case "shift+tab", "left":
		s = s.switchKind(-1)
	case "/":
		s.filtering = true
	}
	return s, nil
}

// searching shows that a search for query is on its way. The results of the
// last one go, so they are not taken for the new one's.
func (s searchContent) searching(query string) searchContent {
	s.running, s.problem, s.asked = true, "", query
	s.byType, s.kind = nil, ""
	return s.show()
}

// failed says why a search has no results.
func (s searchContent) failed(problem string) searchContent {
	s.running, s.problem = false, problem
	s.byType, s.kind = nil, ""
	return s.show()
}

// found shows a search's results, on the first type that has any. A type the
// row does not name is left out.
func (s searchContent) found(query string, results sonoscloud.SearchResults) searchContent {
	s.running, s.problem, s.asked = false, "", query
	s.byType, s.kind = map[sonoscloud.ResultType][]sonoscloud.Result{}, ""
	for _, group := range results.Groups {
		for _, r := range group.Results {
			if _, named := searchTypeNames[r.Type]; named {
				s.byType[r.Type] = append(s.byType[r.Type], r)
			}
		}
	}
	for _, kind := range searchTypes {
		if len(s.byType[kind]) > 0 {
			s.kind = kind
			break
		}
	}
	return s.show()
}

// switchKind moves to the next type with results in the direction given,
// passing over the empty ones. At either end it lights that side of the box.
func (s searchContent) switchKind(dir int) searchContent {
	at := -1
	for i, kind := range searchTypes {
		if kind == s.kind {
			at = i
		}
	}
	for i := at + dir; at >= 0 && i >= 0 && i < len(searchTypes); i += dir {
		if len(s.byType[searchTypes[i]]) > 0 {
			s.kind = searchTypes[i]
			return s.show()
		}
	}
	if dir < 0 {
		s.listView = s.bump(bumpLeft)
	} else {
		s.listView = s.bump(bumpRight)
	}
	return s
}

// show puts the results of the type in view on the list, with the cursor at
// the top.
func (s searchContent) show() searchContent {
	s.items, s.cursor, s.offset = s.byType[s.kind], 0, 0
	return s
}

// animating reports the arrows breathing while the list overflows, and a
// bumped edge cooling.
func (s searchContent) animating() bool { return s.overflows() || s.bumping() }

// hintReady holds the hint back while there are no rows for its keys.
func (s searchContent) hintReady() bool { return s.note() == "" }

func (s searchContent) resized(rows, width int) tea.Model {
	s.rows, s.width = rows-searchBoxRows, width
	return s
}

func (s searchContent) View() tea.View {
	first, second := s.columns(func(r sonoscloud.Result) string { return r.Name }, resultDetail)
	list := s.render(s.typeRow(), s.note(), func(index int, r sonoscloud.Result, arrow string) string {
		return s.drawRow(index, r.Name, resultDetail(r), [2]int{first, second}, queueRowStyle(), arrow, s.filtering)
	})
	return tea.NewView(s.boxLine() + "\n" + list)
}

// boxLine is the line above the list: the modal's name and the query. While
// the box has the keys, / is its prompt, as in the filter box.
func (s searchContent) boxLine() string {
	parts := []headingPart{{text: searchTitle, style: searchHeadingStyle()}}
	switch {
	case s.filtering:
		parts = append(parts,
			headingPart{text: headingSeparator, style: searchHeadingStyle()},
			headingPart{text: "/", style: queueFilterPromptStyle()},
			headingPart{text: s.query, style: searchHeadingStyle()})
	case s.query != "":
		parts = append(parts, headingPart{text: headingSeparator + s.query, style: searchHeadingStyle(), shrink: true})
	}
	var b strings.Builder
	for _, part := range fitHeading(parts, s.width) {
		b.WriteString(part.style.Render(part.text))
	}
	return b.String()
}

// typeRow names every type with how many results it has, the one in view
// lit and the empty ones subdued, so the row stays put from one search to the
// next. Too narrow for all of them, it shows the one in view between ‹ and ›.
func (s searchContent) typeRow() []headingPart {
	var parts []headingPart
	for i, kind := range searchTypes {
		if i > 0 {
			parts = append(parts, headingPart{text: searchTypeGap})
		}
		parts = append(parts, headingPart{text: s.typeLabel(kind), style: s.typeStyle(kind)})
	}
	if headingWidth(parts) <= s.width || s.kind == "" {
		return fitHeading(parts, s.width)
	}

	at := 0
	for i, kind := range searchTypes {
		if kind == s.kind {
			at = i
		}
	}
	return fitHeading([]headingPart{
		{text: "‹", style: favouritesArrowStyle(at > 0)},
		{text: " " + s.typeLabel(s.kind) + " ", style: s.typeStyle(s.kind)},
		{text: "›", style: favouritesArrowStyle(at < len(searchTypes)-1)},
	}, s.width)
}

// typeLabel is a type's name, and its count once there are results.
func (s searchContent) typeLabel(kind sonoscloud.ResultType) string {
	if s.byType == nil {
		return searchTypeNames[kind]
	}
	return fmt.Sprintf("%s %d", searchTypeNames[kind], len(s.byType[kind]))
}

func (s searchContent) typeStyle(kind sonoscloud.ResultType) lipgloss.Style {
	switch {
	case kind == s.kind:
		return searchKindStyle()
	case len(s.byType[kind]) == 0:
		return lipgloss.NewStyle().Foreground(themedArrowRest())
	default:
		return searchHeadingStyle().Bold(false)
	}
}

// note is the whole body when there are no rows to draw, and empty otherwise.
func (s searchContent) note() string {
	switch {
	case s.running:
		return fmt.Sprintf(searchRunningFmt, s.asked)
	case s.problem != "":
		return s.problem
	case s.byType == nil:
		return searchAskText
	case s.kind == "":
		return fmt.Sprintf(searchNothingFmt, s.asked)
	default:
		return ""
	}
}

// resultDetail is a result's second column: a track's artist and album, an
// album's artist and year, a station's summary.
func resultDetail(r sonoscloud.Result) string {
	var artists []string
	for _, a := range r.Artists {
		artists = append(artists, a.Name)
	}
	artist := strings.Join(artists, ", ")

	switch r.Type {
	case sonoscloud.TypeTrack:
		if r.Album != nil && r.Album.Name != "" {
			return joinDetail(artist, r.Album.Name)
		}
		return artist
	case sonoscloud.TypeAlbum:
		_, year, _ := r.GenreYear()
		return joinDetail(artist, year)
	case sonoscloud.TypeProgram:
		return r.Summary
	default:
		return ""
	}
}

// joinDetail joins the parts of a second column that are there.
func joinDetail(parts ...string) string {
	var there []string
	for _, part := range parts {
		if part != "" {
			there = append(there, part)
		}
	}
	return strings.Join(there, headingSeparator)
}

// searchBorderColor is teal, so the search is easy to tell from the queue's
// amber and the favourites' rose.
func searchBorderColor() color.Color {
	return theme.LightDark(lipgloss.Color("#0F766E"), lipgloss.Color("#2DD4BF"))
}

func searchHeadingStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(searchBorderColor())
}

// searchKindStyle draws the type in view in the border's teal taken nearly to
// white, or to its deepest on a light ground, as the favourites draw theirs.
func searchKindStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Underline(true).
		Foreground(theme.LightDark(lipgloss.Color("#134E4A"), lipgloss.Color("#CCFBF1")))
}

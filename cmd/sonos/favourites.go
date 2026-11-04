package main

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/store"
	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

// favouriteKinds is the order ← and → walk the media types in.
var favouriteKinds = []store.FavouriteKind{
	store.FavouriteTrack, store.FavouriteAlbum, store.FavouritePlaylist, store.FavouriteStation,
}

// favouriteKindNames is what the heading calls each media type.
var favouriteKindNames = map[store.FavouriteKind]string{
	store.FavouriteTrack:    "Tracks",
	store.FavouriteAlbum:    "Albums",
	store.FavouritePlaylist: "Playlists",
	store.FavouriteStation:  "Stations",
}

// favouriteAction is what a key asks to do with the favourite under the cursor.
type favouriteAction int

const (
	favouritePlay       favouriteAction = iota // replace the queue and play; a station just plays
	favouritePlayNext                          // put it after the playing track
	favouriteAddToQueue                        // put it at the end of the queue
)

type (
	// favouritesLoadedMsg carries the favourites read for the modal, and the
	// media type it showed last.
	favouritesLoadedMsg struct {
		favourites []store.Favourite
		kind       store.FavouriteKind
		err        error
	}

	// favouriteKindChosenMsg says the reader switched media type, so the
	// choice can be remembered for the next time the modal opens.
	favouriteKindChosenMsg struct{ kind store.FavouriteKind }

	// favouriteActionMsg asks for a favourite to start on the speaker in view.
	favouriteActionMsg struct {
		favourite store.Favourite
		action    favouriteAction
	}
)

// favouritesContent lists the favourites of one media type inside a modal.
// all holds every favourite; the list shows those of kind.
type favouritesContent struct {
	listView[store.Favourite]

	all     []store.Favourite
	kind    store.FavouriteKind // the media type on screen; empty with no favourites
	loading bool
	err     error
}

var _ listContent = favouritesContent{}

func (favouritesContent) Init() tea.Cmd { return nil }

// Update moves through the list, switches media type with ← and →, and asks
// for the favourite under the cursor to be played.
func (f favouritesContent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		var delta int
		if f.listView, delta = f.wheelStep(wheel); delta != 0 {
			f.listView = f.wheelMove(delta)
		}
		return f, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, nil
	}

	if list, moved := f.navigate(key.String()); moved {
		f.listView = list
		return f, nil
	}

	switch key.String() {
	case "left":
		return f.switchKind(-1)
	case "right":
		return f.switchKind(1)
	case "enter":
		return f, f.ask(favouritePlay)
	case "n":
		return f, f.ask(favouritePlayNext)
	case "N":
		return f, f.ask(favouriteAddToQueue)
	}
	return f, nil
}

// loaded fills the modal with the favourites read, on the media type shown
// last, or on the first that has favourites when that one has none now.
func (f favouritesContent) loaded(msg favouritesLoadedMsg) favouritesContent {
	f.loading, f.err, f.all = false, msg.err, msg.favourites
	kinds := f.kinds()
	switch {
	case len(kinds) == 0:
		f.kind = ""
	case f.has(msg.kind):
		f.kind = msg.kind
	default:
		f.kind = kinds[0]
	}
	return f.show()
}

// switchKind moves to the next media type with favourites, in the direction
// given. At either end it lights that side of the box, as the end of a list
// lights its top or bottom.
func (f favouritesContent) switchKind(dir int) (favouritesContent, tea.Cmd) {
	kinds := f.kinds()
	at := -1
	for i, kind := range kinds {
		if kind == f.kind {
			at = i
		}
	}
	next := at + dir
	if at < 0 || next < 0 || next >= len(kinds) {
		if dir < 0 {
			f.listView = f.bump(bumpLeft)
		} else {
			f.listView = f.bump(bumpRight)
		}
		return f, nil
	}

	f.kind = kinds[next]
	chosen := f.kind
	return f.show(), func() tea.Msg { return favouriteKindChosenMsg{kind: chosen} }
}

// show puts the favourites of the current media type on the list, with the
// cursor at the top.
func (f favouritesContent) show() favouritesContent {
	var items []store.Favourite
	for _, fav := range f.all {
		if fav.Kind == f.kind {
			items = append(items, fav)
		}
	}
	f.items, f.cursor, f.offset = items, 0, 0
	return f
}

// kinds are the media types that have favourites, in the order ← and → walk.
func (f favouritesContent) kinds() []store.FavouriteKind {
	var kinds []store.FavouriteKind
	for _, kind := range favouriteKinds {
		if f.has(kind) {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// neighbours reports whether a media type with favourites lies before and
// after the one on screen.
func (f favouritesContent) neighbours() (before, after bool) {
	kinds := f.kinds()
	for i, kind := range kinds {
		if kind == f.kind {
			return i > 0, i < len(kinds)-1
		}
	}
	return false, false
}

func (f favouritesContent) has(kind store.FavouriteKind) bool {
	for _, fav := range f.all {
		if fav.Kind == kind {
			return true
		}
	}
	return false
}

// ask is the command a key sends for the favourite under the cursor, or nil
// when the list has none.
func (f favouritesContent) ask(action favouriteAction) tea.Cmd {
	if f.cursor >= len(f.items) {
		return nil
	}
	fav := f.items[f.cursor]
	return func() tea.Msg { return favouriteActionMsg{favourite: fav, action: action} }
}

// animating reports the arrows breathing while the list overflows, and a
// bumped edge cooling.
func (f favouritesContent) animating() bool { return f.overflows() || f.bumping() }

// hintReady holds the hint back while there are no rows for its keys.
func (f favouritesContent) hintReady() bool { return f.note() == "" }

func (f favouritesContent) resized(rows, width int) tea.Model {
	f.rows, f.width = rows, width
	return f
}

func (f favouritesContent) View() tea.View {
	title, artist := f.columns(
		func(fav store.Favourite) string { return fav.Title },
		func(fav store.Favourite) string { return fav.Artist })

	return tea.NewView(f.render(f.headingParts(), f.note(), func(index int, fav store.Favourite, arrow string) string {
		return f.drawRow(index, fav.Title, fav.Artist, [2]int{title, artist}, queueRowStyle(), arrow, false)
	}))
}

// heading is the heading as plain text.
func (f favouritesContent) heading() string {
	var text string
	for _, part := range f.headingParts() {
		text += part.text
	}
	return text
}

// headingParts names the modal, the media type in view, and how much of it
// is showing. ‹ and › mark the sides ← and → can still go to, each on its own.
func (f favouritesContent) headingParts() []headingPart {
	parts := []headingPart{{text: "Favourites", style: favouritesHeadingStyle()}}
	if f.kind != "" {
		left, name, right := f.kindLabel()
		parts = append(parts,
			headingPart{text: headingSeparator, style: favouritesHeadingStyle()},
			headingPart{text: left, style: favouritesArrowStyle()},
			headingPart{text: name, style: favouritesHeadingStyle()},
			headingPart{text: right, style: favouritesArrowStyle()})
	}
	switch {
	case f.note() != "":
	case f.overflows():
		parts = append(parts, headingPart{text: headingSeparator + f.rangeLabel(), style: favouritesHeadingStyle()})
	default:
		parts = append(parts, headingPart{text: fmt.Sprintf("%s%d", headingSeparator, len(f.items)), style: favouritesHeadingStyle()})
	}
	return fitHeading(parts, f.width)
}

// kindLabel is the media type and the arrows either side, always the same
// width so the heading stays still as ← and → switch: an arrow that does not
// apply leaves its space, and the name is centred in the longest one's width.
func (f favouritesContent) kindLabel() (left, name, right string) {
	before, after := f.neighbours()
	left, right = "  ", "  "
	if before {
		left = "‹ "
	}
	if after {
		right = " ›"
	}

	name = favouriteKindNames[f.kind]
	spare := widestKindName() - lipgloss.Width(name)
	name = strings.Repeat(" ", spare/2) + name + strings.Repeat(" ", spare-spare/2)
	return left, name, right
}

// widestKindName is the display width of the longest media type name.
func widestKindName() int {
	widest := 0
	for _, name := range favouriteKindNames {
		widest = max(widest, lipgloss.Width(name))
	}
	return widest
}

// note is the whole body when there are no rows to draw, and empty otherwise.
func (f favouritesContent) note() string {
	switch {
	case f.loading:
		return "Reading the favourites…"
	case f.err != nil:
		return "Could not read the favourites: " + f.err.Error()
	case len(f.all) == 0:
		return "No favourites yet."
	default:
		return ""
	}
}

// favouritesBorderColor is rose, so the favourites are easy to tell from the
// queue's amber at a glance.
func favouritesBorderColor() color.Color {
	return theme.LightDark(lipgloss.Color("#BE185D"), lipgloss.Color("#F472B6"))
}

func favouritesHeadingStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(favouritesBorderColor())
}

// favouritesArrowStyle sets ‹ and › apart from the name between them: white
// on a dark ground, near-black on a light one, as the queue's scroll arrows.
func favouritesArrowStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(themedArrowLit())
}

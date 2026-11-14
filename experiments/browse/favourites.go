package main

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
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

// favouritesFilter matches a favourite by title or artist, as the queue does
// a track, and keeps the cursor on a favourite by its row id.
var favouritesFilter = listFilter[store.Favourite]{
	match: func(fav store.Favourite, text string) bool {
		return fav.Kind == backKind || anyContains(text, fav.Title, fav.Artist)
	},
	same: func(a, b store.Favourite) bool { return a.ID == b.ID },
}

// favouritesCountFloor is the longest list the heading leaves uncounted:
// up to this many rows, the reader sees how many there are at a glance.
const favouritesCountFloor = 7

// favouriteRemoveFade is how long a removed favourite stays on the list,
// dimmed as a track leaving the queue is, before it goes. Short: the delete
// itself is done at the press, so nothing waits on it.
const favouriteRemoveFade = 400 * time.Millisecond

// favouriteAction is what a key asks to do with the favourite under the cursor.
type favouriteAction int

const (
	favouritePlay         favouriteAction = iota // replace the queue and play; a station just plays
	favouritePlayNext                            // put it after the playing track
	favouriteAddToQueue                          // put it at the end of the queue
	favouritePlayShuffled                        // replace the queue and play it shuffled; an album or playlist only
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

	// favouriteMarkMsg asks for tracks to be stored as favourites. The first
	// goes to the top of the list, the rest below it in order.
	favouriteMarkMsg struct{ tracks []sonos.TrackMetadata }

	// favouriteRemoveMsg asks for a favourite to be deleted. The modal fades
	// it out on its own.
	favouriteRemoveMsg struct{ favourite store.Favourite }

	// favouriteRestoreMsg asks for a removed favourite to be stored again,
	// with its id and added time, so it is back in its old place.
	favouriteRestoreMsg struct{ favourite store.Favourite }

	// favouriteArrivedMsg ends the fade of a favourite put back. It carries
	// the generation of the last press, so an earlier press's tick is ignored.
	favouriteArrivedMsg struct{ gen int }

	// favouriteFadedMsg ends the fade of the favourites removed. It carries
	// the generation of the last press, so an earlier press's tick is ignored.
	favouriteFadedMsg struct{ gen int }

	// favouriteActionMsg asks for a favourite to start on the speaker in view.
	favouriteActionMsg struct {
		favourite store.Favourite
		action    favouriteAction
	}
)

// favouritesContent lists the favourites of one media type inside a modal.
// everyKind holds every favourite; the list shows those of kind.
type favouritesContent struct {
	listView[store.Favourite]

	everyKind []store.Favourite
	kind      store.FavouriteKind // the media type on screen; empty with no favourites
	browsing  *browseLevel        // the favourite opened, or nil at the top
	going     []int64             // ids removed and still fading on the list
	goingGen  int                 // the generation of the last removal's fade
	removed   []store.Favourite   // removed while the modal is open, the last on top
	coming    int64               // the id put back and still fading in; 0 for none
	comingGen int                 // the generation of the last put back's fade
	loading   bool
	err       error
}

var _ listContent = favouritesContent{}

func (favouritesContent) Init() tea.Cmd { return nil }

// Update moves through the list, switches media type with ← and →, filters
// with /, and asks for the favourite under the cursor to be played.
func (f favouritesContent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		var delta int
		if f.listView, delta = f.wheelStep(wheel); delta == 0 {
			return f, nil
		}
		// While the box has the keys, the wheel steps out of it as down does.
		if f.filtering {
			if delta > 0 {
				f.listView = f.filterKey(favouritesFilter, "down")
			}
			return f, nil
		}
		f.listView = f.wheelMove(delta)
		return f, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, nil
	}

	// The filter box has the keys while it is open, as in the queue, except
	// tab and shift+tab: they type nothing, so they switch type with it open.
	if f.filtering && key.String() != "tab" && key.String() != "shift+tab" {
		f.listView = f.filterKey(favouritesFilter, key.String())
		return f, nil
	}

	if list, moved := f.navigate(key.String()); moved {
		f.listView = list
		return f, nil
	}

	switch key.String() {
	// Inside an opened favourite, tab leaves it first, then switches type.
	case "tab":
		return f.leave().switchKind(1)
	case "shift+tab":
		return f.leave().switchKind(-1)
	case "right", "l":
		return f.open(), nil
	case "left", "h":
		return f.back(), nil
	case "enter":
		if f.onBackRow() {
			return f.back(), nil
		}
		return f, f.ask(favouritePlay)
	case "shift+enter":
		return f, f.ask(favouritePlayShuffled)
	case "n":
		return f, f.ask(favouritePlayNext)
	case "N":
		return f, f.ask(favouriteAddToQueue)
	case "x", "delete":
		if f.browsing == nil {
			return f.remove()
		}
	case "X":
		if f.browsing == nil {
			return f.restore()
		}
	case "/":
		f.listView = f.openFilter(favouritesFilter)
	case "esc":
		if f.listView.ClaimsEsc() {
			f.listView = f.closeFilter(favouritesFilter)
		} else {
			return f.back(), nil
		}
	}
	return f, nil
}

// backKind marks the row at the top of an opened favourite that goes back up.
// It is not a real kind: no favourite is stored with it.
const backKind store.FavouriteKind = "back"

// backRow is that row. It matches every filter, so it is always there.
var backRow = store.Favourite{ID: -1, Kind: backKind, Title: "← back"}

// browseLevel is a favourite opened to show what it holds. saved is the list
// as it was before, cursor and filter included, so going back finds it again.
type browseLevel struct {
	parent store.Favourite
	saved  listView[store.Favourite]
}

// openable reports the kinds that hold something to show.
func openable(kind store.FavouriteKind) bool {
	return kind == store.FavouriteAlbum || kind == store.FavouritePlaylist
}

// open shows what the favourite under the cursor holds: an album's or a
// playlist's tracks. Anything else lights the right edge, as pressing
// against the end of a list does.
func (f favouritesContent) open() favouritesContent {
	if f.browsing != nil || f.cursor >= len(f.items) || !openable(f.items[f.cursor].Kind) {
		f.listView = f.bump(bumpRight)
		return f
	}
	parent := f.items[f.cursor]
	f.browsing = &browseLevel{parent: parent, saved: f.listView}
	// The back row comes first; the cursor starts on the first track below it.
	f.items = append([]store.Favourite{backRow}, childrenOf(parent)...)
	f.all, f.filter, f.filtering = nil, "", false
	f.cursor, f.offset = min(1, len(f.items)-1), 0
	f.listView = f.follow()
	return f
}

// back returns from an opened favourite to the list it was opened from. At
// the top it lights the left edge.
func (f favouritesContent) back() favouritesContent {
	if f.browsing == nil {
		f.listView = f.bump(bumpLeft)
		return f
	}
	return f.leave()
}

// leave returns from an opened favourite, if one is open, with no light.
func (f favouritesContent) leave() favouritesContent {
	if f.browsing == nil {
		return f
	}
	saved := f.browsing.saved
	saved.rows, saved.width = f.rows, f.width // the box may have been resized meanwhile
	f.listView, f.browsing = saved, nil
	return f
}

// trackCountLabel counts an opened favourite's tracks, leaving out the back
// row, which sits at index 0: track n is at index n.
func (f favouritesContent) trackCountLabel() string {
	tracks := len(f.items) - 1
	switch {
	case f.filter != "":
		return fmt.Sprintf("%d of %d", tracks, len(f.everything())-1)
	case f.overflows():
		first, last := f.window()
		return fmt.Sprintf("%d-%d of %d", max(first, 1), last-1, tracks)
	case tracks > favouritesCountFloor:
		return fmt.Sprintf("%d", tracks)
	}
	return ""
}

// onBackRow reports the cursor on the row that goes back up.
func (f favouritesContent) onBackRow() bool {
	return f.cursor < len(f.items) && f.items[f.cursor].Kind == backKind
}

// ClaimsEsc keeps esc from closing the modal while it can go back a level, as
// well as while there is a filter to clear.
func (f favouritesContent) ClaimsEsc() bool {
	return f.listView.ClaimsEsc() || f.browsing != nil
}

// loaded fills the modal with the favourites read, on the media type shown
// last, or on the first that has favourites when that one has none now.
func (f favouritesContent) loaded(msg favouritesLoadedMsg) favouritesContent {
	f.loading, f.err, f.everyKind = false, msg.err, msg.favourites
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
	// A fade still running ends now: the rows would be gone on the way back.
	if len(f.going) > 0 {
		var settled tea.Cmd
		if f, settled = f.settle(); settled != nil {
			return f, settled // the type emptied, and settling moved off it already
		}
	}
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

// remove deletes the favourite under the cursor at once, as x does in the
// queue. The row fades on the list for favouriteRemoveFade, and the cursor
// moves off it straight away, so a second x takes the next favourite.
func (f favouritesContent) remove() (favouritesContent, tea.Cmd) {
	if f.cursor >= len(f.items) || f.isGoing(f.items[f.cursor]) {
		return f, nil
	}
	gone := f.items[f.cursor]
	f.going = append(slices.Clone(f.going), gone.ID)
	f.removed = append(slices.Clone(f.removed), gone)
	f.cursor = f.restingRow(f.cursor)
	f.listView = f.follow()

	f.goingGen = nextGen()
	gen := f.goingGen
	return f, tea.Batch(
		func() tea.Msg { return favouriteRemoveMsg{favourite: gone} },
		tea.Tick(favouriteRemoveFade, func(time.Time) tea.Msg { return favouriteFadedMsg{gen: gen} }))
}

// restore puts back the favourite removed last, on its own media type and in
// its old place, with the cursor on it. It fades in for favouriteRemoveFade,
// the fade of a removal run backwards.
func (f favouritesContent) restore() (favouritesContent, tea.Cmd) {
	if len(f.removed) == 0 {
		return f, nil
	}
	f, _ = f.settle()
	back := f.removed[len(f.removed)-1]
	f.removed = f.removed[:len(f.removed)-1]

	at := slices.IndexFunc(f.everyKind, func(fav store.Favourite) bool { return listedBefore(back, fav) })
	if at < 0 {
		at = len(f.everyKind)
	}
	f.everyKind = slices.Insert(slices.Clone(f.everyKind), at, back)

	cmds := []tea.Cmd{func() tea.Msg { return favouriteRestoreMsg{favourite: back} }}
	if f.kind != back.Kind {
		f.kind = back.Kind
		chosen := f.kind
		cmds = append(cmds, func() tea.Msg { return favouriteKindChosenMsg{kind: chosen} })
	}
	f = f.show()
	// A filter that hides the favourite put back is closed, so it is seen.
	if !slices.ContainsFunc(f.items, func(fav store.Favourite) bool { return fav.ID == back.ID }) {
		f.listView = f.closeFilter(favouritesFilter)
	}
	f.cursor = slices.IndexFunc(f.items, func(fav store.Favourite) bool { return fav.ID == back.ID })
	f.listView = f.follow()

	f.coming, f.comingGen = back.ID, nextGen()
	gen := f.comingGen
	cmds = append(cmds, tea.Tick(favouriteRemoveFade, func(time.Time) tea.Msg { return favouriteArrivedMsg{gen: gen} }))
	return f, tea.Batch(cmds...)
}

// arrived ends the fade of the favourite put back, if no later press has
// started another.
func (f favouritesContent) arrived(gen int) favouritesContent {
	if gen == f.comingGen {
		f.coming, f.comingGen = 0, 0
	}
	return f
}

// listedBefore reports whether a comes before b in the list: newest first,
// and the higher id first at the same time, as the store lists them.
func listedBefore(a, b store.Favourite) bool {
	if !a.AddedAt.Equal(b.AddedAt) {
		return a.AddedAt.After(b.AddedAt)
	}
	return a.ID > b.ID
}

// faded takes the removed favourites off the list once the last press's fade
// is over.
func (f favouritesContent) faded(gen int) (favouritesContent, tea.Cmd) {
	if gen != f.goingGen {
		return f, nil
	}
	return f.settle()
}

// settle takes the removed favourites off the list. The cursor stays on the
// favourite it was on; the last of a media type moves to another type, and
// the new type is remembered as ← and → would.
func (f favouritesContent) settle() (favouritesContent, tea.Cmd) {
	if len(f.going) == 0 {
		return f, nil
	}
	var on int64 = -1
	if f.cursor < len(f.items) {
		on = f.items[f.cursor].ID
	}
	f.everyKind = slices.DeleteFunc(slices.Clone(f.everyKind), f.isGoing)
	f.going, f.goingGen = nil, 0

	if !f.has(f.kind) {
		f.kind = f.neighbourKind()
		if f.kind == "" {
			return f.show(), nil
		}
		chosen := f.kind
		return f.show(), func() tea.Msg { return favouriteKindChosenMsg{kind: chosen} }
	}

	f.items, f.all = f.ofKind(), nil
	f.listView = f.narrow(favouritesFilter)
	at := slices.IndexFunc(f.items, func(fav store.Favourite) bool { return fav.ID == on })
	if at < 0 {
		at = min(f.cursor, f.lastRow())
	}
	f.cursor = at
	f.listView = f.follow()
	return f, nil
}

func (f favouritesContent) isGoing(fav store.Favourite) bool {
	return slices.Contains(f.going, fav.ID)
}

// restingRow is the nearest row not fading out, looking down first and then
// up, as in the queue. With every row fading, the cursor stays where it is.
func (f favouritesContent) restingRow(from int) int {
	for i := from; i < len(f.items); i++ {
		if !f.isGoing(f.items[i]) {
			return i
		}
	}
	for i := min(from, len(f.items)) - 1; i >= 0; i-- {
		if !f.isGoing(f.items[i]) {
			return i
		}
	}
	return from
}

// neighbourKind is the media type to show once the current one has no
// favourites left: the next with favourites, or the one before when none
// follows. It is empty when no favourites are left at all.
func (f favouritesContent) neighbourKind() store.FavouriteKind {
	at := slices.Index(favouriteKinds, f.kind)
	for _, kind := range favouriteKinds[at+1:] {
		if f.has(kind) {
			return kind
		}
	}
	for i := at - 1; i >= 0; i-- {
		if f.has(favouriteKinds[i]) {
			return favouriteKinds[i]
		}
	}
	return ""
}

// show puts the favourites of the current media type on the list, with the
// cursor at the top.
func (f favouritesContent) show() favouritesContent {
	f.items, f.all, f.cursor, f.offset = f.ofKind(), nil, 0, 0
	f.listView = f.narrow(favouritesFilter)
	return f
}

// ofKind is the favourites of the current media type, newest first.
func (f favouritesContent) ofKind() []store.Favourite {
	var items []store.Favourite
	for _, fav := range f.everyKind {
		if fav.Kind == f.kind {
			items = append(items, fav)
		}
	}
	return items
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
	for _, fav := range f.everyKind {
		if fav.Kind == kind {
			return true
		}
	}
	return false
}

// ask is the command a key sends for the favourite under the cursor, or nil
// when the list has none.
func (f favouritesContent) ask(action favouriteAction) tea.Cmd {
	if f.cursor >= len(f.items) || f.isGoing(f.items[f.cursor]) || f.onBackRow() {
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
		style := queueRowStyle()
		if fav.Kind == backKind || f.isGoing(fav) || fav.ID == f.coming {
			style = style.Foreground(themedGoingColor()).Bold(false)
		}
		return f.drawRow(index, fav.Title, fav.Artist, [2]int{title, artist}, style, arrow, false)
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

// headingParts names the modal and, beside it, how much of the list there
// is, when that is worth saying. The media type in view sits at the right
// edge with ‹ and › either side, each subdued where ← or → cannot go.
func (f favouritesContent) headingParts() []headingPart {
	// Inside an opened favourite, its name stands in for "Favourites". The
	// arrows on the right still say where tab can go, at every level.
	name := "Favourites"
	if f.browsing != nil {
		name = f.browsing.parent.Title
	}
	left := []headingPart{{text: name, style: favouritesHeadingStyle()}}
	// The filter box sits beside the name, as in the queue: / is the prompt,
	// drawn quieter than what is typed after it.
	if f.filtering {
		left = append(left,
			headingPart{text: headingSeparator, style: favouritesHeadingStyle()},
			headingPart{text: "/", style: queueFilterPromptStyle()},
			headingPart{text: f.filter, style: favouritesHeadingStyle()})
	}
	if count := f.countLabel(); count != "" {
		left = append(left, headingPart{text: headingSeparator + count, style: favouritesHeadingStyle()})
	}
	if f.kind == "" {
		return fitHeading(left, f.width)
	}
	before, after := f.neighbours()
	right := []headingPart{
		{text: "‹", style: f.arrowStyle(before, bumpLeft)},
		{text: " " + favouriteKindNames[f.kind] + " ", style: favouritesKindStyle()},
		{text: "›", style: f.arrowStyle(after, bumpRight)},
	}
	// The count goes first when the box is too narrow for all of it: the
	// type is what ← and → change, so it stays in view longest.
	if headingWidth(left)+headingWidth(right) >= f.width && len(left) > 1 {
		left = left[:1]
	}
	gap := max(f.width-headingWidth(left)-headingWidth(right), 1)
	parts := append(left, headingPart{text: strings.Repeat(" ", gap)})
	return fitHeading(append(parts, right...), f.width)
}

// countLabel is how much of the list there is: which part is on screen when
// it overflows, how many there are past favouritesCountFloor, and nothing
// for a list short enough to count at a glance.
func (f favouritesContent) countLabel() string {
	if f.browsing != nil {
		return f.trackCountLabel()
	}
	switch {
	case f.note() != "":
		return ""
	case f.filter != "":
		return f.matchLabel()
	case f.overflows():
		return f.rangeLabel()
	case len(f.items) > favouritesCountFloor:
		return fmt.Sprintf("%d", len(f.items))
	default:
		return ""
	}
}

// headingWidth is the display width of heading parts laid side by side.
func headingWidth(parts []headingPart) int {
	width := 0
	for _, part := range parts {
		width += lipgloss.Width(part.text)
	}
	return width
}

// note is the whole body when there are no rows to draw, and empty otherwise.
func (f favouritesContent) note() string {
	switch {
	case f.loading:
		return "Reading the favourites…"
	case f.err != nil:
		return "Could not read the favourites: " + f.err.Error()
	case len(f.everyKind) == 0:
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

// arrowStyle is how an arrow is drawn: lit or subdued as it can be used,
// and, while a press against its side lights that border, in the same colour
// and cooling with it, so the arrow answers the press too.
func (f favouritesContent) arrowStyle(usable bool, side bumpEdge) lipgloss.Style {
	if heat := f.heat(); f.edge == side && heat >= bumpVisibleAt {
		return lipgloss.NewStyle().Bold(true).Foreground(bumpColour(heat))
	}
	return favouritesArrowStyle(usable)
}

// favouritesKindStyle draws the media type in the border's rose taken nearly
// to white, or to its deepest on a light ground, so it stands out from the
// rose text beside it: it is what ← and → change, and what the list holds.
func favouritesKindStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).
		Foreground(theme.LightDark(lipgloss.Color("#831843"), lipgloss.Color("#FCE7F3")))
}

// favouritesArrowStyle sets ‹ and › apart from the name between them: white
// on a dark ground, near-black on a light one, as the queue's scroll arrows,
// and subdued as they rest where there is nowhere to go.
func favouritesArrowStyle(usable bool) lipgloss.Style {
	if !usable {
		return lipgloss.NewStyle().Foreground(themedArrowRest())
	}
	return lipgloss.NewStyle().Bold(true).Foreground(themedArrowLit())
}

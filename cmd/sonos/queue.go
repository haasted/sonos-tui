package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

const (
	queueHeaderRows = 2 // the room line and the rule below it
	queueMarkWidth  = 2 // the cursor and the space after it
	queueArrowWidth = 1 // the scroll arrow at the right edge of a row
	queueGap        = 2 // between one column and the next

	// The floors the two columns keep when the modal cannot hold them both.
	// Below these a column stops carrying information and only carries an
	// ellipsis.
	queueTitleMin  = 20
	queueArtistMin = 12

	// queuePulseInterval is how often the arrows repaint. A breath takes five
	// seconds, so twenty frames a second is more than the eye asks for.
	queuePulseInterval = 50 * time.Millisecond

	// bumpDuration is how long the edge stays lit after the reader presses
	// against it. Short: it answers a keypress and then gets out of the way.
	bumpDuration = 320 * time.Millisecond

	// highlightDuration is how long a moved track flashes before fading back
	// to normal. Short, like the edge bump: it answers a command that just
	// landed, not a state to dwell on.
	highlightDuration = 500 * time.Millisecond
)

// selectionCountFloor is how many tracks must be picked before the heading
// says how many. Up to it the marks on the rows are the answer and the reader
// has not lost count; a number appearing on the first press is noise.
const selectionCountFloor = 3

// queueCursorGlyph marks the row the reader is on. It used to mark the playing
// track, which now shows as colour alone: the cursor moves and so needs the
// stronger signal of the two.
const queueCursorGlyph = "▶"

// bumpEdge is which end of the list the reader pressed against.
type bumpEdge int

const (
	bumpNone bumpEdge = iota
	bumpTop
	bumpBottom
)

// selectedTrack names one slot of the queue in a way that survives a reload.
// The URI alone cannot: the same track can sit in a queue twice, so a URI
// names both copies and a command meant for the second would reach the first.
// A position cannot either, since any move changes it.
//
// nth is which copy of that URI this is, counting from the top of the queue.
type selectedTrack struct {
	uri string
	nth int // 0-based, among the items sharing this URI
}

// queueContent lists a speaker's queue inside a modal. It holds what it was
// given and where the reader has scrolled to; it fetches nothing itself.
type queueContent struct {
	room  string
	items []sonos.QueueItem

	// filter is what the reader has typed, and filtering says the box is open
	// and taking their keys. Open with nothing typed still shows everything,
	// so the two are not one field.
	filter    string
	filtering bool

	// all is the whole queue while a filter narrows items to part of it, and
	// nil otherwise -- with no filter, items already IS everything. Read it
	// through everything(), never directly.
	//
	// Anything that decides a track's identity has to read the whole queue:
	// nth counts the copies of a URI above a track, and counted against a
	// filtered list it would change with whatever the reader typed.
	all     []sonos.QueueItem
	playing int
	err     error
	loading bool

	cursor   int       // the row the reader is on, as an index into items
	offset   int       // first visible item
	rows     int       // how many rows the modal will show, its frame excluded
	width    int       // how many columns it will show, likewise
	openedAt time.Time // when the list appeared, which is when the breath began

	edge     bumpEdge  // which end the reader last pressed against
	bumpedAt time.Time // when they did; zero when nothing is lit

	// selected are the tracks the reader has picked out, in the order they
	// picked them. A slice because that order is the point, and a map has
	// none of its own.
	selected []selectedTrack

	// moveFrom is where the moving track sat when the window opened, 1-based.
	// It is what the speaker is told, since the rows the reader now sees exist
	// only here. Zero when no move is pending.
	moveFrom int
	moveGen  int // id of the open commit window, matched against arriving ticks

	// removing are the tracks the reader has taken off, by their 1-based
	// position in the speaker's queue. They stay in items and are drawn
	// struck through until the speaker's own queue arrives without them, so
	// the reader can see what is on its way out rather than watching rows
	// vanish and the list jump.
	//
	// Positions, because that is what the speaker is told, and they stay
	// right while only this list is edited: nothing here renumbers anything.
	removing []int

	// removeGen is the open window, matched against arriving ticks, and zero
	// once the window has closed. Zero with removing still full is the state
	// between telling the speaker and hearing its answer.
	removeGen int

	// awaitingAt is when the speaker was last told to change its queue, and
	// is zero once it has answered with one.
	//
	// While it is set, every Position on this list describes the queue from
	// BEFORE that change, so no command naming a position may be sent: the
	// speaker has renumbered and this list has not heard. The version check
	// does not cover it -- the version is the one the change produced, so a
	// stale position is accepted and reaches the wrong track.
	awaitingAt time.Time

	// settling are the tracks to look for in the queue that comes back: the
	// speaker is what says where they really went. URIs rather than
	// positions, because the position is precisely what changed. More than
	// one when a whole selection was played next, and none for a removal,
	// which has no track left to find.
	//
	// settlingFollow says whether landing should also move the cursor onto
	// the track (a shift committed with J/K, which the reader is watching
	// move) or only flash it (play next, which deliberately leaves the
	// cursor where it was -- see playNext).
	settling       []string
	settlingFollow bool

	// highlightURIs are the tracks flashing briefly after a move landed, so
	// the reader sees where they went even when the cursor does not follow
	// them there. Nil when nothing is highlighted.
	//
	// A map, so a row asks in one lookup rather than searching a list every
	// time it is drawn. Replaced wholesale rather than written to, for the
	// reason selected carries: every copy of this content shares it.
	highlightURIs map[string]bool
	highlightAt   time.Time
}

var _ tea.Model = queueContent{}

func (queueContent) Init() tea.Cmd { return nil }

// Update scrolls the list. The modal forwards every key except esc, so these
// keys mean the queue here and the player everywhere else: j is the previous
// track outside this list and one row down inside it.
//
// The keys are the ones charm.land/bubbles binds by default, which are the
// ones less and vi established.
func (q queueContent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return q, nil
	}

	// The filter box has the keyboard while it is open: a reader typing a
	// title must not be moving the cursor with every letter of it.
	if q.filtering {
		return q.filterKey(key.String()), nil
	}

	switch key.String() {
	case "esc":
		return q.closeFilter(), nil
	case "/":
		return q.openFilter(), nil
	case "up", "k":
		q = q.moveBy(-1)
	case "down", "j":
		q = q.moveBy(1)
	case "pgup", "b":
		q = q.moveBy(-q.page())
	case "pgdown", "f", "space":
		q = q.moveBy(q.page())
	case "ctrl+u":
		q = q.moveBy(-q.halfPage())
	case "ctrl+d":
		q = q.moveBy(q.halfPage())
	case "home", "g":
		q = q.moveToEnd(bumpTop)
	case "end", "G":
		q = q.moveToEnd(bumpBottom)
	case "c":
		q = q.jumpToPlaying()
	case "x", "delete":
		return q.remove()
	case "N":
		return q.sendToBottom()
	case "n":
		return q.playNext()
	case "s", "right":
		return q.toggleSelect()
	case "S":
		q = q.clearSelection()
	case "*":
		q = q.invertSelection()
	case "enter":
		return q.playFromCursor()
	case "K", "shift+up":
		return q.shift(-1)
	case "J", "shift+down":
		return q.shift(1)
	}
	return q, nil
}

// shift moves the track under the cursor one place and keeps the cursor on it.
//
// THIS IS THE ONE PLACE THE LIST EDITS ITSELF, and it is deliberate. Everywhere
// else the speaker is told and the ContentDirectory event brings the new queue
// back. A move cannot wait for that: the track has to follow the key on every
// press or the key feels broken, so for as long as the window below is open the
// rows on screen are the reader's and not the speaker's.
func (q queueContent) shift(delta int) (queueContent, tea.Cmd) {
	// A move has been sent and the queue it produced has not arrived. Every
	// position held here still describes the queue from before it, so a second
	// move built on them would name the wrong track. Wait: the list is about to
	// be replaced, and moving rows that are about to be thrown away is what
	// makes tracks appear to jump.
	if q.awaitingQueue() {
		return q, nil
	}

	target := q.cursor + delta
	if q.cursor >= len(q.items) || target < 0 || target >= len(q.items) {
		if delta < 0 {
			return q.bump(bumpTop), nil
		}
		return q.bump(bumpBottom), nil
	}

	// Where the track started, remembered once. A run of presses is one move
	// from there, and the rows it passed over never existed for the speaker.
	if q.moveFrom == 0 {
		q.moveFrom = q.items[q.cursor].Position
	}

	q.items = movedItems(q.items, q.cursor, target)
	q.cursor = target
	q = q.follow()

	// Every press restarts the wait, taking a generation of its own so the
	// tick the last press left behind is ignored when it arrives. The move
	// therefore goes out once the keys are still, however long the run.
	q.moveGen = nextQueueGen()
	return q, queueMoveTick(q.moveGen)
}

// movedItems is the list with one item lifted out and put back at another
// index. It copies rather than reordering in place: the slice it is given is
// shared with the Device that read it, and editing that would change what
// every other reader of the queue sees.
func movedItems(items []sonos.QueueItem, from, to int) []sonos.QueueItem {
	moved := make([]sonos.QueueItem, 0, len(items))
	moved = append(moved, items[:from]...)
	moved = append(moved, items[from+1:]...)

	rest := make([]sonos.QueueItem, 0, len(items))
	rest = append(rest, moved[:to]...)
	rest = append(rest, items[from])
	return append(rest, moved[to:]...)
}

// pendingMove is the move waiting to be sent, and whether there is one. to is
// the position the track now appears at, which is what the speaker is asked
// for.
func (q queueContent) pendingMove() (from, to int, pending bool) {
	if q.moveFrom == 0 || q.cursor >= len(q.items) {
		return 0, 0, false
	}
	return q.moveFrom, q.cursor + 1, true
}

// clearMove closes the window, whether it was committed or abandoned.
func (q queueContent) clearMove() queueContent {
	q.moveFrom, q.moveGen = 0, 0
	return q
}

// settleMove records that a committed move has gone to the speaker and names
// the track to look for in the queue that comes back, moving the cursor onto
// it once found.
func (q queueContent) settleMove(uri string) queueContent {
	q.settling, q.settlingFollow = []string{uri}, true
	return q.awaitQueue()
}

// awaitHighlight is settleMove without moving the cursor: the queue that
// comes back is still recognised as this command's own answer, but only the
// flash follows the track, the way playNext wants it.
func (q queueContent) awaitHighlight(uris ...string) queueContent {
	q.settling, q.settlingFollow = uris, false
	return q.awaitQueue()
}

// awaitQueue records that the speaker has been told to change its queue, so
// nothing naming a position is sent until it answers.
func (q queueContent) awaitQueue() queueContent {
	q.awaitingAt = time.Now()
	return q
}

// awaitingQueue reports a change sent to the speaker whose queue has not come
// back. Every command that names a position asks it first, and they all name
// positions: a move, a removal and a played-next move alike.
//
// It expires, because the event that would end it can be dropped -- Device
// says so when its channel fills -- and a reader must not be left unable to
// change anything.
func (q queueContent) awaitingQueue() bool {
	return !q.awaitingAt.IsZero() && time.Since(q.awaitingAt) < queueWriteSettle
}

// clearAwaiting closes the wait without looking for any track, which is what
// a removal leaves behind: the tracks it was about are gone.
func (q queueContent) clearAwaiting() queueContent {
	q.awaitingAt = time.Time{}
	return q
}

// followSettled puts the cursor back on the track a move sent away, now that
// the speaker has said where it went. The reader was watching that track, not
// that row.
func (q queueContent) followSettled() queueContent {
	q, uris := q.clearSettling()
	if len(uris) == 0 {
		return q
	}

	// Only ever one: J and K commit a single track, and the cursor cannot
	// follow more than one anyway.
	for i, item := range q.items {
		if item.URI == uris[0] {
			q.cursor = i
			return q.follow()
		}
	}
	return q
}

// highlightSettled flashes the tracks a played-next move sent away, now that
// the speaker has said where they landed, and leaves the cursor on a track
// that did not move -- see playNext for why it does not follow them.
func (q queueContent) highlightSettled() queueContent {
	q, uris := q.clearSettling()
	if len(uris) == 0 {
		return q
	}

	moved := make(map[string]bool, len(uris))
	for _, uri := range uris {
		moved[uri] = true
	}

	q = q.restOn(moved)
	return q.highlight(moved)
}

// restOn puts the cursor on the nearest track that did NOT move, looking
// down first and then up.
//
// The tracks the reader sent away are now somewhere else, and a cursor left
// sitting on one would have followed them there without being asked. Looking
// down first keeps it near the rows they were working through.
func (q queueContent) restOn(moved map[string]bool) queueContent {
	if q.cursor >= len(q.items) || !moved[q.items[q.cursor].URI] {
		return q.follow()
	}

	for i := q.cursor + 1; i < len(q.items); i++ {
		if !moved[q.items[i].URI] {
			q.cursor = i
			return q.follow()
		}
	}
	for i := q.cursor - 1; i >= 0; i-- {
		if !moved[q.items[i].URI] {
			q.cursor = i
			return q.follow()
		}
	}
	return q.follow() // every track moved; there is nowhere untouched to go
}

// clearSettling closes the wait, returning the updated content alongside the
// track it was waiting for, or empty if none was.
func (q queueContent) clearSettling() (queueContent, []string) {
	uris := q.settling
	q.settling, q.settlingFollow = nil, false
	q = q.clearAwaiting()
	return q, uris
}

// highlight marks the tracks that just landed, to flash briefly the next
// time the queue holding them is drawn. The map is taken as given and never
// written to afterwards -- see the field.
func (q queueContent) highlight(uris map[string]bool) queueContent {
	q.highlightURIs, q.highlightAt = uris, time.Now()
	return q
}

// highlightGlow is how lit uri's flash is, from 1 just after landing to 0
// once highlightDuration has passed, and for any other track.
// highlightHold is the share of highlightDuration the flash stays at full
// glow before it starts to fade. A pulse that starts fading on its very
// first frame is only ever seen mid-fade -- held here for a beat first, so a
// reader glancing right after the queue lands still catches it fully lit.
const highlightHold = 0.35

func (q queueContent) highlightGlow(uri string) float32 {
	if !q.highlightURIs[uri] {
		return 0
	}

	t := float32(time.Since(q.highlightAt)) / float32(highlightDuration)
	switch {
	case t >= 1:
		return 0
	case t <= highlightHold:
		return 1
	default:
		return 1 - (t-highlightHold)/(1-highlightHold)
	}
}

// highlighting reports whether a flash is still fading, so the repaint chain
// knows to keep ticking for it even where nothing else asks for a repaint.
func (q queueContent) highlighting() bool {
	return len(q.highlightURIs) > 0 && time.Since(q.highlightAt) < highlightDuration
}

// withSelection returns the content carrying a changed selection, built by
// change against a copy of the current one.
//
// Copied rather than appended to, because the backing array is the one thing
// here every copy of queueContent shares: an append into spare capacity would
// reach a copy a superseded update was about to throw away. The same reason
// movedItems copies the item slice.
func (q queueContent) withSelection(change func([]selectedTrack) []selectedTrack) queueContent {
	q.selected = change(slices.Clone(q.selected))
	return q
}

// matchesFilter reports whether a track answers what the reader typed. The
// title and the artist both, and case-insensitively: a reader typing "nick"
// is naming an artist as readily as a song.
func matchesFilter(item sonos.QueueItem, filter string) bool {
	if filter == "" {
		return true
	}
	want := strings.ToLower(filter)
	return strings.Contains(strings.ToLower(item.Title), want) ||
		strings.Contains(strings.ToLower(item.Artist), want)
}

// applyFilter narrows items to the tracks that answer the filter, keeping the
// whole queue in all so nothing that decides identity loses sight of it.
//
// The cursor keeps the track it was on where that track is still shown, and
// otherwise goes to the top: a cursor left on a row number would land on a
// different track every keystroke.
func (q queueContent) applyFilter() queueContent {
	var on string
	if q.cursor < len(q.items) {
		on = q.items[q.cursor].URI
	}

	q = q.narrow()

	q.cursor = 0
	for i, item := range q.items {
		if item.URI == on {
			q.cursor = i
			break
		}
	}
	return q.follow()
}

// narrow is applyFilter without the cursor keeping, for a queue arriving from
// the speaker: the cursor there is settled afterwards by whatever the read
// was answering, and a row remembered from the list before it means nothing.
//
// A queue must go through here on the way in. Assigning items straight from
// the speaker would widen the list behind a filter that is still on screen.
func (q queueContent) narrow() queueContent {
	all := q.everything()
	if q.filter == "" {
		q.items, q.all = all, nil
		return q
	}

	shown := make([]sonos.QueueItem, 0, len(all))
	for _, item := range all {
		if matchesFilter(item, q.filter) {
			shown = append(shown, item)
		}
	}
	q.items, q.all = shown, all
	return q
}

// ClaimsEsc keeps esc from closing the modal while there is a filter to
// clear. The reader's first esc undoes the narrowing; a second closes the
// queue, as it always did.
func (q queueContent) ClaimsEsc() bool { return q.filtering || q.filter != "" }

// filterKey is one key while the filter box is open. enter and down both
// leave the box and keep the narrowing, so the keys that act on tracks reach
// the rows it found: down because the reader's next move after typing is
// into the list, and reaching for it should be enough.
func (q queueContent) filterKey(key string) queueContent {
	switch key {
	case "esc":
		return q.closeFilter()
	case "enter", "down":
		q.filtering = false
		return q
	case "backspace":
		return q.typeFilter("backspace")
	}
	if len([]rune(key)) == 1 {
		return q.typeFilter(key)
	}
	return q
}

// openFilter starts taking the reader's keys as a filter, from nothing. A box
// that opened holding the last search would narrow the list before the reader
// had asked for anything, and the term they were shown would be one they had
// forgotten typing.
func (q queueContent) openFilter() queueContent {
	q.filtering, q.filter = true, ""
	return q.applyFilter()
}

// closeFilter stops filtering and shows the whole queue again. What the
// reader did while it was narrowed stays done: the selection is held by
// track, and tracks on their way out by position, neither of which the
// filter ever touched.
func (q queueContent) closeFilter() queueContent {
	if !q.filtering && q.filter == "" {
		return q
	}
	q.filtering, q.filter = false, ""
	return q.applyFilter()
}

// typeFilter adds to what the reader has typed, or takes the last rune back.
func (q queueContent) typeFilter(key string) queueContent {
	switch key {
	case "backspace":
		if q.filter == "" {
			return q
		}
		runes := []rune(q.filter)
		q.filter = string(runes[:len(runes)-1])
	default:
		q.filter += key
	}
	return q.applyFilter()
}

// everything is the whole queue, filtered or not.
func (q queueContent) everything() []sonos.QueueItem {
	if q.all != nil {
		return q.all
	}
	return q.items
}

// eachTrack calls visit for every track in the WHOLE queue with the position
// it sits at and the track that names it. The one place that decides which
// copy of a repeated URI a track is, so the answer cannot differ between the
// reader picking it and the list drawing it -- nor with what a filter is
// showing at the time.
func (q queueContent) eachTrack(visit func(position int, track selectedTrack)) {
	all := q.everything()

	seen := make(map[string]int, len(all))
	for _, item := range all {
		visit(item.Position, selectedTrack{uri: item.URI, nth: seen[item.URI]})
		seen[item.URI]++
	}
}

// trackOf names one track, by counting the copies of its URI that sit above
// it. Taken from the item rather than from a row, because a row belongs to
// whatever the reader is being shown and the answer must not.
func (q queueContent) trackOf(item sonos.QueueItem) (found selectedTrack) {
	q.eachTrack(func(position int, track selectedTrack) {
		if position == item.Position {
			found = track
		}
	})
	return found
}

// selectedRows resolves the selection against the queue as it stands, giving
// the rows that are picked out.
//
// Resolved in one walk rather than a lookup per row, because which copy a row
// is depends on every row above it. A selected track no longer in the queue
// resolves to no row and is kept rather than pruned, which is what stops a
// read landing on a part-written queue from dropping a selection that is
// about to come back -- see SONOS-NOTES.md.
func (q queueContent) pickedPositions() map[int]bool {
	if len(q.selected) == 0 {
		return nil
	}

	picked := make(map[selectedTrack]bool, len(q.selected))
	for _, track := range q.selected {
		picked[track] = true
	}

	positions := make(map[int]bool, len(q.selected))
	q.eachTrack(func(position int, track selectedTrack) {
		if picked[track] {
			positions[position] = true
		}
	})
	return positions
}

// selectionCount is how many of the picked tracks are in the queue as it
// stands, which is not len(selected): the selection may name a track that a
// reload has taken away.
func (q queueContent) selectionCount() int { return len(q.pickedPositions()) }

// toggleSelect picks the track under the cursor out, or puts it back, and
// then steps down a row. Marking a run is then s, s, s rather than a step
// between each: the key answers with the row the reader wants next.
//
// The last row has no next to step to, so it answers with the bump every
// other key that cannot go further already uses.
func (q queueContent) toggleSelect() (queueContent, tea.Cmd) {
	if q.cursor >= len(q.items) {
		return q, nil // an empty queue, or one still being read
	}

	track := q.trackOf(q.items[q.cursor])
	q = q.withSelection(func(selected []selectedTrack) []selectedTrack {
		if at := slices.Index(selected, track); at >= 0 {
			return slices.Delete(selected, at, at+1)
		}
		return append(selected, track)
	})

	if q.cursor >= q.lastRow() {
		return q.bump(bumpBottom), nil
	}
	q.cursor++
	return q.follow(), nil
}

// clearSelection drops every track from the selection at once. It does not
// move the cursor: it answers the whole list rather than the row under it.
// Replaces the slice rather than truncating it, for withSelection's reason:
// the array it held is shared with every other copy of this content.
func (q queueContent) clearSelection() queueContent {
	q.selected = nil
	return q
}

// invertSelection picks out everything that was not picked, and puts back
// everything that was. Tracks it brings in are ordered by where they sit in
// the queue: a track the reader never pressed s on has no order of its own.
//
// It reaches only the tracks in the queue as it stands, so a selection that
// is no longer here is dropped by the inversion rather than carried.
func (q queueContent) invertSelection() queueContent {
	picked := q.pickedPositions()
	named := q.tracksByPosition()

	shown := make(map[int]bool, len(q.items))
	for _, item := range q.items {
		shown[item.Position] = true
	}

	// Picks the reader cannot see are left as they are. Inverting what is on
	// screen must not silently reach the rest of the queue behind a filter.
	next := make([]selectedTrack, 0, len(q.selected)+len(q.items))
	for _, track := range q.selected {
		if position, here := q.positionOfTrack(track); here && !shown[position] {
			next = append(next, track)
		}
	}
	for _, item := range q.items {
		if !picked[item.Position] {
			next = append(next, named[item.Position])
		}
	}

	q.selected = next
	return q
}

// tracksByPosition names every track in the whole queue.
func (q queueContent) tracksByPosition() map[int]selectedTrack {
	named := make(map[int]selectedTrack, len(q.everything()))
	q.eachTrack(func(position int, track selectedTrack) {
		named[position] = track
	})
	return named
}

// positionOfTrack is where a named track sits now, false when the queue no
// longer holds it.
func (q queueContent) positionOfTrack(want selectedTrack) (int, bool) {
	var at int
	var found bool
	q.eachTrack(func(position int, track selectedTrack) {
		if track == want {
			at, found = position, true
		}
	})
	return at, found
}

// selectedPositions is where the picked tracks sit in the queue, 1-based and
// in queue order. Empty when nothing is picked, which is how a command tells
// "act on the selection" from "act on the cursor".
//
// Queue order rather than the order they were pressed. Removal sorts them
// anyway, and a command that needs the reader's own order has it in selected.
func (q queueContent) selectedPositions() []int {
	picked := q.pickedPositions()
	if len(picked) == 0 {
		return nil
	}

	positions := make([]int, 0, len(picked))
	for _, item := range q.items {
		if picked[item.Position] {
			positions = append(positions, item.Position)
		}
	}
	return positions
}

// remove takes the picked tracks off the list, or the one under the cursor
// when nothing is picked, and starts the wait before the speaker is told.
//
// THE ROWS GO AT ONCE, for the reason shift edits the list: a key that
// answers only when the speaker has been round trips through an event and a
// read, and a reader pressing x four times sees nothing happen and then
// everything happen. The speaker is told once the keys are still, and a run
// of presses is then one call rather than one each.
//
// The cursor keeps its place, so the row below slides up under it, which is
// what a reader clearing several in a row wants; at the end of the list it is
// clamped back by one.
//
// The selection is dropped at once. It names tracks that have just left the
// list, and a reader whose removal fails would rather pick again than wonder
// what is still marked.
//
// A move still waiting to be sent is dropped first. Both commands address
// tracks by position, and a removal shifts every position below it -- so a move
// committed afterwards would name a track the reader never chose.
func (q queueContent) remove() (queueContent, tea.Cmd) {
	// A change already sent has not been answered. Every position here still
	// describes the queue from before it, so a removal built on them would
	// take the wrong track -- and the version check would not notice, since
	// the version is the one that change produced.
	if q.awaitingQueue() {
		return q, nil
	}

	positions := q.selectedPositions()
	if len(positions) == 0 {
		if q.cursor >= len(q.items) {
			return q, nil // an empty queue, or one still being read
		}
		positions = []int{q.items[q.cursor].Position}
	}

	// A track already on its way out must not be named twice: the second
	// naming would reach whatever the first left in its place.
	positions = slices.DeleteFunc(positions, q.isRemoving)
	if len(positions) == 0 {
		return q, nil
	}

	var dropped tea.Cmd
	if _, _, pending := q.pendingMove(); pending {
		q = q.clearMove()
		dropped = notify(panes.NoticeWarn, queueMoveAbandoned)
	}
	q = q.clearSelection()

	// Cloned rather than appended to, for the reason withSelection carries:
	// every copy of this content shares the array behind it.
	q.removing = append(slices.Clone(q.removing), positions...)
	q.cursor = q.restingRow(q.cursor)
	q = q.follow()

	// Every press restarts the wait, taking a generation of its own, so the
	// tick an earlier press left behind is ignored when it arrives.
	q.removeGen = nextQueueGen()
	return q, tea.Batch(queueRemoveTick(q.removeGen), dropped)
}

// pendingRemove reports the tracks taken off the list and NOT YET SENT. A
// window still open, as against one closed and waiting for the answer.
func (q queueContent) pendingRemove() ([]int, bool) {
	return q.removing, q.removeGen != 0 && len(q.removing) > 0
}

// closeRemoveWindow ends the wait for more presses, keeping the tracks
// themselves: they are on their way out and stay struck through until the
// speaker's queue says they have gone.
func (q queueContent) closeRemoveWindow() queueContent {
	q.removeGen = 0
	return q
}

// isRemoving reports a track already on its way out, which must not be sent
// twice and is not drawn as if it were staying.
func (q queueContent) isRemoving(position int) bool {
	return slices.Contains(q.removing, position)
}

// restingRow is the nearest row that is not on its way out, looking down
// first and then up. The cursor never sits on a struck-through row: the key
// would do nothing there, which reads as the key being broken.
func (q queueContent) restingRow(from int) int {
	for i := from; i < len(q.items); i++ {
		if !q.isRemoving(q.items[i].Position) {
			return i
		}
	}
	for i := min(from, len(q.items)) - 1; i >= 0; i-- {
		if !q.isRemoving(q.items[i].Position) {
			return i
		}
	}
	return min(from, q.lastRow()) // every row is going
}

// clearRemoving closes the window, whether the tracks went to the speaker or
// the window was abandoned.
func (q queueContent) clearRemoving() queueContent {
	q.removing, q.removeGen = nil, 0
	return q
}

// pickedInPressOrder is the picked tracks as queue positions, in the order
// the reader
// picked them. selected holds that order; pickedPositions says which of them
// are still in the queue.
func (q queueContent) pickedInPressOrder() []int {
	picked := q.pickedPositions()
	if len(picked) == 0 {
		return nil
	}

	at := make(map[selectedTrack]int, len(picked))
	q.eachTrack(func(position int, track selectedTrack) {
		if picked[position] {
			at[track] = position
		}
	})

	ordered := make([]int, 0, len(picked))
	for _, track := range q.selected {
		if position, here := at[track]; here {
			ordered = append(ordered, position)
		}
	}
	return ordered
}

// playNextMoves works out the moves that put tracks directly after the one
// playing, in the order given, and returns them with the URIs they carry.
//
// Built against a model of the queue rather than against arithmetic. Every
// move shifts the tracks around it, upwards or downwards depending on which
// way it travelled, so where a track sits when its turn comes is not where
// the reader saw it. The model is walked forward move by move, and each move
// reads its own starting place from it.
//
// The tracks go in REVERSE order, each aimed at the same slot right after the
// one playing: the last one lands there first and the ones before it push it
// down, so they end up in the order asked for. A track already in the place
// its turn would put it is left alone rather than sent to where it is.
// queueModel is the whole queue as a run of positions, with the URI each one
// carries. POSITIONS, not URIs: a position names one track even when the
// queue holds the same track twice, where a URI names both and the first
// copy would be moved for the second.
func (q queueContent) queueModel() ([]int, map[int]string) {
	all := q.everything()

	model := make([]int, 0, len(all))
	uriAt := make(map[int]string, len(all))
	for _, item := range all {
		model = append(model, item.Position)
		uriAt[item.Position] = item.URI
	}
	return model, uriAt
}

func (q queueContent) playNextMoves(positions []int) ([]queueMove, []string) {
	model, uriAt := q.queueModel()

	wanted := make([]int, 0, len(positions))
	for _, position := range positions {
		if position == q.playing {
			continue // it cannot be next of itself
		}
		wanted = append(wanted, position)
	}

	var moves []queueMove
	var carried []string
	for i := len(wanted) - 1; i >= 0; i-- {
		position := wanted[i]

		from := slices.Index(model, position)
		// Read from the model each time round: a track lifted from above the
		// playing one shifts it up, so where it sits is not where it started.
		playing := slices.Index(model, q.playing)
		if from < 0 || playing < 0 || from == playing+1 {
			continue // gone from the queue, or already where this step wants it
		}

		to := playing + 1
		if from < playing {
			to = playing // lifting it from above shifts the playing track up first
		}

		moves = append(moves, queueMove{from: from + 1, to: to + 1})
		carried = append(carried, uriAt[position])

		model = slices.Delete(model, from, from+1)
		model = slices.Insert(model, to, position)
	}
	return moves, carried
}

// sendToBottomMoves works out the moves that put tracks at the end of the
// queue, in the order given.
//
// FORWARD through them, where playNextMoves runs backward: each track is
// sent to the end, so the one sent first ends up above the one sent after
// it and the run reads in the order the reader picked them. Aimed at the
// end both times, as playNextMoves aims at the one place after the playing
// track, so only the target differs.
func (q queueContent) sendToBottomMoves(positions []int) ([]queueMove, []string) {
	model, uriAt := q.queueModel()

	var moves []queueMove
	var carried []string
	for _, position := range positions {
		from := slices.Index(model, position)
		last := len(model) - 1
		if from < 0 || from == last {
			continue // gone from the queue, or already the bottom track
		}

		moves = append(moves, queueMove{from: from + 1, to: last + 1})
		carried = append(carried, uriAt[position])

		model = slices.Delete(model, from, from+1)
		model = slices.Insert(model, last, position)
	}
	return moves, carried
}

// sendToBottom asks for the picked tracks to move to the end of the queue,
// or the one under the cursor when nothing is picked. The opposite of play
// next, and it answers the same way: the cursor stays behind on a track that
// did not move, and every track that did flashes as it lands.
//
// The playing track may go, unlike play next where it cannot be next of
// itself. It keeps playing where it lands, and the queue carries on from
// where it was, which is odd but is what the reader asked for.
func (q queueContent) sendToBottom() (queueContent, tea.Cmd) {
	if q.awaitingQueue() {
		return q, nil // a change already sent has not been answered
	}

	positions := q.pickedInPressOrder()
	if len(positions) == 0 {
		if q.cursor >= len(q.items) {
			return q, nil // an empty queue, or one still being read
		}
		positions = []int{q.items[q.cursor].Position}
	}

	moves, carried := q.sendToBottomMoves(positions)
	if len(moves) == 0 {
		return q.clearSelection(), notify(panes.NoticeInfo, queueAlreadyLast)
	}

	var dropped tea.Cmd
	if _, _, pending := q.pendingMove(); pending {
		q = q.clearMove()
		dropped = notify(panes.NoticeWarn, queueMoveAbandoned)
	}
	q = q.clearSelection()

	ask := func() tea.Msg { return queueMovesMsg{moves: moves} }
	return q.awaitHighlight(carried...), tea.Batch(ask, dropped)
}

// playNext asks for the picked tracks to move directly after the one
// playing, so they are the next things heard, in the order the reader picked
// them. With nothing picked it acts on the track under the cursor.
//
// Unlike shift's debounced window this is decided and sent in one step: there
// is no run of presses to gather, so the list waits for the ContentDirectory
// event like remove does.
//
// The cursor does not follow the tracks. Once the reordered queue arrives it
// settles on the nearest track that did not move -- see restOn.
func (q queueContent) playNext() (queueContent, tea.Cmd) {
	if q.awaitingQueue() {
		return q, nil // a move already sent has not been answered yet
	}

	positions := q.pickedInPressOrder()
	picked := len(positions) > 0
	if !picked {
		if q.cursor >= len(q.items) {
			return q, nil // an empty queue, or one still being read
		}
		positions = []int{q.items[q.cursor].Position}
	}

	// A station plays past the queue without touching it. DeviceState.Track
	// then counts the stream, so playing is left at zero: there is no current
	// track to be "next" of, even though the queue behind it is right there.
	if q.playing <= 0 {
		return q, notify(panes.NoticeInfo, queuePlayNextNoTrack)
	}

	// One track that is already playing, or already next, has nothing to do
	// and is worth saying so. A selection says it with the count instead: a
	// picked track in either place is simply left where it is.
	if !picked {
		switch positions[0] {
		case q.playing:
			return q, notify(panes.NoticeInfo, queueAlreadyPlaying)
		case q.playing + 1:
			return q, notify(panes.NoticeInfo, queueAlreadyNext)
		}
	}

	moves, carried := q.playNextMoves(positions)
	if len(moves) == 0 {
		q = q.clearSelection()
		return q, notify(panes.NoticeInfo, queueAlreadyNext)
	}

	var dropped tea.Cmd
	if _, _, pending := q.pendingMove(); pending {
		q = q.clearMove()
		dropped = notify(panes.NoticeWarn, queueMoveAbandoned)
	}
	q = q.clearSelection()

	ask := func() tea.Msg { return queueMovesMsg{moves: moves} }
	return q.awaitHighlight(carried...), tea.Batch(ask, dropped)
}

// playFromCursor asks for the track under the cursor to start playing now,
// whatever the speaker was doing before -- a station, another track, or this
// one paused. Restarting the track already playing is a real answer to enter,
// not a state worth refusing the way playNext refuses an unchanged position.
func (q queueContent) playFromCursor() (queueContent, tea.Cmd) {
	if q.cursor >= len(q.items) {
		return q, nil // an empty queue, or one still being read
	}

	position := q.items[q.cursor].Position
	ask := func() tea.Msg { return queuePlayFromMsg{position: position} }
	return q, ask
}

// moveBy moves the cursor by delta rows, stopping at either end. It lights an
// edge only where the cursor could not move at all: a page down that would run
// past the last row lands on it instead, which is no refusal.
//
// Every relative key goes through it, and every absolute one through
// moveToEnd, so what a move means is decided in two functions rather than in
// each of eight bindings.
func (q queueContent) moveBy(delta int) queueContent {
	target := max(min(q.cursor+delta, q.lastRow()), 0)
	if target == q.cursor {
		if delta < 0 {
			return q.bump(bumpTop)
		}
		return q.bump(bumpBottom)
	}
	q.cursor = target
	return q.follow()
}

// moveToEnd puts the cursor on the first or the last row. The edge is passed in
// rather than derived from the row it lands on, because a queue of one item has
// the same row at both ends and the two keys would then light the same border.
func (q queueContent) moveToEnd(edge bumpEdge) queueContent {
	target := 0
	if edge == bumpBottom {
		target = q.lastRow()
	}
	if target == q.cursor {
		return q.bump(edge)
	}
	q.cursor = target
	return q.follow()
}

// jumpToPlaying moves the cursor to the track now playing. Pausing or muting
// the speaker does not change which track that is, only stopping the queue
// itself does, so this answers "playing" the same way in every one of those
// states.
func (q queueContent) jumpToPlaying() queueContent {
	if q.playing <= 0 {
		return q
	}
	for i, item := range q.items {
		if item.Position == q.playing {
			q.cursor = i
			return q.follow()
		}
	}
	return q
}

// follow scrolls the window the least it can to keep the cursor on screen. The
// window is what moves; the cursor is where the reader is.
func (q queueContent) follow() queueContent {
	switch {
	case q.cursor < q.offset:
		q.offset = q.cursor
	case q.cursor >= q.offset+q.visibleRows():
		q.offset = q.cursor - q.visibleRows() + 1
	}
	q.offset = max(min(q.offset, q.maxOffset()), 0)
	return q
}

// lastRow is the index of the final item, or zero for a queue with none. The
// cursor always sits somewhere, even on a list with nowhere to sit.
func (q queueContent) lastRow() int { return max(len(q.items)-1, 0) }

// page is how far a page move travels: one whole window, as a pager does. The
// reader keeps their place by the heading, which names the rows on screen.
func (q queueContent) page() int { return q.visibleRows() }

// halfPage is the move ctrl+u and ctrl+d make, which keeps half the window in
// view and so needs no heading to stay oriented.
func (q queueContent) halfPage() int { return max(q.visibleRows()/2, 1) }

// bump lights the end the reader pressed against. Every press restarts it, so
// a held key holds the edge lit and it cools once the key is released.
func (q queueContent) bump(edge bumpEdge) queueContent {
	q.edge, q.bumpedAt = edge, time.Now()
	return q
}

// bumping reports whether an edge is still lit.
func (q queueContent) bumping() bool { return q.heat() > 0 }

// heat is how lit the bumped edge is, from 1 at the press to 0 once cooled.
func (q queueContent) heat() float32 {
	if q.bumpedAt.IsZero() {
		return 0
	}
	left := bumpDuration - time.Since(q.bumpedAt)
	if left <= 0 {
		return 0
	}
	return float32(left) / float32(bumpDuration)
}

func (q queueContent) View() tea.View {
	title, artist := q.columns()

	var b strings.Builder
	var heading strings.Builder
	for _, part := range q.headingParts() {
		heading.WriteString(part.style.Render(part.text))
	}
	b.WriteString(heading.String() + "\n")
	b.WriteString(queueRuleStyle().Render(strings.Repeat("─", q.ruleWidth())) + "\n")

	if note := q.note(); note != "" {
		b.WriteString(queueNoteStyle().Render(note))
		return tea.NewView(b.String())
	}

	// Resolved once for the whole pass: which copy of a repeated URI a row is
	// depends on every row above it, so asking per row would walk the queue
	// again for each one. Keyed by the speaker's own position, which a row
	// carries with it wherever the row sits.
	picked := q.pickedPositions()

	first, last := q.window()
	for i, item := range q.items[first:last] {
		b.WriteString(q.row(item, first+i, picked[item.Position], title, artist,
			q.arrow(first, last, i)) + "\n")
	}

	return tea.NewView(strings.TrimRight(b.String(), "\n"))
}

// columns is how wide the title and the artist should be. Neither is truncated
// while both fit. They are measured across the WHOLE queue rather than the
// visible window, so the columns do not shift as the reader scrolls.
func (q queueContent) columns() (title, artist int) {
	for _, item := range q.items {
		title = max(title, lipgloss.Width(item.Title))
		artist = max(artist, lipgloss.Width(item.Artist))
	}

	room := q.width - queueMarkWidth - queueGap
	if q.overflows() {
		room -= queueGap + queueArrowWidth
	}
	if room <= 0 {
		return title, artist
	}
	if title+artist <= room {
		// The box is a fixed width, so the row fills it rather than stopping
		// short of the rule above. The title takes the slack, being the field
		// that most often wants it.
		return room - artist, artist
	}

	// The artist is the shorter field and the one a reader recognises from
	// less of it, so it keeps its natural width while that fits in half the
	// row. The title takes what is left, which is the larger share.
	artist = min(artist, max(room/2, queueArtistMin))
	return max(room-artist, queueTitleMin), artist
}

// ruleWidth spans the widest thing under it, so the rule marks the list out
// rather than stopping short of it or running past.
func (q queueContent) ruleWidth() int { return q.width }

// heading names the room and the size of its queue, so a reader knows what
// they are looking at and how much of it they can see, and says how many
// tracks are picked when any are.
//
// The count is here because the rows alone cannot carry it: a reader who has
// scrolled away from what they picked, or who inverted the selection, has
// nothing else to read it from.
// headingPart is one run of the heading, drawn in its own style.
type headingPart struct {
	text  string
	style lipgloss.Style
}

func (q queueContent) heading() string {
	var b strings.Builder
	for _, part := range q.headingParts() {
		b.WriteString(part.text)
	}
	return b.String()
}

// headingParts is the heading split where its colour changes: the room and
// the size of the queue, then the selection count that follows them. The
// count is empty when there is none to show, or no room to show it in.
func (q queueContent) headingParts() []headingPart {
	parts := []headingPart{{q.room, queueRoomStyle()}}

	// The key that opens the box, kept on screen while it is open so a reader
	// who has not typed yet can see that they may. Drawn quieter than what
	// they type into it: it is the prompt, not the search.
	if q.filtering {
		parts = append(parts,
			headingPart{" · ", queueRoomStyle()},
			headingPart{"/", queueFilterPromptStyle()},
			headingPart{q.filter, queueRoomStyle()})
	}

	// How much of the queue is showing, or how big it is. Never both: a
	// narrowed list's own length is not a thing the reader asked about.
	if q.filter != "" {
		parts = append(parts, headingPart{" · " + q.matchLabel(), queueRoomStyle()})
	} else {
		parts = append(parts, headingPart{q.sizeSuffix(), queueRoomStyle()})
	}

	if picked := q.selectionCount(); picked > selectionCountFloor {
		parts = append(parts, headingPart{fmt.Sprintf(" · %d selected", picked), queueCountStyle()})
	}
	return q.fitHeading(parts)
}

// fitHeading drops what a narrow box cannot hold, last part first. The room
// and how much of the queue is showing answer where the reader is, which they
// need before the rest, so the rest is what goes rather than anything being
// cut in half.
func (q queueContent) fitHeading(parts []headingPart) []headingPart {
	width := func(parts []headingPart) int {
		var total int
		for _, part := range parts {
			total += lipgloss.Width(part.text)
		}
		return total
	}

	for q.width > 0 && len(parts) > 1 && width(parts) > q.width {
		parts = parts[:len(parts)-1]
	}
	if q.width > 0 && width(parts) > q.width {
		parts[0].text = clip(parts[0].text, q.width)
	}
	return parts
}

// queueLabel is the room and the size of its queue, which is the whole
// heading until something is picked.

// matchLabel is how much of the queue is showing.
func (q queueContent) matchLabel() string {
	return fmt.Sprintf("%d of %d", len(q.items), len(q.everything()))
}

// sizeSuffix says how big the queue is, and nothing at all while it is being
// read or is empty -- the note below the rule says that better.
func (q queueContent) sizeSuffix() string {
	switch {
	case q.loading, len(q.items) == 0:
		return ""
	case q.overflows():
		first, last := q.window()
		return " · " + q.rangeLabel(first, last)
	case len(q.items) == 1:
		return " · 1 track"
	default:
		return fmt.Sprintf(" · %d tracks", len(q.items))
	}
}

// rangeLabel says which part of the queue is on screen.
func (q queueContent) rangeLabel(first, last int) string {
	return fmt.Sprintf("%d-%d of %d", first+1, last, len(q.items))
}

// note is the whole body when there are no rows to draw, and empty otherwise.
func (q queueContent) note() string {
	switch {
	case q.loading:
		return "Reading the queue…"
	case q.err != nil:
		return "Could not read the queue: " + q.err.Error()
	case len(q.items) == 0:
		return "The queue is empty."
	default:
		return ""
	}
}

// window is the slice of items on screen. It keeps both marker lines in place
// while the queue overflows, so the layout does not jump as the reader scrolls.
func (q queueContent) window() (first, last int) {
	first = min(q.offset, q.maxOffset())
	return first, min(first+q.visibleRows(), len(q.items))
}

// visibleRows is how many items fit. Every row below the heading holds an item,
// so the count does not change with where the reader has scrolled to.
func (q queueContent) visibleRows() int {
	return max(q.rows-queueHeaderRows, 1)
}

// overflows reports whether the queue is longer than the window.
func (q queueContent) overflows() bool {
	return len(q.items) > q.rows-queueHeaderRows
}

func (q queueContent) maxOffset() int {
	return max(len(q.items)-q.visibleRows(), 0)
}

// arrow is the mark at the right edge of one visible row. It sits on the first
// and the last row of the window, and only where the queue really goes on past
// that edge. Rows carry it rather than lines of their own, so the list keeps
// one height from the top of a queue to the bottom.
func (q queueContent) arrow(first, last, row int) string {
	switch {
	case row == 0 && first > 0:
		return "↑"
	case row == last-first-1 && last < len(q.items):
		return "↓"
	default:
		return " "
	}
}

// row draws one queue item: the title and the artist, a marker on the one
// that is playing, and a brief flash on one that just landed from a move.
func (q queueContent) row(item sonos.QueueItem, index int, picked bool, title, artist int, arrow string) string {
	// No cursor while the filter box has the keyboard. It would point at a
	// row the reader's keys are not reaching, and move under them as the
	// list narrows on every letter.
	mark := " "
	if index == q.cursor && !q.filtering {
		mark = queueCursorGlyph
	}

	body := fmt.Sprintf("%s %s  %s", mark,
		padRight(clip(item.Title, title), title),
		clip(item.Artist, artist))

	base := themedRowColor()
	style := queueRowStyle()
	if q.isPlaying(item) {
		base = themedPlayingColor()
		style = queuePlayingStyle()
	}
	if glow := q.highlightGlow(item.URI); glow > 0 {
		style = style.Foreground(highlightColour(base, glow))
	}

	// The selection sits behind whatever the foreground is saying, so a
	// picked row can still be the one playing, or the one that just landed.
	if picked {
		style = style.Background(themedSelectedBackground())
	}

	// A track on its way out is drawn over whatever else the row is saying:
	// it is leaving, so what it was is no longer the useful thing to read.
	//
	// Dimmed and not struck through. lipgloss re-emits the whole style for
	// every grapheme of a struck line -- 1750 bytes and 140 escapes against
	// 92 and 2, measured -- and the list repaints every 50ms while the
	// arrows breathe. The contrast drop is large enough on its own.
	if q.isRemoving(item.Position) {
		style = style.Foreground(themedGoingColor()).Bold(false)
	}

	// The arrow column exists only where the queue overflows, so a short queue
	// draws no trailing space and the modal still shrinks to it. The arrow
	// carries its own colour, which breathes, so it is styled apart from the row.
	if !q.overflows() {
		return style.Render(body)
	}

	full := queueMarkWidth + title + queueGap + artist
	return style.Render(padRight(body, full)) +
		strings.Repeat(" ", queueGap) +
		queueArrowStyle(q.openedAt).Render(arrow)
}

// isPlaying guards the zero as well as matching the position. A queue counts
// from one, so zero means nothing is playing from it — and an item that
// somehow carries no position must not answer to that.
func (q queueContent) isPlaying(item sonos.QueueItem) bool {
	return q.playing > 0 && item.Position == q.playing
}

// borderLight is the pulse that answers a press against an end of the list. It
// leaves the centre of that border and runs out to both corners, which is the
// language the notice entrance already speaks. Nil when nothing is lit, so the
// modal draws its own resting colour.
func (q queueContent) borderLight() panes.BorderLight {
	heat := q.heat()
	if heat <= 0 {
		return nil
	}
	progress := 1 - heat

	return func(edge panes.ModalEdge, fromCentre float32) panes.BorderCell {
		if !q.lights(edge) {
			return panes.BorderCell{}
		}

		lit := flashsweep.Spread(fromCentre, progress)
		if lit < bumpVisibleAt {
			return panes.BorderCell{} // the modal keeps its own border here
		}
		return panes.BorderCell{Color: bumpColour(lit), Heavy: lit >= bumpHeavyAt}
	}
}

// lights reports whether a border carries the pulse. The end the reader pressed
// against is the one that answers.
func (q queueContent) lights(edge panes.ModalEdge) bool {
	return (q.edge == bumpTop && edge == panes.ModalTop) ||
		(q.edge == bumpBottom && edge == panes.ModalBottom)
}

// clip shortens to a display width, marking the cut. It walks runes rather
// than bytes: track titles carry accents, and half a rune is not a character.
func clip(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	kept, total := []rune(nil), 0
	for _, r := range s {
		w := lipgloss.Width(string(r))
		if total+w > width-1 {
			break
		}
		kept, total = append(kept, r), total+w
	}
	return string(kept) + "…"
}

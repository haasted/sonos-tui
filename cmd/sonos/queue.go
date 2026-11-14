package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-tui/internal/sonos"
	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// highlightDuration is how long a moved track flashes before fading back
// to normal. Short, like the edge bump: it answers a command that just
// landed, not a state to dwell on.
const highlightDuration = 500 * time.Millisecond

// selectionCountFloor is how many tracks must be picked before the heading
// says how many. Up to it the marks on the rows are the answer and the reader
// has not lost count; a number appearing on the first press is noise.
const selectionCountFloor = 3

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

// landing is where a moved track should be in the queue the speaker sends
// back, and the URI that should be found there. A copy count cannot say it:
// it changes when a track moves past another copy of itself.
type landing struct {
	at  int // 1-based position once the moves are done
	uri string
}

// landingsIn reads where each carried track sits in model, the queue as it
// will be once the moves are done.
func landingsIn(model, carried []int, uriAt map[int]string) []landing {
	landings := make([]landing, 0, len(carried))
	for _, position := range carried {
		landings = append(landings, landing{at: slices.Index(model, position) + 1, uri: uriAt[position]})
	}
	return landings
}

// queueContent lists a speaker's queue inside a modal. It holds what it was
// given and where the reader has scrolled to; it fetches nothing itself.
type queueContent struct {
	listView[sonos.QueueItem]

	room   string
	source sonos.QueueSource // the playlist or album the queue was started from, if known

	// The filter lives on listView. Anything that decides a track's identity
	// reads the whole queue through everything(): nth counts the copies of a
	// URI above a track, and against a filtered list it would change.
	playing int
	err     error
	loading bool

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

	// settling are where the tracks of a move should land, looked for in the
	// queue that comes back. settlingFollow moves the cursor onto the one a
	// J/K shift sent; play next and send to bottom only flash theirs.
	settling       []landing
	settlingFollow bool

	// highlighted are the tracks flashing after a move landed, by copy so a
	// duplicate stays dark. Replaced wholesale, never written to: every copy
	// of this content shares the map.
	highlighted map[selectedTrack]bool
	highlightAt time.Time
}

var _ listContent = queueContent{}

func (queueContent) Init() tea.Cmd { return nil }

// animating reports anything on screen that moves: the arrows breathe while
// the queue overflows, a bumped edge cools, and a moved track fades.
func (q queueContent) animating() bool { return q.overflows() || q.bumping() || q.highlighting() }

// hintReady holds the hint back while the queue is loading, empty or could not
// be read: there are no tracks for the keys to act on.
func (q queueContent) hintReady() bool { return q.note() == "" }

func (q queueContent) resized(rows, width int) tea.Model {
	q.rows, q.width = rows, width
	return q
}

// Update scrolls the list and acts on its tracks. The modal forwards every key
// except esc, so these keys mean the queue here and the player everywhere
// else: j is the previous track outside this list and one row down inside it.
func (q queueContent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		return q.wheel(wheel), nil
	}
	if click, ok := msg.(listClickMsg); ok {
		return q.click(click)
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return q, nil
	}

	// The filter box has the keyboard while it is open: a reader typing a
	// title must not be moving the cursor with every letter of it.
	if q.filtering {
		return q.filterKey(key.String()), nil
	}

	if list, moved := q.navigate(key.String()); moved {
		q.listView = list
		return q, nil
	}

	switch key.String() {
	case "esc":
		return q.closeFilter(), nil
	case "/":
		return q.openFilter(), nil
	case "c":
		q = q.seekPlaying()
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
	case "a":
		return q.markFavourites()
	case "K", "shift+up":
		return q.shift(-1)
	case "J", "shift+down":
		return q.shift(1)
	}
	return q, nil
}

// wheel moves the cursor as the arrow keys do, or through the filter's own
// keys while the filter box has the keyboard.
func (q queueContent) wheel(msg tea.MouseWheelMsg) queueContent {
	var delta int
	if q.listView, delta = q.wheelStep(msg); delta == 0 {
		return q
	}
	if q.filtering {
		if delta < 0 {
			return q.filterKey("up")
		}
		return q.filterKey("down")
	}
	q.listView = q.wheelMove(delta)
	return q
}

// click puts the cursor on the track clicked, and a double-click plays from
// it, as enter does. A click on the heading, or on a note, does nothing.
func (q queueContent) click(msg listClickMsg) (queueContent, tea.Cmd) {
	if q.note() != "" {
		return q, nil
	}
	list, _, double, ok := q.listView.click(msg.line, msg.at)
	if !ok {
		return q, nil
	}
	q.listView = list
	if double {
		return q.playFromCursor()
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
	q.moveGen = nextGen()
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
func (q queueContent) settleMove(to landing) queueContent {
	q.settling, q.settlingFollow = []landing{to}, true
	return q.awaitQueue()
}

// awaitHighlight is settleMove without moving the cursor: the queue that
// comes back is still recognised as this command's own answer, but only the
// flash follows the track, the way playNext wants it.
func (q queueContent) awaitHighlight(landings ...landing) queueContent {
	q.settling, q.settlingFollow = landings, false
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
	q, landings := q.clearSettling()
	positions := q.landedAt(landings)
	if len(positions) == 0 {
		return q
	}

	// Only ever one: J and K commit a single track.
	for i, item := range q.items {
		if item.Position == positions[0] {
			q.cursor = i
			return q.follow()
		}
	}
	return q
}

// landedAt finds the position each landing came to rest at: where it was sent,
// if that holds the URI it carried, and otherwise the first copy of that URI,
// since the queue was changed elsewhere in the meantime.
func (q queueContent) landedAt(landings []landing) []int {
	all := q.everything()

	var positions []int
	for _, l := range landings {
		if i := l.at - 1; i >= 0 && i < len(all) && all[i].URI == l.uri {
			positions = append(positions, all[i].Position)
			continue
		}
		if i := slices.IndexFunc(all, func(item sonos.QueueItem) bool { return item.URI == l.uri }); i >= 0 {
			positions = append(positions, all[i].Position)
		}
	}
	return positions
}

// highlightSettled flashes the tracks a played-next move sent away, now that
// the speaker has said where they landed, and leaves the cursor on a track
// that did not move -- see playNext for why it does not follow them.
func (q queueContent) highlightSettled() queueContent {
	q, landings := q.clearSettling()
	positions := q.landedAt(landings)
	if len(positions) == 0 {
		return q
	}

	moved := make(map[int]bool, len(positions))
	for _, position := range positions {
		moved[position] = true
	}
	tracks := make(map[selectedTrack]bool, len(positions))
	q.eachTrack(func(position int, track selectedTrack) {
		if moved[position] {
			tracks[track] = true
		}
	})

	q = q.restOn(moved)
	return q.highlight(tracks)
}

// restOn puts the cursor on the nearest track that did NOT move, looking
// down first and then up.
//
// The tracks the reader sent away are now somewhere else, and a cursor left
// sitting on one would have followed them there without being asked. Looking
// down first keeps it near the rows they were working through.
func (q queueContent) restOn(moved map[int]bool) queueContent {
	if q.cursor >= len(q.items) || !moved[q.items[q.cursor].Position] {
		return q.follow()
	}

	for i := q.cursor + 1; i < len(q.items); i++ {
		if !moved[q.items[i].Position] {
			q.cursor = i
			return q.follow()
		}
	}
	for i := q.cursor - 1; i >= 0; i-- {
		if !moved[q.items[i].Position] {
			q.cursor = i
			return q.follow()
		}
	}
	return q.follow() // every track moved; there is nowhere untouched to go
}

// clearSettling closes the wait, returning the updated content alongside the
// track it was waiting for, or empty if none was.
func (q queueContent) clearSettling() (queueContent, []landing) {
	landings := q.settling
	q.settling, q.settlingFollow = nil, false
	q = q.clearAwaiting()
	return q, landings
}

// highlight marks the tracks that just landed, to flash briefly the next
// time the queue holding them is drawn. The map is taken as given and never
// written to afterwards -- see the field.
func (q queueContent) highlight(tracks map[selectedTrack]bool) queueContent {
	q.highlighted, q.highlightAt = tracks, time.Now()
	return q
}

// highlightGlow is how lit track's flash is, from 1 just after landing to 0
// once highlightDuration has passed, and for any other track.
// highlightHold is the share of highlightDuration the flash stays at full
// glow before it starts to fade. A pulse that starts fading on its very
// first frame is only ever seen mid-fade -- held here for a beat first, so a
// reader glancing right after the queue lands still catches it fully lit.
const highlightHold = 0.35

func (q queueContent) highlightGlow(track selectedTrack) float32 {
	if !q.highlighted[track] {
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
	return len(q.highlighted) > 0 && time.Since(q.highlightAt) < highlightDuration
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
	return anyContains(filter, item.Title, item.Artist)
}

// queueFilter matches a track by title or artist, and keeps the cursor on a
// track by its URI.
var queueFilter = listFilter[sonos.QueueItem]{
	match: matchesFilter,
	same:  func(a, b sonos.QueueItem) bool { return a.URI == b.URI },
}

// applyFilter, narrow, filterKey, openFilter, closeFilter and typeFilter are
// the list's own filter with the queue's rule, kept on the queue so a chain
// of queue methods stays one type.
func (q queueContent) applyFilter() queueContent {
	q.listView = q.listView.applyFilter(queueFilter)
	return q
}

// narrow must see a queue arriving from the speaker: assigning items straight
// from it would widen the list behind a filter that is still on screen.
func (q queueContent) narrow() queueContent {
	q.listView = q.listView.narrow(queueFilter)
	return q
}

func (q queueContent) filterKey(key string) queueContent {
	q.listView = q.listView.filterKey(queueFilter, key)
	return q
}

func (q queueContent) openFilter() queueContent {
	q.listView = q.listView.openFilter(queueFilter)
	return q
}

// closeFilter shows the whole queue again. What the reader did while it was
// narrowed stays done: the selection is held by track, and tracks on their
// way out by position, neither of which the filter ever touched.
func (q queueContent) closeFilter() queueContent {
	q.listView = q.listView.closeFilter(queueFilter)
	return q
}

func (q queueContent) typeFilter(key string) queueContent {
	q.listView = q.listView.typeFilter(queueFilter, key)
	return q
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
	q.removeGen = nextGen()
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

func (q queueContent) playNextMoves(positions []int) ([]queueMove, []landing) {
	model, uriAt := q.queueModel()

	wanted := make([]int, 0, len(positions))
	for _, position := range positions {
		if position == q.playing {
			continue // it cannot be next of itself
		}
		wanted = append(wanted, position)
	}

	var moves []queueMove
	var carried []int
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
		carried = append(carried, position)

		model = slices.Delete(model, from, from+1)
		model = slices.Insert(model, to, position)
	}
	return moves, landingsIn(model, carried, uriAt)
}

// sendToBottomMoves works out the moves that put tracks at the end of the
// queue, in the order given.
//
// FORWARD through them, where playNextMoves runs backward: each track is
// sent to the end, so the one sent first ends up above the one sent after
// it and the run reads in the order the reader picked them. Aimed at the
// end both times, as playNextMoves aims at the one place after the playing
// track, so only the target differs.
func (q queueContent) sendToBottomMoves(positions []int) ([]queueMove, []landing) {
	model, uriAt := q.queueModel()

	var moves []queueMove
	var carried []int
	for _, position := range positions {
		from := slices.Index(model, position)
		last := len(model) - 1
		if from < 0 || from == last {
			continue // gone from the queue, or already the bottom track
		}

		moves = append(moves, queueMove{from: from + 1, to: last + 1})
		carried = append(carried, position)

		model = slices.Delete(model, from, from+1)
		model = slices.Insert(model, last, position)
	}
	return moves, landingsIn(model, carried, uriAt)
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

// markFavourites asks for the picked tracks to be marked as favourites, in
// the order they were picked, as n and N use; with none picked, the track
// under the cursor, or a choice between it and the queue's playlist or album.
// The selection is cleared, as the queue's other keys do.
func (q queueContent) markFavourites() (queueContent, tea.Cmd) {
	var tracks []sonos.TrackMetadata
	if positions := q.pickedInPressOrder(); len(positions) > 0 {
		for _, position := range positions {
			if at := slices.IndexFunc(q.items, func(item sonos.QueueItem) bool { return item.Position == position }); at >= 0 {
				tracks = append(tracks, q.items[at].TrackMetadata)
			}
		}
		q = q.clearSelection()
	} else if q.cursor < len(q.items) {
		// The playlist or album the queue started from can be marked too,
		// and a chooser asks which; with nothing else to offer, no chooser.
		track := q.items[q.cursor].TrackMetadata
		if markableSource(q.source) {
			source := withAlbumArtist(q.source, q.everything())
			return q, func() tea.Msg { return favouriteChooseMsg{track: track, source: source} }
		}
		tracks = []sonos.TrackMetadata{track}
	}
	if len(tracks) == 0 {
		return q, nil
	}
	return q, func() tea.Msg { return favouriteMarkMsg{tracks: tracks} }
}

// moveBy, moveToEnd, follow and bump are the list's own, kept on the queue so
// a chain of queue methods stays one type.
func (q queueContent) moveBy(delta int) queueContent {
	q.listView = q.listView.moveBy(delta)
	return q
}

func (q queueContent) moveToEnd(edge bumpEdge) queueContent {
	q.listView = q.listView.moveToEnd(edge)
	return q
}

func (q queueContent) follow() queueContent {
	q.listView = q.listView.follow()
	return q
}

func (q queueContent) bump(edge bumpEdge) queueContent {
	q.listView = q.listView.bump(edge)
	return q
}

// jumpToPlaying moves the cursor to the track now playing and lands the
// window on it. Pausing or muting does not change which track that is; only
// stopping the queue does.
func (q queueContent) jumpToPlaying() queueContent {
	row, ok := q.playingRow()
	if !ok {
		return q
	}
	q.cursor = row
	q.listView = q.listView.land()
	return q
}

// seekPlaying is what c does: it moves the cursor to the track now playing,
// and leaves the window where it is when that track is already on screen.
func (q queueContent) seekPlaying() queueContent {
	row, ok := q.playingRow()
	if !ok {
		return q
	}
	if first, last := q.window(); row >= first && row < last {
		q.cursor = row
		return q
	}
	return q.jumpToPlaying()
}

// playingRow is the row of the track now playing, if the list holds it.
func (q queueContent) playingRow() (int, bool) {
	if q.playing <= 0 {
		return 0, false
	}
	for i, item := range q.items {
		if item.Position == q.playing {
			return i, true
		}
	}
	return 0, false
}

func (q queueContent) View() tea.View {
	// Resolved once for the whole pass: which copy of a repeated URI a row is
	// depends on every row above it, so asking per row would walk the queue
	// again for each one. Keyed by the speaker's own position.
	picked := q.pickedPositions()
	title, artist := q.columns(
		func(item sonos.QueueItem) string { return item.Title },
		func(item sonos.QueueItem) string { return item.Artist })

	return tea.NewView(q.render(q.headingParts(), q.note(), func(index int, item sonos.QueueItem, arrow string) string {
		return q.row(item, index, picked[item.Position], [2]int{title, artist}, arrow)
	}))
}

// heading names the room and the size of its queue, so a reader knows what
// they are looking at and how much of it they can see, and says how many
// tracks are picked when any are. The count is here because the rows alone
// cannot carry it once the reader scrolls away from what they picked.
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
	parts := []headingPart{{text: q.room, style: queueRoomStyle()}}

	// The key that opens the box, kept on screen while it is open so a reader
	// who has not typed yet can see that they may. Drawn quieter than what
	// they type into it: it is the prompt, not the search.
	if q.filtering {
		parts = append(parts,
			headingPart{text: " · ", style: queueRoomStyle()},
			headingPart{text: "/", style: queueFilterPromptStyle()},
			headingPart{text: q.filter, style: queueRoomStyle()})
	}

	// How much of the queue is showing, or how big it is. Never both: a
	// narrowed list's own length is not a thing the reader asked about.
	if q.filter != "" {
		parts = append(parts, headingPart{text: " · " + q.matchLabel(), style: queueRoomStyle()})
	} else {
		parts = append(parts, headingPart{text: q.sizeSuffix(), style: queueRoomStyle()})
	}

	picked := q.selectionCount()
	if picked > selectionCountFloor {
		parts = append(parts, headingPart{text: fmt.Sprintf(" · %d selected", picked), style: queueCountStyle()})
	}

	// What the queue was loaded from, while nothing temporary -- a search or a
	// selection -- needs the heading. It gives way first when space runs out.
	if q.source.Title != "" && !q.filtering && q.filter == "" && picked <= selectionCountFloor {
		parts = append(parts, headingPart{text: headingSeparator + q.source.Title, style: queueRoomStyle(), shrink: true})
	}
	return fitHeading(parts, q.width)
}

// sizeSuffix says how big the queue is, and nothing at all while it is being
// read or is empty -- the note below the rule says that better.
func (q queueContent) sizeSuffix() string {
	switch {
	case q.loading, len(q.items) == 0:
		return ""
	case q.overflows():
		return " · " + q.rangeLabel()
	case len(q.items) == 1:
		return " · 1 track"
	default:
		return fmt.Sprintf(" · %d tracks", len(q.items))
	}
}

// note is the whole body when there are no rows to draw, and empty otherwise.
func (q queueContent) note() string {
	switch {
	case q.loading:
		return "Reading the queue…"
	case q.err != nil:
		return "Could not read the queue: " + faultText(q.err)
	case len(q.items) == 0:
		return "The queue is empty."
	default:
		return ""
	}
}

// row draws one queue item: the title and the artist, coloured for the one
// that is playing, and a brief flash on one that just landed from a move.
func (q queueContent) row(item sonos.QueueItem, index int, picked bool, widths [2]int, arrow string) string {
	base := themedRowColor()
	style := queueRowStyle()
	if q.isPlaying(item) {
		base = themedPlayingColor()
		style = queuePlayingStyle()
	}
	if q.highlighting() {
		if glow := q.highlightGlow(q.trackOf(item)); glow > 0 {
			style = style.Foreground(highlightColour(base, glow))
		}
	}

	// The selection sits behind whatever the foreground is saying, so a
	// picked row can still be the one playing, or the one that just landed.
	if picked {
		style = style.Background(themedSelectedBackground())
	}

	// A track on its way out is drawn over whatever else the row is saying.
	// Dimmed and not struck through: lipgloss re-emits the whole style for
	// every grapheme of a struck line, and the list repaints every 50ms.
	if q.isRemoving(item.Position) {
		style = style.Foreground(themedGoingColor()).Bold(false)
	}

	// No cursor while the filter box has the keyboard. It would point at a
	// row the reader's keys are not reaching.
	return q.drawRow(index, item.Title, item.Artist, widths, style, arrow, q.filtering)
}

// isPlaying guards the zero as well as matching the position. A queue counts
// from one, so zero means nothing is playing from it — and an item that
// somehow carries no position must not answer to that.
func (q queueContent) isPlaying(item sonos.QueueItem) bool {
	return q.playing > 0 && item.Position == q.playing
}

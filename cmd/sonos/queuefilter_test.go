package main

import (
	"strings"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: / narrowing the list as the reader types, matching the title and the
// artist alike and caring about neither's case.
func TestQueueFilter_NarrowsOnTitleAndArtist(t *testing.T) {
	for _, typed := range []string{"nick", "NICK", "cave"} {
		q := typeInto(mixedQueue(), typed)

		if len(q.items) != 2 {
			t.Fatalf("%q showed %d tracks, want the two by Nick Cave", typed, len(q.items))
		}
	}

	q := typeInto(mixedQueue(), "horses")
	if len(q.items) != 1 || q.items[0].Title != "Bright Horses" {
		t.Fatalf("a title match showed %d tracks", len(q.items))
	}
}

// Case: the heading, which must say the list is narrowed. Its count is of
// the rows on screen, and would otherwise read as the whole queue.
func TestQueueFilter_TheHeadingSaysSo(t *testing.T) {
	typing := typeInto(mixedQueue(), "nick")

	head := typing.heading()
	if !strings.Contains(head, "nick") {
		t.Fatalf("the reader cannot read back what they are typing: %q", head)
	}
	if !strings.Contains(head, "/nick") {
		t.Fatalf("the heading does not show the box the reader is typing into: %q", head)
	}
	if !strings.Contains(head, "2 of 4") {
		t.Fatalf("the heading does not say how much of the queue is showing: %q", head)
	}

	// Once the box has gone the term goes with it -- it was what the reader
	// typed, not what the list is -- but the count must still say it is a part.
	closed := moveQueue(typing, "enter").heading()
	if strings.Contains(closed, "nick") {
		t.Fatalf("the search term outlived the box it was typed into: %q", closed)
	}
	if !strings.Contains(closed, "2 of 4") {
		t.Fatalf("the heading reads %q, which will be taken for the whole queue", closed)
	}
}

// Case: esc, which the modal takes for itself unless the content wants it.
// The first clears the filter; only then does the modal get it.
func TestQueueFilter_EscClearsBeforeTheModalCloses(t *testing.T) {
	q := typeInto(mixedQueue(), "nick")
	if !q.ClaimsEsc() {
		t.Fatal("a narrowed list let esc through to close the modal")
	}

	cleared := moveQueue(q, "esc")
	if len(cleared.items) != 4 {
		t.Fatalf("esc left %d tracks, want the whole queue back", len(cleared.items))
	}
	if cleared.ClaimsEsc() {
		t.Fatal("a second esc would be claimed again rather than closing the modal")
	}
}

// Case: THE POINT OF THE FEATURE. Narrow, pick out what is shown, clear the
// filter, and the picks are still there to act on.
func TestQueueFilter_PicksSurviveClearingIt(t *testing.T) {
	// enter leaves the box, so the keys that act on tracks reach the rows it
	// narrowed to rather than being typed into it.
	q := moveQueue(typeInto(mixedQueue(), "nick"), "enter")
	q = moveQueue(q, "*") // every Nick Cave track

	if q.selectionCount() != 2 {
		t.Fatalf("%d tracks picked out of the two shown", q.selectionCount())
	}

	cleared := moveQueue(q, "esc")
	if cleared.selectionCount() != 2 {
		t.Fatalf("%d tracks are picked once the filter went, want the same two",
			cleared.selectionCount())
	}
	for _, position := range cleared.selectedPositions() {
		if artist := cleared.items[position-1].Artist; artist != "Nick Cave" {
			t.Fatalf("position %d is by %q, so the picks moved to other tracks", position, artist)
		}
	}
}

// Case: inverting while narrowed, which must reach only what is shown. A
// reader cannot mean tracks the filter is hiding from them.
func TestQueueFilter_InvertReachesOnlyWhatIsShown(t *testing.T) {
	q := moveQueue(typeInto(mixedQueue(), "nick"), "enter")

	q = moveQueue(q, "*")
	cleared := moveQueue(q, "esc")

	if cleared.selectionCount() != 2 {
		t.Fatalf("invert picked %d tracks; only the two shown were its business",
			cleared.selectionCount())
	}
}

// Case: typing while the box is open, which must not also be moving the
// cursor. A title holds j and k like any other letters.
func TestQueueFilter_TypingDoesNotMoveTheCursor(t *testing.T) {
	q := typeInto(mixedQueue(), "j")

	if q.filter != "j" {
		t.Fatalf("the filter reads %q; j moved the cursor instead of being typed", q.filter)
	}
}

// Case: enter, which leaves the box but keeps the narrowing, so the keys
// that act on tracks reach the rows it left.
func TestQueueFilter_EnterLeavesTheBoxAndKeepsTheRows(t *testing.T) {
	q := moveQueue(typeInto(mixedQueue(), "nick"), "enter")

	if q.filtering {
		t.Fatal("enter left the filter box open")
	}
	if len(q.items) != 2 {
		t.Fatalf("enter showed %d tracks again, want the narrowing kept", len(q.items))
	}
}

// Case: backspace, widening the list again a letter at a time.
func TestQueueFilter_BackspaceWidensIt(t *testing.T) {
	q := moveQueue(typeInto(mixedQueue(), "nick"), "backspace")

	if q.filter != "nic" {
		t.Fatalf("the filter reads %q after one backspace", q.filter)
	}
}

// mixedQueue is four tracks by two artists, for cases about what a filter
// keeps and what it hides.
func mixedQueue() queueContent {
	tracks := []struct{ title, artist string }{
		{"Bright Horses", "Nick Cave"},
		{"Waiting for You", "Nick Cave"},
		{"Night Raid", "Sleaford Mods"},
		{"Jolly Fucker", "Sleaford Mods"},
	}

	items := make([]sonos.QueueItem, 0, len(tracks))
	for i, track := range tracks {
		items = append(items, sonos.QueueItem{
			Position: i + 1,
			TrackMetadata: sonos.TrackMetadata{
				Title: track.title, Artist: track.artist,
				URI: "x-sonos-http:track-" + string(rune('a'+i)) + ".mp3",
			},
		})
	}
	return queueContent{listView: listView[sonos.QueueItem]{items: items, rows: 12, width: 70}, room: "Køkken"}
}

// typeInto opens the filter and types, a key at a time, as a reader would.
func typeInto(q queueContent, text string) queueContent {
	q = moveQueue(q, "/")
	for _, r := range text {
		q = moveQueue(q, string(r))
	}
	return q
}

// Case: down, which must leave the box exactly as enter does. Reaching for
// the list is the reader saying they are done typing.
func TestQueueFilter_DownLeavesTheBoxLikeEnter(t *testing.T) {
	byDown := moveQueue(typeInto(mixedQueue(), "nick"), "down")
	byEnter := moveQueue(typeInto(mixedQueue(), "nick"), "enter")

	if byDown.filtering {
		t.Fatal("down left the filter box open")
	}
	if byDown.filter != byEnter.filter || len(byDown.items) != len(byEnter.items) {
		t.Fatalf("down gave %q with %d rows, enter gave %q with %d",
			byDown.filter, len(byDown.items), byEnter.filter, len(byEnter.items))
	}
	if byDown.cursor != byEnter.cursor {
		t.Fatalf("down left the cursor on row %d, enter on row %d", byDown.cursor, byEnter.cursor)
	}
}

// Case: the cursor while the box has the keyboard. It must not be drawn --
// it points at a row the reader's keys are not reaching, and it would move
// under them with every letter as the list narrows.
func TestQueueFilter_NoCursorUntilTheListHasTheKeys(t *testing.T) {
	typing := typeInto(mixedQueue(), "nick")
	if strings.Contains(plain(typing.View().Content), listCursorGlyph) {
		t.Fatal("the cursor is drawn while the reader is still typing")
	}

	inList := moveQueue(typing, "down")
	if !strings.Contains(plain(inList.View().Content), listCursorGlyph) {
		t.Fatal("the cursor did not come back once the list had the keys")
	}
}

// Case: a queue arriving from the speaker while a filter is on screen. It
// must be narrowed on the way in: widening the list while the heading still
// says /nick shows the reader a filter that is not doing anything.
func TestQueueFilter_AQueueArrivingIsNarrowedToo(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	model, _ := m.Update(keyMsgFor("enter"))
	m = model.(appModel)
	speaker, _ := m.activeSpeaker()

	for _, key := range []string{"/", "1"} { // Track 1, Track 10 … Track 19
		model, _ = m.Update(keyMsgFor(key))
		m = model.(appModel)
	}
	narrowed := len(queueOf(t, m).items)
	if narrowed == 20 {
		t.Fatal("the filter narrowed nothing, so this proves nothing")
	}

	model, _ = m.Update(queueLoadedMsg{
		device: speaker.device, room: speaker.device.RoomName(),
		items: queueItems(20), playing: 1,
	})
	content := queueOf(t, model.(appModel))

	if len(content.items) != narrowed {
		t.Fatalf("the queue came back and widened the list to %d rows, want %d kept",
			len(content.items), narrowed)
	}
	if !strings.Contains(content.heading(), "of 20") {
		t.Fatalf("the heading no longer says the list is a part: %q", content.heading())
	}
}

// Case: opening the filter again after one has been used. It must start
// empty: a box holding the last search narrows the list before the reader
// has asked for anything.
func TestQueueFilter_OpensEmptyEachTime(t *testing.T) {
	q := moveQueue(typeInto(mixedQueue(), "nick"), "enter")
	if q.filter != "nick" {
		t.Fatal("the first filter did not take, so this proves nothing")
	}

	again := moveQueue(q, "/")

	if again.filter != "" {
		t.Fatalf("the box opened holding %q", again.filter)
	}
	if len(again.items) != 4 {
		t.Fatalf("opening the box left %d rows, want the whole queue back", len(again.items))
	}
	if strings.Contains(again.heading(), "nick") {
		t.Fatalf("the heading still names the old search: %q", again.heading())
	}
	if strings.Contains(again.heading(), " of ") {
		t.Fatalf("the heading says the list is a part when nothing is narrowing it: %q",
			again.heading())
	}
}

// Case: the box open with nothing typed yet. The key that opened it stays on
// screen, or a reader who has not started has nothing telling them they may.
func TestQueueFilter_ThePromptShowsBeforeAnythingIsTyped(t *testing.T) {
	opened := moveQueue(mixedQueue(), "/")

	if !strings.Contains(opened.heading(), "/") {
		t.Fatalf("an open box says nothing to the reader: %q", opened.heading())
	}
	if strings.Contains(opened.heading(), " of ") {
		t.Fatalf("an empty box narrows nothing, so the heading must not say it does: %q",
			opened.heading())
	}
}

// Case: the prompt against what is typed into it. They must be drawn apart,
// or the key the reader pressed reads as part of their search.
func TestQueueFilter_ThePromptIsNotDrawnLikeTheSearch(t *testing.T) {
	line := strings.Split(typeInto(mixedQueue(), "nick").View().Content, "\n")[0]

	if styleOf(line, "/") == styleOf(line, "nick") {
		t.Fatal("the prompt is drawn in the same style as the term typed after it")
	}
}

// Case: the prompt going with the box. Once it has closed there is nothing
// to type into, so a key suggesting otherwise would be a lie.
func TestQueueFilter_ThePromptGoesWithTheBox(t *testing.T) {
	closed := moveQueue(typeInto(mixedQueue(), "nick"), "enter")

	if strings.Contains(closed.heading(), "/") {
		t.Fatalf("the prompt outlived the box: %q", closed.heading())
	}
}

// Case: a letter while the filter box has the keyboard. It belongs to what
// the reader is typing, and no command may take it first -- "Waiting for
// You" is a track in this very queue. Nothing binds w above the modal today;
// this is here for whatever is bound next.
func TestQueueFilter_ALetterTypesWhileTheBoxIsOpen(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)
	model, _ := m.Update(keyMsgFor("enter"))
	m = model.(appModel)

	for _, key := range []string{"/", "w"} {
		model, _ = m.Update(keyMsgFor(key))
		m = model.(appModel)
	}

	if got := queueOf(t, m).filter; got != "w" {
		t.Fatalf("the filter reads %q; a command took the letter instead", got)
	}
}

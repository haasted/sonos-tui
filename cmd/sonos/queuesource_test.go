package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: a queue loaded from a playlist, with room to spare. The heading says
// where the reader is, how much is showing, and what the queue came from.
func TestQueueSource_TheHeadingNamesIt(t *testing.T) {
	q := sourcedQueue("Awesome singles")

	heading := headingOf(q)

	if !strings.HasSuffix(heading, " · Awesome singles") || !strings.HasPrefix(heading, "Køkken · ") {
		t.Fatalf("heading is %q, want the room, the range, then the playlist", heading)
	}
}

// Case: the temporary states, which need the heading for themselves. A search
// open or applied, or a selection being counted, hides the name.
func TestQueueSource_GivesWayToSearchAndSelection(t *testing.T) {
	cases := map[string]queueContent{
		"filter box open":    sourcedQueue("Awesome singles").openFilter(),
		"filter applied":     filtered(sourcedQueue("Awesome singles"), "track 1"),
		"selection counting": picked(sourcedQueue("Awesome singles"), selectionCountFloor+1),
	}
	for name, q := range cases {
		if heading := headingOf(q); strings.Contains(heading, "Awesome singles") {
			t.Errorf("%s: the name stayed in the heading: %q", name, heading)
		}
	}
}

// Case: a box too narrow for the whole name. It is clipped with an ellipsis,
// and the room and the range keep every character.
func TestQueueSource_ANarrowBoxClipsTheName(t *testing.T) {
	q := sourcedQueue("Awesome singles")
	q.width = baseWidth(q) + lipgloss.Width(headingSeparator) + 9

	heading := headingOf(q)

	if !strings.HasSuffix(heading, "…") || !strings.Contains(heading, " · Awesome") {
		t.Fatalf("heading is %q, want the name clipped with an ellipsis", heading)
	}
	if lipgloss.Width(heading) > q.width {
		t.Fatalf("the heading is %d wide in a box %d wide", lipgloss.Width(heading), q.width)
	}
}

// Case: a box with room for only a scrap of the name. "Aw…" says nothing, so
// the name goes and the rest of the heading is as it would be without one.
func TestQueueSource_TooLittleRoomDropsTheName(t *testing.T) {
	q := sourcedQueue("Awesome singles")
	q.width = baseWidth(q) + lipgloss.Width(headingSeparator) + headingShortest

	heading := headingOf(q)

	without := q
	without.source = sonos.QueueSource{}
	if heading != headingOf(without) {
		t.Fatalf("heading is %q, want it as if there were no name: %q", heading, headingOf(without))
	}
}

// Case: the whole path, from a real capture to the open queue. The speaker
// reports the playlist, the program keeps it, and the queue's heading shows it.
func TestQueueSource_ACapturedPlaylistReachesTheHeading(t *testing.T) {
	m, device, _ := withQueueServer(t, 40)
	body, err := os.ReadFile("../../internal/sonos/testdata/avtransport_queue_playlist.xml")
	if err != nil {
		t.Fatal(err)
	}
	if err := device.HandleAVTransportEvent(context.Background(), body); err != nil {
		t.Fatal(err)
	}
	<-device.Events()

	m = pressKey(resize(m), "enter")

	if heading := headingOf(queueOf(t, m)); !strings.Contains(heading, "Awesome singles") {
		t.Fatalf("the open queue's heading is %q, without the playlist", heading)
	}
}

func sourcedQueue(source string) queueContent {
	q := navQueue(40, 0)
	q.source = sonos.QueueSource{Title: source}
	return q
}

func filtered(q queueContent, text string) queueContent {
	q = q.openFilter()
	for _, r := range text {
		q = q.filterKey(string(r))
	}
	return q.filterKey("enter") // keeps the filter, hands the keys to the list
}

func picked(q queueContent, n int) queueContent {
	for range n {
		q = moveQueue(q, "s")
	}
	return q
}

// headingOf is the heading as plain text, its styled runs joined.
func headingOf(q queueContent) string {
	var b strings.Builder
	for _, part := range q.headingParts() {
		b.WriteString(part.text)
	}
	return b.String()
}

// baseWidth is how wide the heading is without a name in it.
func baseWidth(q queueContent) int {
	q.source = sonos.QueueSource{}
	return lipgloss.Width(headingOf(q))
}

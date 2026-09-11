package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// Case: the trigger and the dismissal, which are the whole feature at its
// coarsest. enter opens the queue, esc puts it away.
func TestQueue_EnterOpensAndEscCloses(t *testing.T) {
	m := pressKey(sizedModel(t), "enter")
	if !m.queue.IsOpen() {
		t.Fatal("enter did not open the queue modal")
	}

	m = pressKey(m, "esc")
	if m.queue.IsOpen() {
		t.Fatal("esc did not close the queue modal")
	}
}

// Case: the arrow keys, which two things want. While the queue is open they
// scroll it; once it closes they must move the volume again.
func TestQueue_ArrowsScrollTheListAndNotTheVolume(t *testing.T) {
	m := openWithQueue(t, 40)
	before := pendingVolume(t, m)

	m = pressKey(pressKey(m, "down"), "down")

	if got := queueOf(t, m).offset; got != 2 {
		t.Fatalf("two down presses scrolled to %d, want 2", got)
	}
	if got := pendingVolume(t, m); got != before {
		t.Fatalf("scrolling the queue moved the volume by %+d", got-before)
	}

	m = pressKey(pressKey(m, "esc"), "down")
	if got := pendingVolume(t, m); got == before {
		t.Fatal("the volume keys did not come back after the modal closed")
	}
}

// Case: a queue far longer than the modal, which must be reachable end to end
// rather than showing only its first screen.
func TestQueue_LongQueueIsFullyReachable(t *testing.T) {
	const total = 40
	m := openWithQueue(t, total)

	if strings.Contains(plainQueue(t, m), "Track 40") {
		t.Fatal("a 40-item queue fits the modal, so this test proves nothing")
	}

	for range total {
		m = pressKey(m, "down")
	}

	if !strings.Contains(plainQueue(t, m), "Track 40") {
		t.Fatal("the last item is not reachable by scrolling")
	}
}

// Case: the reader needs to know that more exists in each direction, since a
// window with no edge markers looks like the whole list.
func TestQueue_ShowsWhatLiesBeyondTheWindow(t *testing.T) {
	m := openWithQueue(t, 40)

	if got := plainQueue(t, m); !strings.Contains(got, "↓") {
		t.Fatalf("nothing indicates more items below:\n%s", got)
	}

	m = pressKey(pressKey(m, "down"), "down")
	if got := plainQueue(t, m); !strings.Contains(got, "↑") {
		t.Fatalf("nothing indicates the items scrolled past above:\n%s", got)
	}
}

// Case: the scroll boundaries, which must hold rather than run off either end.
func TestQueue_ScrollStopsAtBothEnds(t *testing.T) {
	m := openWithQueue(t, 40)

	for range 5 {
		m = pressKey(m, "up")
	}
	if got := queueOf(t, m).offset; got != 0 {
		t.Fatalf("scrolling up from the top reached offset %d, want 0", got)
	}

	for range 100 {
		m = pressKey(m, "down")
	}
	content := queueOf(t, m)
	if got, want := content.offset, content.maxOffset(); got != want {
		t.Fatalf("scrolling past the end reached offset %d, want a stop at %d", got, want)
	}
}

// Case: a speaker playing a station. The queue survives a station starting, so
// it is shown — but DeviceState.Track then counts the stream, and highlighting
// item 1 would point confidently at the wrong track.
func TestQueue_RadioHighlightsNothing(t *testing.T) {
	station := sonos.DeviceState{
		Source: sonos.SourceInfo{URI: "x-sonosapi-hls:dr.p6"},
		Track:  1,
	}
	if got := playingPosition(station); got != 0 {
		t.Fatalf("a station highlighted queue item %d; nothing in the queue is playing", got)
	}

	queue := sonos.DeviceState{
		Source: sonos.SourceInfo{URI: "x-rincon-queue:RINCON_1234#0"},
		Track:  7,
	}
	if got := playingPosition(queue); got != 7 {
		t.Fatalf("a queue source highlighted item %d, want 7", got)
	}
}

// Case: the three bodies with no rows. Each must say what happened rather than
// render an empty box the reader has to interpret.
func TestQueue_SaysWhyThereAreNoRows(t *testing.T) {
	cases := map[string]queueContent{
		"Reading":   {room: "Køkken", loading: true, rows: 14},
		"empty":     {room: "Køkken", rows: 14},
		"Could not": {room: "Køkken", err: fmt.Errorf("connection refused"), rows: 14},
	}

	for want, content := range cases {
		if got := plain(content.View().Content); !strings.Contains(got, want) {
			t.Errorf("body %q does not mention %q", got, want)
		}
	}
}

// Case: a result arriving for a speaker the reader has since left. It must be
// dropped rather than listed under the wrong room name.
func TestQueue_DropsAResultForAnotherSpeaker(t *testing.T) {
	m := pressKey(sizedModel(t), "enter")

	other := sonos.NewDevice(sonos.Speaker{IP: "10.0.0.99"})
	updated, _ := m.Update(queueLoadedMsg{
		device: other,
		room:   "Somewhere Else",
		items:  queueItems(3),
	})

	if got := plainQueue(t, updated.(appModel)); strings.Contains(got, "Somewhere Else") {
		t.Fatalf("another speaker's queue was shown:\n%s", got)
	}
}

// openWithQueue opens the modal and delivers a queue of the given length, as
// the fetch would.
func openWithQueue(t *testing.T, items int) appModel {
	t.Helper()

	m := pressKey(sizedModel(t), "enter")
	speaker, ok := m.activeSpeaker()
	if !ok {
		t.Fatal("no speaker on screen to read a queue from")
	}

	updated, _ := m.Update(queueLoadedMsg{
		device:  speaker.device,
		room:    speaker.device.DisplayName(),
		items:   queueItems(items),
		playing: 1,
	})
	return updated.(appModel)
}

func queueItems(n int) []sonos.QueueItem {
	items := make([]sonos.QueueItem, 0, n)
	for i := 1; i <= n; i++ {
		items = append(items, sonos.QueueItem{
			Position: i,
			TrackMetadata: sonos.TrackMetadata{
				Title:    fmt.Sprintf("Track %d", i),
				Artist:   "An Artist",
				Duration: "3:45",
			},
		})
	}
	return items
}

func queueOf(t *testing.T, m appModel) queueContent {
	t.Helper()

	content, ok := m.queue.Content.(queueContent)
	if !ok {
		t.Fatal("the modal is not holding a queue")
	}
	return content
}

func plainQueue(t *testing.T, m appModel) string {
	t.Helper()
	return plain(queueOf(t, m).View().Content)
}

func pendingVolume(t *testing.T, m appModel) int {
	t.Helper()

	speaker, ok := m.activeSpeaker()
	if !ok {
		t.Fatal("no speaker on screen")
	}
	return speaker.pendingVolume
}

var queueANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(s string) string { return queueANSI.ReplaceAllString(s, "") }

// Case: an item carrying no position, against a queue where nothing plays.
// Both are zero, and a plain equality would mark the row as playing.
func TestQueue_NothingPlayingMarksNoRow(t *testing.T) {
	q := queueContent{
		room:  "Ovenpå",
		items: []sonos.QueueItem{{TrackMetadata: sonos.TrackMetadata{Title: "A Track"}}},
		rows:  14,
		width: 70,
	}

	if got := plain(q.View().Content); strings.Contains(got, "▶") {
		t.Fatalf("a row is marked as playing when nothing is:\n%s", got)
	}
}

// Case: two columns that both fit, which must not be shortened at all. This is
// the reported complaint: a one-track queue truncated an artist with room to
// spare.
func TestQueue_KeepsBothColumnsWholeWhenTheyFit(t *testing.T) {
	const artist = "John Cxnnor & The Doom Squad"

	q := queueContent{
		room:  "Soveværelse",
		items: []sonos.QueueItem{{Position: 1, TrackMetadata: sonos.TrackMetadata{Title: "HellWalkers", Artist: artist}}},
		rows:  14,
		width: 70,
	}

	body := plain(q.View().Content)
	if !strings.Contains(body, artist) {
		t.Fatalf("the artist was shortened with room to spare:\n%s", body)
	}

	// And the row takes only what it needs. Padding both columns out to the
	// modal's full allowance would keep the text whole but leave a wide, mostly
	// empty box around one track.
	want := queueMarkWidth + lipgloss.Width("HellWalkers") + queueGap + lipgloss.Width(artist)
	if got := lipgloss.Width(strings.Split(body, "\n")[2]); got != want {
		t.Fatalf("the row spans %d columns for %d columns of content", got, want)
	}
}

// Case: the artist column, which must start in the same place on every row.
// A clipped title used to measure one column wider than a padded one, so the
// long rows sat one step to the right of the short ones.
func TestQueue_ArtistColumnStartsAtOneOffset(t *testing.T) {
	q := queueContent{
		room: "Spisestue",
		items: []sonos.QueueItem{
			{Position: 1, TrackMetadata: sonos.TrackMetadata{Title: "Night Raid", Artist: "Nick Cave"}},
			{Position: 2, TrackMetadata: sonos.TrackMetadata{
				Title: "Idiot Prayer (Live at Alexandra Palace, 2020) and then some more", Artist: "Nick Cave"}},
		},
		rows:  14,
		width: 70,
	}

	// Display columns, not bytes: the ellipsis a clipped title ends on is three
	// bytes wide and one column, so strings.Index alone would report a
	// difference that is not on the screen.
	var offsets []int
	for _, line := range strings.Split(plain(q.View().Content), "\n") {
		if i := strings.Index(line, "Nick Cave"); i > 0 {
			offsets = append(offsets, lipgloss.Width(line[:i]))
		}
	}

	if len(offsets) != 2 {
		t.Fatalf("expected two rows carrying the artist, found %d", len(offsets))
	}
	if offsets[0] != offsets[1] {
		t.Fatalf("the artist starts at column %d on a short title and %d on a clipped one",
			offsets[0], offsets[1])
	}
}

// Case: the height of the list, from the top of a queue to the bottom. A
// reserved line that empties at an edge used to cost the modal a row, so the
// box jumped on the last press.
func TestQueue_KeepsOneHeightWhileScrolling(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(32), playing: 3, rows: 9, width: 70}
	want := lipgloss.Height(q.View().Content)

	for step := range 40 {
		q.offset = step
		if got := lipgloss.Height(q.View().Content); got != want {
			t.Fatalf("the list is %d lines at offset %d, want a steady %d", got, step, want)
		}
	}
}

// Case: the top of the list, which must carry an item rather than a blank line
// held open for a count that is not there.
func TestQueue_TopRowHoldsAnItem(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(32), rows: 9, width: 70}

	first := strings.Split(plain(q.View().Content), "\n")[2]
	if strings.TrimSpace(first) == "" {
		t.Fatal("the first row below the rule is blank")
	}
	if !strings.Contains(first, "Track 1") {
		t.Fatalf("the first row is %q, want the first track", first)
	}
}

// Case: the two marks on one row. The play marker owns the column left of the
// title, so a scroll arrow in that cell would hide it.
func TestQueue_PlayMarkerAndArrowBothFitOnARow(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(32), playing: 3, rows: 9, width: 70, offset: 2}

	row := strings.Split(plain(q.View().Content), "\n")[2]
	if !strings.Contains(row, "▶") || !strings.Contains(row, "↑") {
		t.Fatalf("the playing track at the top of the window lost a mark: %q", row)
	}
}

// Case: the width of the list while the reader scrolls. The heading carries a
// range whose numbers grow, so a list measured against the heading on screen
// would widen as the reader moved down it.
//
// The titles here are one character, which is what makes the heading the widest
// thing in the box. With real titles the rows govern and the fault is hidden.
func TestQueue_KeepsOneWidthWhileScrolling(t *testing.T) {
	items := queueItems(124)
	for i := range items {
		items[i].Title, items[i].Artist = "A", "B"
	}
	q := queueContent{room: "Køkken", items: items, rows: 9, width: 70}

	want := lipgloss.Width(q.View().Content)
	for _, offset := range []int{0, 1, 9, 60, 999} {
		q.offset = offset
		if got := lipgloss.Width(q.View().Content); got != want {
			t.Fatalf("the list is %d columns at offset %d, want a steady %d: %q",
				got, offset, want, q.heading())
		}
	}
}

// Case: the heading itself, which a reader looks at. Padding the two numbers to
// one width holds the box steady but puts a gap either side of the dash, which
// reads as a fault rather than as alignment.
func TestQueue_HeadingRangeHasNoStrayPadding(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(124), rows: 9, width: 70, offset: 3}

	if got := q.heading(); strings.Contains(got, " -") || strings.Contains(got, "- ") {
		t.Fatalf("the heading %q pads around the dash", got)
	}
}

// Case: the two ends of the queue. An arrow at an edge with nothing past it
// promises more than the queue holds.
func TestQueue_NoArrowWhereTheQueueEnds(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(32), rows: 9, width: 70}

	if got := plain(q.View().Content); strings.Contains(got, "↑") {
		t.Fatalf("an up arrow at the top of the queue:\n%s", got)
	}

	q.offset = 99 // clamped to the last window
	if got := plain(q.View().Content); strings.Contains(got, "↓") {
		t.Fatalf("a down arrow at the end of the queue:\n%s", got)
	}
}

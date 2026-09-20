package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
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

	if got := queueOf(t, m).cursor; got != 2 {
		t.Fatalf("two down presses moved the cursor to %d, want 2", got)
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

	// To the end rather than a couple of rows down: the window only moves once
	// the cursor reaches its edge, so two presses scroll nothing.
	m = pressKey(m, "end")
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
		room:    speaker.device.RoomName(),
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
				Title:  fmt.Sprintf("Track %d", i),
				Artist: "An Artist",
				// The URI is what a track is known by once its position has
				// changed, which is the whole of how a move is followed.
				URI:      fmt.Sprintf("x-sonos-http:track-%d.mp3", i),
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
//
// The marker cannot answer this any more: it follows the cursor, which is
// always on some row. Only the playing style, which is the bold half of bold
// and blue, says a row is playing.
func TestQueue_NothingPlayingMarksNoRow(t *testing.T) {
	q := queueContent{
		room:  "Ovenpå",
		items: []sonos.QueueItem{{TrackMetadata: sonos.TrackMetadata{Title: "A Track"}}},
		rows:  14,
		width: 70,
	}

	if anyRowIsPlaying(q) {
		t.Fatal("a row is drawn in the playing style when nothing is playing")
	}

	playing := q
	playing.items[0].Position, playing.playing = 1, 1
	if !anyRowIsPlaying(playing) {
		t.Fatal("the playing row is not drawn in the playing style, so this proves nothing")
	}
}

// anyRowIsPlaying reports whether a list row carries the playing style. It
// looks at the rows alone: the heading above them is bold as well, and the
// marker cannot answer this any more.
//
// lipgloss writes bold and the colour in one escape, so the opening of that
// escape is what to look for rather than a bold code of its own.
func anyRowIsPlaying(q queueContent) bool {
	lines := strings.Split(q.View().Content, "\n")
	if len(lines) <= queueHeaderRows {
		return false
	}
	for _, line := range lines[queueHeaderRows:] {
		if strings.Contains(line, "\x1b[1;") {
			return true
		}
	}
	return false
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

// Case: the two marks on one row. The cursor owns the column left of the title,
// so a scroll arrow in that cell would hide it.
func TestQueue_CursorAndArrowBothFitOnARow(t *testing.T) {
	// The playing track is far down the queue, so the marker on the top row can
	// only have come from the cursor.
	q := queueContent{room: "Køkken", items: queueItems(32), playing: 30,
		rows: 9, width: 70, cursor: 2, offset: 2}

	row := strings.Split(plain(q.View().Content), "\n")[2]
	if !strings.Contains(row, "▶") || !strings.Contains(row, "↑") {
		t.Fatalf("the top row of the window lost a mark: %q", row)
	}
}

// Case: the marker, which follows the cursor and not the playing track. What
// is playing is said by colour alone.
func TestQueue_TheMarkerFollowsTheCursor(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(8), playing: 6, rows: 12, width: 70, cursor: 1}

	rows := strings.Split(plain(q.View().Content), "\n")[queueHeaderRows:]
	if !strings.Contains(rows[1], "▶") {
		t.Fatalf("the cursor row carries no marker: %q", rows[1])
	}
	if strings.Contains(rows[5], "▶") {
		t.Fatalf("the playing row still carries the marker: %q", rows[5])
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

// Case: the breath starting. A queue with an arrow needs a repaint chain, and
// one without an arrow would repaint a still picture.
func TestQueue_TheBreathRunsOnlyWhereThereIsAnArrow(t *testing.T) {
	// The queue arriving is what starts the chain, so the test has to watch the
	// command that delivery returns, not only the one a later tick returns.
	if _, cmd := deliverQueue(t, 40); cmd == nil {
		t.Fatal("a list with an arrow never starts breathing")
	}
	if _, cmd := deliverQueue(t, 2); cmd != nil {
		t.Fatal("a list with no arrow starts breathing anyway")
	}

	long := openWithQueue(t, 40)
	if !queueOf(t, long).overflows() {
		t.Fatal("a 40-item queue should overflow the modal, or this proves nothing")
	}
	if _, cmd := long.Update(queueTickMsg{gen: long.queueGen}); cmd == nil {
		t.Fatal("a list with an arrow stops repainting")
	}

	short := openWithQueue(t, 2)
	if _, cmd := short.Update(queueTickMsg{gen: short.queueGen}); cmd != nil {
		t.Fatal("a list with no arrow repaints anyway")
	}
}

// deliverQueue opens the list and hands it a queue, keeping the command that
// delivery returns.
func deliverQueue(t *testing.T, items int) (appModel, tea.Cmd) {
	t.Helper()

	m := pressKey(sizedModel(t), "enter")
	speaker, ok := m.activeSpeaker()
	if !ok {
		t.Fatal("no speaker on screen")
	}

	updated, cmd := m.Update(queueLoadedMsg{
		device: speaker.device,
		room:   speaker.device.RoomName(),
		items:  queueItems(items),
	})
	return updated.(appModel), cmd
}

// Case: a tick from a closed list. Three faults in this project came from a
// chain that outlived what began it.
func TestQueue_TheBreathDiesWithTheList(t *testing.T) {
	m := openWithQueue(t, 40)
	gen := m.queueGen

	m = pressKey(m, "esc")
	if m.queue.IsOpen() {
		t.Fatal("esc did not close the list")
	}

	if _, cmd := m.Update(queueTickMsg{gen: gen}); cmd != nil {
		t.Fatal("a tick from the closed list started another chain")
	}
	if m.queueGen == gen {
		t.Fatal("closing the list left its generation in place")
	}
}

// Case: two opens in a row, which must not leave two chains running.
func TestQueue_ASecondOpenDoesNotRunTwoBreaths(t *testing.T) {
	first := openWithQueue(t, 40)
	firstGen := first.queueGen

	second := openWithQueue(t, 40)
	if second.queueGen == firstGen {
		t.Fatal("the second open reused the first generation")
	}

	if _, cmd := second.Update(queueTickMsg{gen: firstGen}); cmd != nil {
		t.Fatal("the first chain is still running behind the second")
	}
}

// Case: the arrow itself, which must change colour as the breath runs. The
// row beside it must not.
func TestQueue_TheArrowChangesColourAndTheRowDoesNot(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(40), rows: 9, width: 70}

	q.openedAt = time.Now()
	atRest := q.View().Content

	q.openedAt = time.Now().Add(-flashsweep.BreathPeriod * 42 / 100) // the peak
	atPeak := q.View().Content

	if atRest == atPeak {
		t.Fatal("the arrow looks the same at the bottom and the top of a breath")
	}
	if plain(atRest) != plain(atPeak) {
		t.Fatal("the breath changed the text, not only its colour")
	}
}

// Case: how far the arrow travels between rest and the top of a breath. A
// narrow swing is found only by a reader who already knows to look for it.
func TestQueue_TheArrowSwingIsWideEnoughToNotice(t *testing.T) {
	const leastSwing = 120 // of 255. The first attempt managed 74 and went unseen.

	rest := arrowBrightness(t, 0)
	lit := arrowBrightness(t, flashsweep.BreathPeriod*42/100) // the peak

	if swing := abs(lit - rest); swing < leastSwing {
		t.Fatalf("the arrow moves %d of 255 between rest and the top of a breath, want at least %d",
			swing, leastSwing)
	}
}

// arrowBrightness renders the arrow at a point in its breath and reports the
// red channel, which is enough: both ends of the range are grey.
func arrowBrightness(t *testing.T, at time.Duration) int {
	t.Helper()

	rendered := queueArrowStyle(time.Now().Add(-at)).Render("↓")
	m := regexp.MustCompile(`38;2;(\d+);`).FindStringSubmatch(rendered)
	if m == nil {
		t.Fatalf("the arrow carries no truecolor foreground: %q", rendered)
	}

	value, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("unreadable colour %q", m[1])
	}
	return value
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Case: the two walls. Pressing against an end must say so, because a key that
// does nothing cannot be told from a program that has stopped answering.
func TestQueue_PressingAgainstAnEndLightsThatEnd(t *testing.T) {
	m := openWithQueue(t, 40)

	top := queueOf(t, pressKey(m, "up"))
	if !top.bumping() || top.edge != bumpTop {
		t.Fatal("up at the top of the list did not light the top")
	}

	atEnd := m
	for range 60 {
		atEnd = pressKey(atEnd, "down")
	}
	bottom := queueOf(t, atEnd)
	if !bottom.bumping() || bottom.edge != bumpBottom {
		t.Fatal("down at the end of the list did not light the bottom")
	}
}

// Case: a press that has somewhere to go. It must scroll and stay quiet, or
// every keypress lights the list up.
func TestQueue_ScrollingDoesNotLightAnything(t *testing.T) {
	m := pressKey(openWithQueue(t, 40), "down")

	content := queueOf(t, m)
	if content.cursor != 1 {
		t.Fatalf("down from the top moved the cursor to %d, want 1", content.cursor)
	}
	if content.bumping() {
		t.Fatal("an ordinary scroll lit the list")
	}
}

// Case: which border carries the light. The end the reader pressed against is
// the one that answers, and the other must stay quiet.
func TestQueue_TheLitBorderIsTheOneAtThatEnd(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(40), rows: 7, width: 60}

	top := q.bump(bumpTop).borderLight()
	if (top(panes.ModalTop, 0) == panes.BorderCell{}) {
		t.Fatal("a bump against the top left the top border unlit")
	}
	if (top(panes.ModalBottom, 0) != panes.BorderCell{}) {
		t.Fatal("a bump against the top lit the bottom border as well")
	}

	bottom := q.bump(bumpBottom).borderLight()
	if (bottom(panes.ModalBottom, 0) == panes.BorderCell{}) {
		t.Fatal("a bump against the end left the bottom border unlit")
	}
	if (bottom(panes.ModalTop, 0) != panes.BorderCell{}) {
		t.Fatal("a bump against the end lit the top border as well")
	}
}

// Case: the pulse itself. It starts at the centre of the border and runs out to
// the corners, rather than lighting the whole edge at once.
func TestQueue_TheLightTravelsFromTheCentreOutward(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(40), rows: 7, width: 60}.bump(bumpTop)

	at := func(age time.Duration, fromCentre float32) bool {
		lit := q
		lit.bumpedAt = time.Now().Add(-age)
		paint := lit.borderLight()
		return paint != nil && paint(panes.ModalTop, fromCentre) != panes.BorderCell{}
	}

	if !at(0, 0) {
		t.Error("the pulse does not start at the centre")
	}
	if at(0, 1) {
		t.Error("the corners are lit before the pulse reaches them")
	}
	if !at(bumpDuration/2, 1) {
		t.Error("the pulse never reaches the corners")
	}
	if at(bumpDuration/2, 0) {
		t.Error("the centre is still lit once the pulse has moved on")
	}
}

// Case: the light going out. It answers a keypress and then leaves, rather
// than staying on the screen as a warning would.
func TestQueue_TheLightCoolsToNothing(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(40), rows: 7, width: 60}.bump(bumpTop)

	if q.heat() < 0.9 {
		t.Fatalf("a fresh bump starts at %.2f, want close to 1", q.heat())
	}

	q.bumpedAt = time.Now().Add(-bumpDuration)
	if q.heat() != 0 {
		t.Fatalf("the bump still reads %.2f after its whole duration", q.heat())
	}
	if q.bumping() {
		t.Fatal("a cooled bump still reports itself lit")
	}

	// And the border asks for no colour of its own, so the modal draws its.
	if q.borderLight() != nil {
		t.Fatal("the border is still painted after the bump ended")
	}
}

// Case: a held key, which repeats about thirty times a second. Each press
// restarts the light, so the edge stays lit until the reader lets go.
func TestQueue_AHeldKeyHoldsTheLightOn(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(40), rows: 7, width: 60}.bump(bumpTop)

	q.bumpedAt = time.Now().Add(-bumpDuration * 9 / 10) // nearly cooled
	faded := q.heat()

	q = q.bump(bumpTop) // the key repeats
	if q.heat() <= faded {
		t.Fatalf("a repeat left the light at %.2f, having been %.2f — it must restart",
			q.heat(), faded)
	}
}

// Case: a queue entirely on screen. Pressing up at the top still does nothing,
// so it still needs an answer, even though there is no hidden end.
func TestQueue_AQueueThatFitsStillBumps(t *testing.T) {
	m := openWithQueue(t, 2)
	if queueOf(t, m).overflows() {
		t.Fatal("a two-item queue should fit the modal, or this proves nothing")
	}

	m = pressKey(m, "up")
	if !queueOf(t, m).bumping() {
		t.Fatal("a queue that fits gave no answer to a key that does nothing")
	}
}

// Case: the repaint chain behind a bump. A queue that fits runs no chain of its
// own, so without one the light would freeze at full brightness.
func TestQueue_ABumpStartsARepaintChain(t *testing.T) {
	m := openWithQueue(t, 2)

	updated, cmd := m.Update(keyMsgFor("up"))
	if cmd == nil {
		t.Fatal("a bump on a queue that fits started no repaint chain")
	}

	m = updated.(appModel)
	if _, again := m.Update(queueTickMsg{gen: m.queueGen}); again == nil {
		t.Fatal("the chain stopped while the light was still lit")
	}

	// And it stops once the light is out, rather than repainting a still list.
	content := queueOf(t, m)
	content.bumpedAt = time.Now().Add(-bumpDuration)
	m.queue.Content = content
	if _, stopped := m.Update(queueTickMsg{gen: m.queueGen}); stopped != nil {
		t.Fatal("the chain kept repainting a list with nothing moving")
	}
}

// Case: the list under the border. A bump answers on the frame, so the list
// itself must not change at all — not its size, and not its colours.
func TestQueue_ABumpLeavesTheListAlone(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(40), rows: 7, width: 60}

	before := q.View().Content
	lit := q.bump(bumpTop).View().Content

	if before != lit {
		t.Fatalf("the bump reached the list:\nbefore %q\nafter  %q", before, lit)
	}
}

// Case: the weight of the line. Colour alone spread the pulse into a smear of
// close shades and was hard to see, so the front thickens the border glyph.
func TestQueue_ThePulseThickensTheLine(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(40), rows: 7, width: 60}.bump(bumpTop)

	weight := func(age time.Duration, fromCentre float32) bool {
		lit := q
		lit.bumpedAt = time.Now().Add(-age)
		return lit.borderLight()(panes.ModalTop, fromCentre).Heavy
	}

	if !weight(0, 0) {
		t.Error("the centre does not thicken when the pulse starts there")
	}
	if weight(0, 1) {
		t.Error("the corners thicken before the pulse reaches them")
	}
	if !weight(bumpDuration/2, 1) {
		t.Error("the corners never thicken")
	}
	if weight(bumpDuration/2, 0) {
		t.Error("the centre stays thick after the pulse has moved on")
	}
}

// Case: the shape of the front. The heavy core must be narrower than the
// coloured run, so the pulse has an edge and a fainter tail behind it. A run
// that is heavy wherever it is coloured is the wide blur this replaced.
func TestQueue_TheHeavyCoreIsNarrowerThanTheGlow(t *testing.T) {
	q := queueContent{room: "Køkken", items: queueItems(40), rows: 7, width: 60}.bump(bumpTop)
	q.bumpedAt = time.Now().Add(-bumpDuration / 3)
	paint := q.borderLight()

	var coloured, weighted int
	for i := range 41 {
		switch cell := paint(panes.ModalTop, float32(i)/40); {
		case cell.Heavy:
			weighted++
			coloured++
		case cell != panes.BorderCell{}:
			coloured++
		}
	}

	if weighted == 0 || coloured == 0 {
		t.Fatalf("nothing is lit a third of the way through: %d heavy, %d coloured", weighted, coloured)
	}
	if weighted >= coloured {
		t.Fatalf("%d of %d lit runes are heavy; the front needs a fainter tail behind it",
			weighted, coloured)
	}
}

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// withQueueInMemory is the state after the startup read has landed: a speaker
// on screen whose Device already holds its queue.
func withQueueInMemory(t *testing.T, items int) appModel {
	t.Helper()
	m, _, _ := withQueueServer(t, items)
	return m
}

// withQueueServer is withQueueInMemory plus the two handles a test needs to
// watch a re-read happen: the device, and a knob that changes what the speaker
// will serve next. It gets to its starting state the way the program does,
// through RefreshQueue against a speaker that answers Browse.
func withQueueServer(t *testing.T, items int) (appModel, *sonos.Device, func(int)) {
	t.Helper()

	device, serve := queueDevice(t, items)
	if err := device.RefreshQueue(); err != nil {
		t.Fatalf("reading the queue: %v", err)
	}

	m := sizedModel(t)
	m.tabs.Active = 0
	m.tabs.Items[0].Content = newSpeakerModel(device)
	return m, device, serve
}

// queueDevice is a speaker that answers Browse, with nothing read from it yet.
func queueDevice(t *testing.T, items int) (*sonos.Device, func(int)) {
	t.Helper()

	var mu sync.Mutex
	serve := func(n int) { mu.Lock(); items = n; mu.Unlock() }

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		items := items
		mu.Unlock()

		var didl strings.Builder
		didl.WriteString(`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/">`)
		for i := range items {
			fmt.Fprintf(&didl, `<item id="Q:0/%d"><dc:title>Track %d</dc:title></item>`, i+1, i+1)
		}
		didl.WriteString(`</DIDL-Lite>`)

		escaped := strings.NewReplacer("<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(didl.String())
		fmt.Fprintf(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
			`<s:Body><u:BrowseResponse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">`+
			`<Result>%s</Result><NumberReturned>%d</NumberReturned><TotalMatches>%d</TotalMatches>`+
			`<UpdateID>3</UpdateID></u:BrowseResponse></s:Body></s:Envelope>`, escaped, items, items)
	}))
	t.Cleanup(server.Close)

	return sonos.NewDeviceWithClient(
		sonos.Speaker{IP: "192.168.1.10"},
		&sonos.SonosClient{BaseURL: server.URL, HTTPClient: server.Client()},
	), serve
}

// runBatch runs a command the way the Bubbletea runtime would, each branch of a
// batch on its own. Some branches never return -- waitForEvent blocks on a
// channel -- so nothing here waits for them.
func runBatch(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() {
		if batch, ok := cmd().(tea.BatchMsg); ok {
			for _, branch := range batch {
				if branch != nil {
					go branch()
				}
			}
		}
	}()
}

// Case: the ordinary open, where the queue was read at startup and kept current
// by events since. It must draw from memory and fetch nothing.
func TestQueueCurrent_OpensFromMemoryWithoutFetching(t *testing.T) {
	m := withQueueInMemory(t, 7)

	model, cmd := m.Update(keyMsgFor("enter"))
	m = model.(appModel)

	if cmd != nil {
		t.Fatal("a queue already in memory must not be fetched again")
	}
	content, ok := m.queue.Content.(queueContent)
	if !ok {
		t.Fatalf("the modal holds %T", m.queue.Content)
	}
	if content.loading {
		t.Fatal("a queue already in memory is not loading")
	}
	if len(content.items) != 7 {
		t.Fatalf("the modal drew %d items, not the 7 in memory", len(content.items))
	}
}

// Case: opening before the startup read has landed. The modal has nothing to
// draw, so it says so and asks for the queue itself.
func TestQueueCurrent_OpeningBeforeTheFirstReadStillFetches(t *testing.T) {
	m := sizedModel(t)

	model, cmd := m.Update(keyMsgFor("enter"))
	m = model.(appModel)

	if cmd == nil {
		t.Fatal("a queue never read must be fetched on open")
	}
	if content, ok := m.queue.Content.(queueContent); !ok || !content.loading {
		t.Fatal("the modal must say it is reading")
	}
}

// Case: a change made somewhere else while the modal is open, which is the
// whole point of following the events.
func TestQueueCurrent_AChangeReachesAnOpenModal(t *testing.T) {
	m, _ := deliverQueue(t, 40)
	speaker, _ := m.activeSpeaker()

	model, _ := m.Update(queueLoadedMsg{
		device: speaker.device,
		room:   speaker.device.RoomName(),
		items:  queueItems(12),
	})
	m = model.(appModel)

	content := m.queue.Content.(queueContent)
	if len(content.items) != 12 {
		t.Fatalf("the open modal still shows %d items", len(content.items))
	}
}

// Case: a queue that shrinks under a reader who was on a row past its new end.
// The window clamps on render either way, but a cursor left out of range puts
// the marker nowhere and moves the next keypress from somewhere unseen.
func TestQueueCurrent_AShorterQueueClampsTheScroll(t *testing.T) {
	m, _ := deliverQueue(t, 200)
	speaker, _ := m.activeSpeaker()

	content := m.queue.Content.(queueContent)
	content.cursor, content.offset = 150, 150
	m.queue.Content = content

	model, _ := m.Update(queueLoadedMsg{
		device: speaker.device,
		room:   speaker.device.RoomName(),
		items:  queueItems(10),
	})
	m = model.(appModel)

	content = m.queue.Content.(queueContent)
	if content.cursor > content.lastRow() {
		t.Fatalf("the cursor is left on row %d of a queue ending at %d",
			content.cursor, content.lastRow())
	}
	if content.offset > content.maxOffset() {
		t.Fatalf("the window is left at row %d of a queue ending at %d",
			content.offset, content.maxOffset())
	}
}

// Case: a queue arriving for a speaker the reader is not looking at. Every
// speaker is read, so most results are for some other room.
func TestQueueCurrent_AnotherRoomsQueueIsNotDrawn(t *testing.T) {
	m, _ := deliverQueue(t, 40)

	other := sonos.NewDevice(sonos.Speaker{IP: "192.168.1.99"})
	model, _ := m.Update(queueLoadedMsg{device: other, room: "Elsewhere", items: queueItems(3)})
	m = model.(appModel)

	content := m.queue.Content.(queueContent)
	if len(content.items) != 40 {
		t.Fatalf("another room's queue replaced the one on screen: %d items", len(content.items))
	}
}

// Case: a queue changed from the Sonos app, which is the whole reason for
// following the events. The program must read it again without a keypress.
func TestQueueCurrent_AnEventReadsTheQueueAgain(t *testing.T) {
	m, device, serve := withQueueServer(t, 7)
	serve(12) // the speaker now holds a different queue

	_, cmd := m.Update(sonos.Event{
		Kind:      sonos.EventContentDirectory,
		Speaker:   device.Speaker,
		Directory: queueChangedEvent(42),
	})
	runBatch(cmd)

	assert.Eventually(t, func() bool { return len(device.State().Queue) == 12 },
		2*time.Second, 10*time.Millisecond, "the queue was never read again")
}

// Case: an event about some other container. It says nothing about the queue,
// so a read would be work done for nothing.
func TestQueueCurrent_AnEventAboutAnotherContainerReadsNothing(t *testing.T) {
	m, device, serve := withQueueServer(t, 7)
	serve(12)

	_, cmd := m.Update(sonos.Event{
		Kind:      sonos.EventContentDirectory,
		Speaker:   device.Speaker,
		Directory: sonos.ContentDirectoryEvent{Containers: []sonos.ContainerUpdate{{ID: "SQ:3", UpdateID: 9}}},
	})
	runBatch(cmd)

	assert.Never(t, func() bool { return len(device.State().Queue) == 12 },
		300*time.Millisecond, 20*time.Millisecond, "a saved-queue change must not read the queue")
}

func queueChangedEvent(updateID int) sonos.ContentDirectoryEvent {
	return sonos.ContentDirectoryEvent{
		Containers: []sonos.ContainerUpdate{{ID: "Q:0", UpdateID: updateID}},
	}
}

// Case: a speaker arriving, from discovery or from the cache. Its queue is read
// at once and in the background, so the modal is full the first time it opens.
func TestQueueCurrent_ASpeakerIsReadWhenItArrives(t *testing.T) {
	device, _ := queueDevice(t, 9)
	if device.State().QueueKnown {
		t.Fatal("nothing should be read before the speaker arrives")
	}

	_, cmd := sizedModel(t).Update(speakerReadyMsg{
		title: "Elsewhere", ip: "192.168.1.77", speaker: newSpeakerModel(device),
	})
	runBatch(cmd)

	assert.Eventually(t, func() bool { return len(device.State().Queue) == 9 },
		2*time.Second, 10*time.Millisecond, "a new speaker's queue was never read")
}

// Case: a cached speaker proving it is still there. It takes the same read: a
// tab drawn from the cache holds no queue until one is fetched.
func TestQueueCurrent_AVerifiedSpeakerIsReadToo(t *testing.T) {
	device, _ := queueDevice(t, 6)

	m := sizedModel(t)
	m.tabs.Items[0].Content = newSpeakerModel(device)

	_, cmd := m.Update(speakerVerifiedMsg{ip: "192.168.1.10", title: "Kitchen", speaker: device})
	runBatch(cmd)

	assert.Eventually(t, func() bool { return len(device.State().Queue) == 6 },
		2*time.Second, 10*time.Millisecond, "a verified speaker's queue was never read")
}

package main

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Case: the queue left open with nothing pressed. Once the wait is up the hint
// goes into the border, as the player's goes into the divider.
func TestQueueHint_ShowsAfterTheWait(t *testing.T) {
	m := openedQueue(t, 40)

	m = deliver(m, listHintDueMsg{gen: m.queueHintGen})

	if !m.queue.NoticeShowing() {
		t.Fatal("no hint after the queue sat open with nothing pressed")
	}
}

// Case: a key pressed before the wait is up. The reader has found the
// keyboard, so the hint has nothing to offer them this time.
func TestQueueHint_AKeyCancelsIt(t *testing.T) {
	m := openedQueue(t, 40)
	due := listHintDueMsg{gen: m.queueHintGen}

	m = pressKey(m, "down")
	m = deliver(m, due)

	if m.queue.NoticeShowing() {
		t.Fatal("the hint showed after a key was pressed")
	}
}

// Case: a key pressed while the hint is already showing. It plays out, as the
// player's does, rather than vanishing mid-sentence.
func TestQueueHint_OneAlreadyShowingPlaysOut(t *testing.T) {
	m := openedQueue(t, 40)
	m = deliver(m, listHintDueMsg{gen: m.queueHintGen})

	m = pressKey(m, "down")

	if !m.queue.NoticeShowing() {
		t.Fatal("a key cut a hint off that was already on the border")
	}
}

// Case: the queue closed before the wait is up. A hint about the queue must
// never land over the player.
func TestQueueHint_DiesWithTheQueue(t *testing.T) {
	m := openedQueue(t, 40)
	due := listHintDueMsg{gen: m.queueHintGen}

	m = pressKey(m, "esc")
	m = deliver(m, due)

	if m.queue.IsOpen() || m.queue.NoticeShowing() {
		t.Fatal("the hint outlived the queue it was about")
	}
}

// Case: the queue opened a second time. One wait runs, not two: the first
// open's must not fire into the second.
func TestQueueHint_ReopeningStartsOneWait(t *testing.T) {
	m := openedQueue(t, 40)
	first := listHintDueMsg{gen: m.queueHintGen}
	m = pressKey(pressKey(m, "esc"), "enter")

	m = deliver(m, first)
	if m.queue.NoticeShowing() {
		t.Fatal("the first open's wait fired into the second")
	}

	m = deliver(m, listHintDueMsg{gen: m.queueHintGen})
	if !m.queue.NoticeShowing() {
		t.Fatal("the second open's own wait did nothing")
	}
}

// Case: an empty queue. There are no tracks for the keys to act on.
func TestQueueHint_NotOverAnEmptyQueue(t *testing.T) {
	m := openedQueue(t, 0)

	m = deliver(m, listHintDueMsg{gen: m.queueHintGen})

	if m.queue.NoticeShowing() {
		t.Fatal("a hint over a queue with no tracks in it")
	}
}

// Case: a queue still being read. There is no list yet to offer keys for.
func TestQueueHint_NotWhileTheQueueLoads(t *testing.T) {
	device, _ := queueDevice(t, 40)
	m := sizedModel(t)
	m.tabs.Active = 0
	m.tabs.Items[0].Content = newSpeakerModel(context.Background(), device)
	m = pressKey(resize(m), "enter")

	m = deliver(m, listHintDueMsg{gen: m.queueHintGen})

	if m.queue.NoticeShowing() {
		t.Fatal("a hint over a queue still being read")
	}
}

// Case: the frames that draw it. They carry on while it shows, and stop once
// it has closed back into the border.
func TestQueueHint_FramesRunWhileItShows(t *testing.T) {
	m := openedQueue(t, 40)
	model, frame := m.Update(listHintDueMsg{gen: m.queueHintGen})
	m = model.(appModel)
	if frame == nil {
		t.Fatal("the hint showed with no frames to draw it")
	}

	if _, next := m.Update(listHintTickMsg{gen: m.queueHintGen}); next == nil {
		t.Fatal("the frames stopped while the hint was still showing")
	}
	if _, stale := m.Update(listHintTickMsg{gen: m.queueHintGen - 1}); stale != nil {
		t.Fatal("a frame from an abandoned chain kept running")
	}
}

// openedQueue is the queue open over a speaker whose list is already read.
func openedQueue(t *testing.T, items int) appModel {
	t.Helper()

	m, _, _ := withQueueServer(t, items)
	return pressKey(resize(m), "enter")
}

func resize(m appModel) appModel {
	model, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 24})
	return model.(appModel)
}

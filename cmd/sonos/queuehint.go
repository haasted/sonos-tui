package main

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// queueHintDelay is longer than the player's hint: the reader has just opened
// something and is reading it, not looking at a screen already taken in.
const queueHintDelay = 5 * time.Second

// queueHintDueMsg says the queue has been open queueHintDelay with no key
// pressed. queueHintTickMsg is one frame of the hint once it shows.
type (
	queueHintDueMsg  struct{ gen int }
	queueHintTickMsg struct{ gen int }
)

func queueHintDue(gen int) tea.Cmd {
	return tea.Tick(queueHintDelay, func(time.Time) tea.Msg { return queueHintDueMsg{gen: gen} })
}

func queueHintTick(gen int, every time.Duration) tea.Cmd {
	return tea.Tick(every, func(time.Time) tea.Msg { return queueHintTickMsg{gen: gen} })
}

// restartQueueHint starts the wait for the hint afresh, abandoning any wait or
// frames already running. Every open of the queue begins one.
func (m appModel) restartQueueHint() (appModel, tea.Cmd) {
	m.queueHintGen = nextQueueGen()
	return m, queueHintDue(m.queueHintGen)
}

// retireQueueHint cancels a hint still waiting, because a key was pressed. One
// already on the border plays out, as the player's hint does.
func (m appModel) retireQueueHint() appModel {
	if !m.queue.NoticeShowing() {
		m.queueHintGen = nextQueueGen()
	}
	return m
}

// applyQueueHintDue sets the hint into the queue's bottom border, unless there
// are no tracks yet for the keys to act on: the queue is loading, empty or
// could not be read.
func (m appModel) applyQueueHintDue(msg queueHintDueMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.queueHintGen || !m.queue.IsOpen() {
		return m, nil
	}
	content, ok := m.queue.Content.(queueContent)
	if !ok || content.note() != "" {
		return m, nil
	}

	m.queue = m.queue.Notify(panes.NoticeMsg{
		Level: panes.NoticeInfo, Text: hintText, Entrance: panes.NoticeBloom,
	})
	return m, queueHintTick(msg.gen, m.queue.NoticeInterval())
}

// applyQueueHintTick repaints the hint until it has closed back into the border.
func (m appModel) applyQueueHintTick(msg queueHintTickMsg) (tea.Model, tea.Cmd) {
	if msg.gen != m.queueHintGen || !m.queue.IsOpen() || !m.queue.NoticeShowing() {
		return m, nil
	}
	return m, queueHintTick(msg.gen, m.queue.NoticeInterval())
}

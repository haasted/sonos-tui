// How the application drives a list modal, whichever list it holds: the
// repaint chain behind its arrows and pulses, the key hint, the wheel, and
// its size. listview.go holds the list itself.
package main

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// listHintDelay is longer than the player's hint: the reader has just opened
// something and is reading it, not looking at a screen already taken in.
const listHintDelay = 5 * time.Second

// listContent is what a list modal holds, and what the application needs
// from it without knowing which list it is.
type listContent interface {
	tea.Model
	animating() bool // something on screen moves, so it needs frames
	bumping() bool   // an end is lit, which a fresh chain must carry
	borderLight() panes.BorderLight
	hintReady() bool // there are rows for the hinted keys to act on
	resized(rows, width int) tea.Model
}

// listSlot is one list modal and the two chains that run while it is open:
// the repaint behind its arrows and pulses, and its key hint. The pointers
// reach into the appModel the slot was taken from, so a method takes its
// slots from its own receiver, never from a caller.
type listSlot struct {
	modal *panes.Modal
	pulse *int
	hint  *int
	help  helpTable // what "?" shows over it
}

// listTickMsg is one frame of a list's repaint chain. listHintDueMsg says a
// list has been open listHintDelay with no key pressed, and listHintTickMsg
// is one frame of the hint once it shows. Generations are unique across the
// process, so the generation alone says which list a message is for.
type (
	listTickMsg     struct{ gen int }
	listHintDueMsg  struct{ gen int }
	listHintTickMsg struct{ gen int }
)

func listTick(gen int) tea.Cmd {
	return tea.Tick(listPulseInterval, func(time.Time) tea.Msg { return listTickMsg{gen: gen} })
}

func listHintDue(gen int) tea.Cmd {
	return tea.Tick(listHintDelay, func(time.Time) tea.Msg { return listHintDueMsg{gen: gen} })
}

func listHintTick(gen int, every time.Duration) tea.Cmd {
	return tea.Tick(every, func(time.Time) tea.Msg { return listHintTickMsg{gen: gen} })
}

// listSlots are every list modal, in the order they are drawn.
func (m *appModel) listSlots() []listSlot {
	return []listSlot{m.queueSlot(), m.favouritesSlot()}
}

func (m *appModel) queueSlot() listSlot {
	return listSlot{modal: &m.queue, pulse: &m.queueGen, hint: &m.queueHintGen, help: queueHelp}
}

func (m *appModel) favouritesSlot() listSlot {
	return listSlot{modal: &m.favourites, pulse: &m.favouritesGen, hint: &m.favouritesHintGen, help: favouritesHelp}
}

// listSweeping reports whether the opening light is still going round any
// list modal.
func (m *appModel) listSweeping() bool {
	for _, slot := range m.listSlots() {
		if slot.modal.Sweeping() {
			return true
		}
	}
	return false
}

// openList is the list modal that is open, if any.
func (m *appModel) openList() (listSlot, bool) {
	for _, slot := range m.listSlots() {
		if slot.modal.IsOpen() {
			return slot, true
		}
	}
	return listSlot{}, false
}

// updateList hands a key to the open list modal. It takes the keyboard while
// open, arrow keys included: up and down scroll the list, not the volume.
func (m appModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	slot, ok := m.openList()
	if !ok {
		return m, nil
	}
	retireListHint(slot)
	if msg.String() == "?" {
		return m.openHelp(slot.help)
	}

	modal, cmd := slot.modal.Update(msg)
	*slot.modal = modal
	if !modal.IsOpen() {
		*slot.pulse = nextGen() // the chains die with the list
		*slot.hint = nextGen()
		return m, cmd
	}
	if content, ok := modal.Content.(listContent); ok && content.bumping() {
		return m, tea.Batch(cmd, restartListChain(slot))
	}
	return m, cmd
}

// applyListWheel hands a wheel notch to the open list, the way the arrow keys
// are handed to it, while the list is what the reader is looking at.
func (m appModel) applyListWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	slot, ok := m.openList()
	if !ok || !m.listTakesTheWheel() {
		return m, nil
	}
	retireListHint(slot)

	modal, cmd := slot.modal.Update(msg)
	*slot.modal = modal
	if content, ok := modal.Content.(listContent); ok && content.bumping() {
		return m, tea.Batch(cmd, restartListChain(slot))
	}
	return m, cmd
}

// listTakesTheWheel reports a list as the topmost thing on screen, the only
// time the program asks for the mouse: everywhere else the terminal keeps it,
// and with it the reader's text selection.
func (m appModel) listTakesTheWheel() bool {
	_, open := m.openList()
	return open && !m.help.IsOpen() && m.fits()
}

// restartListChain begins a fresh repaint chain and abandons any running one.
// A bump on a list that fits has no chain to ride, and taking a generation
// each time is what keeps exactly one of them alive.
func restartListChain(slot listSlot) tea.Cmd {
	*slot.pulse = nextGen()
	return listTick(*slot.pulse)
}

// applyListTick repaints a list. It re-arms only while the same modal is open
// and something on it still moves, so the chain cannot outlive either.
func (m appModel) applyListTick(msg listTickMsg) (tea.Model, tea.Cmd) {
	for _, slot := range m.listSlots() {
		if *slot.pulse != msg.gen || !slot.modal.IsOpen() {
			continue
		}
		if content, ok := slot.modal.Content.(listContent); ok && content.animating() {
			return m, listTick(msg.gen)
		}
	}
	return m, nil
}

// restartListHint starts the wait for the hint afresh, abandoning any wait or
// frames already running. Every open of a list begins one.
func restartListHint(slot listSlot) tea.Cmd {
	*slot.hint = nextGen()
	return listHintDue(*slot.hint)
}

// retireListHint cancels a hint still waiting, because a key was pressed. One
// already on the border plays out, as the player's hint does.
func retireListHint(slot listSlot) {
	if !slot.modal.NoticeShowing() {
		*slot.hint = nextGen()
	}
}

// applyListHintDue sets the hint into the list's bottom border, unless there
// are no rows yet for the keys to act on.
func (m appModel) applyListHintDue(msg listHintDueMsg) (tea.Model, tea.Cmd) {
	for _, slot := range m.listSlots() {
		if *slot.hint != msg.gen || !slot.modal.IsOpen() {
			continue
		}
		if content, ok := slot.modal.Content.(listContent); !ok || !content.hintReady() {
			return m, nil
		}
		*slot.modal = slot.modal.Notify(panes.NoticeMsg{
			Level: panes.NoticeInfo, Text: hintText, Entrance: panes.NoticeBloom,
		})
		return m, listHintTick(msg.gen, slot.modal.NoticeInterval())
	}
	return m, nil
}

// applyListHintTick repaints the hint until it has closed back into the border.
func (m appModel) applyListHintTick(msg listHintTickMsg) (tea.Model, tea.Cmd) {
	for _, slot := range m.listSlots() {
		if *slot.hint == msg.gen && slot.modal.IsOpen() && slot.modal.NoticeShowing() {
			return m, listHintTick(msg.gen, slot.modal.NoticeInterval())
		}
	}
	return m, nil
}

// noticeOnList shows a notice in the open list modal's bottom border, where
// the reader is looking. The tab row it would otherwise go to is under the
// modal. It rides the hint's chain, which a notice replaces.
func (m appModel) noticeOnList(msg panes.NoticeMsg) (tea.Model, tea.Cmd) {
	slot, ok := m.openList()
	if !ok {
		return m, nil
	}
	*slot.hint = nextGen()
	*slot.modal = slot.modal.Notify(msg)
	return m, listHintTick(*slot.hint, slot.modal.NoticeInterval())
}

// resizeLists sizes every list modal to the terminal and tells each list how
// many rows it may draw. The modal clips anything past that, so a list which
// does not know cannot scroll correctly.
func (m appModel) resizeLists(width, height int) appModel {
	for _, slot := range m.listSlots() {
		*slot.modal = slot.modal.SetSize(width, height)
		if content, ok := slot.modal.Content.(listContent); ok {
			slot.modal.Content = content.resized(slot.modal.ContentRows(), slot.modal.ContentColumns())
		}
	}
	return m
}

// litLists are the list modals as this frame draws them, each border lit by
// its own pulse. The light is read fresh each frame: it depends on when the
// reader last pressed against an end.
func (m appModel) litLists() []panes.Modal {
	var lit []panes.Modal
	for _, slot := range m.listSlots() {
		modal := *slot.modal
		if content, ok := modal.Content.(listContent); ok {
			modal.Light = content.borderLight()
		}
		lit = append(lit, modal)
	}
	return lit
}

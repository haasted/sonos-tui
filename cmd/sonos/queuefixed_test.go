package main

import (
	"strings"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: the size of the box as the queue shortens, which is what a reader
// removing tracks watches. It must not change: a box that resizes under a
// keypress is one they cannot aim at.
func TestQueueFixed_TheBoxDoesNotChangeAsTracksGo(t *testing.T) {
	modal := fixedQueueModal(40)
	wideW, wideH := modal.BoxSize()

	modal.Content = navQueue(3, 0)
	shortW, shortH := modal.BoxSize()

	if shortW != wideW || shortH != wideH {
		t.Fatalf("the box went from %dx%d to %dx%d when the queue shortened",
			wideW, wideH, shortW, shortH)
	}
}

// Case: the widest title leaving the queue, which used to narrow the box
// because it was measured from what was rendered.
func TestQueueFixed_TheBoxDoesNotChangeWithTheWidestTitle(t *testing.T) {
	modal := fixedQueueModal(20)
	content := navQueue(20, 0)
	content.items[7].Title = strings.Repeat("A very long title indeed ", 3)

	modal.Content = content
	withLong, _ := modal.BoxSize()

	content.items = append(content.items[:7], content.items[8:]...)
	modal.Content = content
	withoutLong, _ := modal.BoxSize()

	if withLong != withoutLong {
		t.Fatalf("the box was %d wide and became %d when the longest title went",
			withLong, withoutLong)
	}
}

// Case: where the box sits. Top-aligned, so it does not drift down the
// screen as its content changes -- and it cannot, its size being fixed.
func TestQueueFixed_TheBoxSitsAtTheTop(t *testing.T) {
	modal := fixedQueueModal(5).Open()
	backdrop := strings.Repeat("background\n", 30)

	over := strings.Split(modal.ViewOver(backdrop), "\n")

	if strings.Contains(over[0], "background") {
		t.Fatalf("the first row is still the view behind it: %q", over[0])
	}
}

// Case: the help popup, which must keep fitting itself to its table. A
// six-row table stretched across the terminal would be worse than it is.
func TestQueueFixed_TheHelpPopupStillFitsItsContent(t *testing.T) {
	help := panes.NewModal(helpContent{table: queueHelp}).SetSize(120, 40)
	allowedW, allowedH := panes.NewModal(queueContent{}).SetSize(120, 40).BoxSize()

	w, h := help.BoxSize()
	if w == allowedW && h == allowedH {
		t.Fatal("the help popup takes the whole allowance; only the queue should")
	}
}

// fixedQueueModal is the queue modal as the program builds it, holding a
// queue of n tracks.
func fixedQueueModal(items int) panes.Modal {
	modal := panes.NewModal(queueContent{})
	modal.Mode = panes.ModalFixed
	modal = modal.SetSize(120, 40)
	modal.Content = navQueue(items, 0)
	return modal
}

// Case: the modal the PROGRAM builds, as against one a test builds for
// itself. Every other case here sets the mode by hand and so cannot see the
// queue being opened in the wrong one.
func TestQueueFixed_TheProgramOpensTheQueueFixed(t *testing.T) {
	m, _, _ := withQueueServer(t, 20)

	if m.queue.Mode != panes.ModalFixed {
		t.Fatalf("the queue modal is in mode %d, want ModalFixed (%d)", m.queue.Mode, panes.ModalFixed)
	}
	if m.help.Mode == panes.ModalFixed {
		t.Fatal("the help popup is fixed too; only the queue should be")
	}
}

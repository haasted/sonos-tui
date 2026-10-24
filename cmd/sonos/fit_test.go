package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Case: a terminal dragged too small mid-session. Quitting would throw the
// session away over a resize, so the message stands in for the UI instead.
func TestFit_TooSmallLaterShowsTheMessageInstead(t *testing.T) {
	m := sizedModel(t)

	m, cmd := sizedTo(m, 40, 8)

	if cmd != nil {
		t.Fatal("a resize mid-session quit the program")
	}
	view := plain(m.View().Content)
	if !strings.Contains(view, "at least") || strings.Contains(view, "Kitchen") {
		t.Fatalf("the UI was drawn into a terminal too small for it:\n%s", view)
	}
}

// Case: the terminal made large enough again. The UI comes straight back.
func TestFit_TheUIReturnsWhenThereIsRoom(t *testing.T) {
	m, _ := sizedTo(sizedModel(t), 40, 8)

	m, _ = sizedTo(m, minTerminalWidth, minTerminalHeight)

	if !strings.Contains(plain(m.View().Content), "Kitchen") {
		t.Fatal("the UI did not come back at exactly the minimum size")
	}
}

// Case: keys pressed while the UI is hidden. They must not act on a screen the
// reader cannot see -- but quitting still works.
func TestFit_KeysDoNothingWhileHiddenExceptQuit(t *testing.T) {
	m, _ := sizedTo(sizedModel(t), 40, 8)

	m = pressKey(m, "enter")
	if m.queue.IsOpen() {
		t.Fatal("a key opened the queue behind the message")
	}

	m = pressKey(m, "q")
	if !m.quitting {
		t.Fatal("q did not quit while the UI was hidden")
	}
}

func sizedTo(m appModel, width, height int) (appModel, tea.Cmd) {
	model, cmd := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return model.(appModel), cmd
}

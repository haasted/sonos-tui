package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: the frame property Bubbletea v2 switches on, which must follow the
// mode — full screen claims the alt buffer, hovering never does.
func TestModal_AltScreenFollowsTheMode(t *testing.T) {
	m := sizedShell(t)
	if m.View().AltScreen {
		t.Fatal("the plain layout must not claim the alt screen")
	}

	m = key(m, "?")
	if m.View().AltScreen {
		t.Fatal("hover mode must not claim the alt screen")
	}

	m = key(m, "m")
	if !m.View().AltScreen {
		t.Fatal("full screen mode must claim the alt screen")
	}

	m = key(m, "esc")
	if m.View().AltScreen {
		t.Fatal("closing must hand the alt screen back")
	}
}

// Case: the inline frame either side of a modal, which must come back exactly
// as it was — a changed height is what would scroll the terminal.
func TestModal_InlineFrameSurvivesTheRoundTrip(t *testing.T) {
	m := sizedShell(t)
	before := m.View().Content

	m = key(m, "?")
	m = key(m, "?")

	if after := m.View().Content; after != before {
		t.Fatalf("inline frame changed across the round trip\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// Case: keys pressed while the modal is up, which must not reach the tabs
// underneath.
func TestModal_OwnsTheKeyboardWhileOpen(t *testing.T) {
	m := sizedShell(t)
	active := m.tabs.Active

	m = key(m, "?")
	m = key(m, "right")

	if m.tabs.Active != active {
		t.Fatal("tab navigation reached the tabs behind an open modal")
	}
	if m.quitting {
		t.Fatal("q behind an open modal must close it, not quit the program")
	}
}

// Case: a resize while the modal is open, which must re-fit it rather than
// leaving it at the old size.
func TestModal_TracksAResizeWhileOpen(t *testing.T) {
	m := sizedShell(t)
	m = key(m, "?")

	m = resize(m, 100, 30)

	view := m.View().Content
	if !strings.Contains(view, "100×30") {
		t.Fatal("modal did not re-fit to the new terminal size")
	}
	if !strings.Contains(view, "resizes open  1") {
		t.Fatal("resize while open was not recorded")
	}
}

// Case: layouts that never opt in, which must keep the shell they had.
func TestModal_IsOptIn(t *testing.T) {
	m := resize(newSimpleTabsModel(demoTabs()), 80, 24)

	m = key(m, "?")

	if m.modal.open {
		t.Fatal("a layout that never asked for a modal must not get one")
	}
	if m.View().AltScreen {
		t.Fatal("a layout without a modal must never claim the alt screen")
	}
}

// Case: full mode, whose frame must match the terminal exactly. One row too
// tall scrolls even in the alt screen.
func TestModal_FullModeFillsTheTerminalExactly(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {60, 12}, {120, 40}} {
		m := key(key(resize(newSimpleTabsModel(demoTabs()).withModal(), size[0], size[1]), "?"), "m")

		view := m.View().Content
		if got := [2]int{lipgloss.Width(view), lipgloss.Height(view)}; got != size {
			t.Errorf("terminal %dx%d got a %dx%d frame", size[0], size[1], got[0], got[1])
		}
	}
}

// Case: the two modes side by side, where hovering must be a sized box and
// full screen must take the whole terminal.
func TestModal_HoverIsSmallerThanFullScreen(t *testing.T) {
	hover := key(resize(newSimpleTabsModel(demoTabs()).withModal(), 100, 30), "?")
	full := key(hover, "m")

	if !strings.Contains(hover.View().Content, "modal         66×20") {
		t.Fatalf("unexpected hover size:\n%s", hover.View().Content)
	}
	if !strings.Contains(full.View().Content, "modal         100×30") {
		t.Fatal("full screen must size the box to the terminal")
	}
}

// Case: hover mode, which sits over the app's own frame without claiming the
// alt screen — and grows that frame, which is what son-5eb bites on.
func TestModal_HoverKeepsTheAppVisibleAndGrowsTheFrame(t *testing.T) {
	m := sizedShell(t)
	inlineRows := lipgloss.Height(m.View().Content)

	m = key(m, "?")

	view := m.View()
	if view.AltScreen {
		t.Fatal("hover mode must not claim the alt screen")
	}
	// The glyph belongs to the tab bar and appears nowhere in the box, so it
	// is unambiguous evidence the app survived underneath.
	if !strings.Contains(view.Content, "⣀") {
		t.Fatalf("the app must still be visible around a hovering modal:\n%s", view.Content)
	}
	if lipgloss.Height(view.Content) <= inlineRows {
		t.Fatal("test is not exercising frame growth, so it proves nothing about son-5eb")
	}
}

func sizedShell(t *testing.T) simpleTabsModel {
	t.Helper()
	return resize(newSimpleTabsModel(demoTabs()).withModal(), 80, 24)
}

func demoTabs() panes.Tabs {
	return panes.NewTabs([]panes.Tab{
		{Title: "Kitchen", Content: panes.StaticContent("a")},
		{Title: "Bedroom", Content: panes.StaticContent("b")},
	})
}

func resize(m simpleTabsModel, w, h int) simpleTabsModel {
	model, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return model.(simpleTabsModel)
}

func key(m simpleTabsModel, k string) simpleTabsModel {
	model, _ := m.Update(keyMsgFor(k))
	return model.(simpleTabsModel)
}

func keyMsgFor(k string) tea.KeyMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	default:
		return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
}

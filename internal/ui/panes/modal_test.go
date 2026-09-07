package panes_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
	"github.com/stretchr/testify/assert"
)

// Case: a modal that has not been opened, which must be invisible rather than
// merely empty — callers render through it unconditionally.
func TestModal_ClosedLeavesTheViewUntouched(t *testing.T) {
	m := panes.NewModal(panes.StaticContent("secret")).SetSize(80, 24)

	assert.False(t, m.IsOpen())
	assert.False(t, m.AltScreen())
	assert.Equal(t, "background", m.ViewOver("background"))
}

// Case: the alt-screen property, which must depend on the mode and only apply
// while open — a closed full-screen modal must not blank the terminal.
func TestModal_AltScreenNeedsBothOpenAndFullScreen(t *testing.T) {
	hover := panes.NewModal(nil).SetSize(80, 24).Open()
	assert.False(t, hover.AltScreen(), "hovering must never claim the alt screen")

	full := hover
	full.Mode = panes.ModalFullScreen
	assert.True(t, full.AltScreen())
	assert.False(t, full.Close().AltScreen(), "a closed modal must hand the alt screen back")
}

// Case: hover mode, which must sit within the caller's frame and leave the
// surrounding UI visible.
func TestModal_HoverKeepsTheBackgroundVisible(t *testing.T) {
	m := panes.NewModal(panes.StaticContent("modal body")).SetSize(80, 24).Open()

	view := m.ViewOver(strings.Repeat("BACKGROUND ", 7))

	assert.Contains(t, view, "modal body")
	assert.Contains(t, view, "BACKGROUND", "the caller's view must survive around a hovering modal")
}

// Case: full-screen mode, which draws onto a blank alternate screen and so
// must pad the background out to the terminal itself.
func TestModal_FullScreenFillsTheTerminal(t *testing.T) {
	m := panes.NewModal(panes.StaticContent("body")).SetSize(60, 20).Open()
	m.Mode = panes.ModalFullScreen

	view := m.ViewOver("short")

	assert.Equal(t, 60, lipgloss.Width(view))
	assert.Equal(t, 20, lipgloss.Height(view))
}

// Case: the sizes the component picks, which must be a clamped share when
// hovering and the whole terminal when full screen.
func TestModal_BoxSizeFollowsTheMode(t *testing.T) {
	m := panes.NewModal(nil).SetSize(120, 40).Open()

	width, height := m.BoxSize()
	assert.Equal(t, 76, width, "hover width must stop at the maximum")
	assert.Equal(t, 22, height, "hover height must stop at the maximum")

	m.Mode = panes.ModalFullScreen
	width, height = m.BoxSize()
	assert.Equal(t, 120, width)
	assert.Equal(t, 40, height)
}

// Case: the renderer drops lines from the top of a frame taller than the
// terminal, so a hovering modal must never exceed it at any size.
func TestModal_HoverNeverExceedsTheTerminal(t *testing.T) {
	// The narrow sizes matter most: they are below the minimum the clamp
	// would otherwise impose, so the terminal has to win.
	for _, size := range [][2]int{{20, 4}, {30, 6}, {40, 10}, {60, 16}, {80, 24}, {200, 60}} {
		m := panes.NewModal(nil).SetSize(size[0], size[1]).Open()

		width, height := m.BoxSize()
		assert.LessOrEqual(t, width, size[0], "modal wider than a %dx%d terminal", size[0], size[1])
		assert.LessOrEqual(t, height, size[1], "modal taller than a %dx%d terminal", size[0], size[1])
	}
}

// Case: content longer than the box, which must be clipped rather than growing
// the frame past the terminal.
func TestModal_OversizedContentIsClipped(t *testing.T) {
	tall := panes.StaticContent(strings.Repeat("line\n", 200))
	m := panes.NewModal(tall).SetSize(80, 24).Open()

	_, height := m.BoxSize()
	assert.Equal(t, height, lipgloss.Height(m.ViewOver("")))
}

// Case: esc, which the modal dismisses itself so every caller does not have to
// reimplement it.
func TestModal_EscapeClosesIt(t *testing.T) {
	m := panes.NewModal(nil).SetSize(80, 24).Open()

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	assert.False(t, m.IsOpen())
}

// Case: any other message while open, which belongs to the content — the modal
// must not swallow what it does not handle.
func TestModal_ForwardsOtherMessagesToItsContent(t *testing.T) {
	m := panes.NewModal(countingContent{}).SetSize(80, 24).Open()

	m, _ = m.Update(tea.KeyPressMsg{Code: 'x'})

	assert.Equal(t, 1, m.Content.(countingContent).seen)
}

// Case: messages arriving while closed, which must not reach the content — a
// dismissed modal is not listening.
func TestModal_IgnoresMessagesWhileClosed(t *testing.T) {
	m := panes.NewModal(countingContent{})

	m, _ = m.Update(tea.KeyPressMsg{Code: 'x'})

	assert.Equal(t, 0, m.Content.(countingContent).seen)
}

// countingContent records how many messages reached it.
type countingContent struct{ seen int }

func (c countingContent) Init() tea.Cmd { return nil }

func (c countingContent) Update(tea.Msg) (tea.Model, tea.Cmd) {
	c.seen++
	return c, nil
}

func (c countingContent) View() tea.View { return tea.NewView("counting") }

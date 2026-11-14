package panes_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/ui/panes"
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

// Case: full screen, which must take the terminal whatever it contains.
func TestModal_FullScreenIgnoresContentSize(t *testing.T) {
	m := panes.NewModal(panes.StaticContent("tiny")).SetSize(120, 40).Open()
	m.Mode = panes.ModalFullScreen

	width, height := m.BoxSize()

	assert.Equal(t, 120, width)
	assert.Equal(t, 40, height)
}

// Case: a hovering modal holding little, which must shrink to it rather than
// take its whole allowance. Every row it does not need is a row of inline
// frame growth avoided.
func TestModal_HoverShrinksToItsContent(t *testing.T) {
	small := panes.NewModal(panes.StaticContent("one\ntwo\nthree")).SetSize(120, 40).Open()
	large := panes.NewModal(panes.StaticContent(strings.Repeat("a line of content\n", 40))).SetSize(120, 40).Open()

	_, smallHeight := small.BoxSize()
	_, largeHeight := large.BoxSize()

	assert.Equal(t, 8, smallHeight, "three lines must not claim the full allowance")
	assert.Equal(t, 26, largeHeight, "long content must still stop at the allowance")
	assert.Less(t, smallHeight, largeHeight)
}

// Case: content wider than the allowance, which must be capped rather than
// widening the box past what the terminal can show.
func TestModal_HoverCapsWideContent(t *testing.T) {
	m := panes.NewModal(panes.StaticContent(strings.Repeat("x", 500))).SetSize(120, 40).Open()

	width, _ := m.BoxSize()

	assert.Equal(t, 76, width, "content must not push the box past its allowance")
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

// Case: a lit border, as a caller sees it. A terminal cannot draw a thicker
// line, so weight is the only way the border can look raised rather than
// merely recoloured.
func TestModal_AHeavyBorderSwapsTheGlyphs(t *testing.T) {
	m := panes.NewModal(panes.StaticContent("body"))
	m.Mode = panes.ModalHover
	m = m.SetSize(40, 12).Open()

	resting := strings.Split(stripANSI(m.ViewOver(backdrop(40, 12))), "\n")

	// Light every rune of the top edge, and nothing of the bottom.
	m.Light = func(edge panes.ModalEdge, _ float32) panes.BorderCell {
		return panes.BorderCell{Heavy: edge == panes.ModalTop}
	}
	lit := strings.Split(stripANSI(m.ViewOver(backdrop(40, 12))), "\n")

	top, bottom := borderRows(t, resting)
	litTop, litBottom := borderRows(t, lit)

	assert.Equal(t, bottom, litBottom, "the unlit edge must keep its own glyphs")
	assert.NotEqual(t, top, litTop, "a heavy edge must not look like a resting one")

	assert.Contains(t, litTop, "━", "the horizontal run should thicken")
	assert.Contains(t, litTop, "┏", "the left corner should take the weight")
	assert.Contains(t, litTop, "┓", "the right corner should take the weight")
}

// Case: a modal with no light, which is every modal today. It must render
// exactly as it did before the hook existed.
func TestModal_AnUnlitBorderIsUntouched(t *testing.T) {
	m := panes.NewModal(panes.StaticContent("body"))
	m.Mode = panes.ModalHover
	m = m.SetSize(40, 12).Open()

	plain := stripANSI(m.ViewOver(backdrop(40, 12)))
	for _, heavy := range []string{"━", "┃", "┏", "┓", "┗", "┛"} {
		assert.NotContains(t, plain, heavy, "an unlit border drew a heavy glyph")
	}
}

func backdrop(w, h int) string {
	return lipgloss.NewStyle().Width(w).Height(h).Render("")
}

// borderRows returns the first and last lines that carry a box-drawing corner.
func borderRows(t *testing.T, lines []string) (top, bottom string) {
	t.Helper()

	for _, line := range lines {
		if strings.ContainsAny(line, "\u256d\u250f\u2570\u2517") {
			if top == "" {
				top = line
			}
			bottom = line
		}
	}
	if top == "" || top == bottom {
		t.Fatalf("expected two border rows, found %q and %q", top, bottom)
	}
	return top, bottom
}

package panes_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// Case: the point where the content's first character is drawn, in each mode
// that draws over a backdrop. It maps to the content's own top-left.
func TestModalPoint_TheContentsFirstCellIsItsOrigin(t *testing.T) {
	for _, mode := range []panes.ModalMode{panes.ModalHover, panes.ModalFixed} {
		m := panes.NewModal(panes.StaticContent("secret\nsecond")).SetSize(80, 24).Open()
		m.Mode = mode
		backdrop := strings.Repeat(strings.Repeat(".", 80)+"\n", 19) + strings.Repeat(".", 80)

		x, y := cellOf(t, m.ViewOver(backdrop), "secret")
		cx, cy, ok := m.ContentPoint(backdrop, x, y)
		require.True(t, ok, "mode %d: the content's first cell is outside it", mode)
		assert.Equal(t, [2]int{0, 0}, [2]int{cx, cy}, "mode %d", mode)

		_, cy, _ = m.ContentPoint(backdrop, x+2, y+1)
		assert.Equal(t, 1, cy, "mode %d: the second line", mode)
	}
}

// Case: points on the border, on the padding, off the box, and with the modal
// closed. None of them is in the content.
func TestModalPoint_OffTheContentIsNotInIt(t *testing.T) {
	m := panes.NewModal(panes.StaticContent("secret")).SetSize(80, 24).Open()
	backdrop := strings.Repeat(strings.Repeat(".", 80)+"\n", 19) + strings.Repeat(".", 80)
	view := m.ViewOver(backdrop)
	x, y := cellOf(t, view, "secret")
	_, bottom := cellOf(t, view, "╰")

	for name, point := range map[string][2]int{
		"padding":       {x - 1, y},
		"left border":   {x - 2, y},
		"top border":    {x, y - 1},
		"bottom border": {x, bottom},
		"backdrop":      {0, 0},
	} {
		_, _, ok := m.ContentPoint(backdrop, point[0], point[1])
		assert.False(t, ok, name)
	}
	_, last, ok := m.ContentPoint(backdrop, x, bottom-1)
	assert.True(t, ok, "the last line inside the box")
	assert.Equal(t, bottom-1-y, last)

	_, _, ok = m.Close().ContentPoint(backdrop, x, y)
	assert.False(t, ok, "a closed modal")
}

// cellOf is the column and line where text is first drawn in a view.
func cellOf(t *testing.T, view, text string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(view), "\n") {
		if at := strings.Index(line, text); at >= 0 {
			return ansi.StringWidth(line[:at]), y
		}
	}
	t.Fatalf("%q is not drawn", text)
	return 0, 0
}

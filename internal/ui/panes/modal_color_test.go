package panes_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// Case: a modal given its own border colour, so two modals can be told apart.
// The border must be drawn in it, and the default must stay as it was.
func TestModal_DrawsItsBorderInItsOwnColour(t *testing.T) {
	plain := panes.NewModal(panes.StaticContent("body")).SetSize(60, 20).Open()
	rose := plain
	rose.Color = lipgloss.Color("#F472B6")

	assert.Contains(t, panes.ModalBox(rose), "38;2;244;114;182m", "the border is not in the modal's colour")
	assert.NotContains(t, panes.ModalBox(plain), "38;2;244;114;182m", "the default border took another modal's colour")
}

// Case: a light on the left side only, as a sideways press asks for. Every
// row between the corners must draw its left rune in the light's colour, and
// the right side must stay at rest.
func TestModal_LightsAVerticalSide(t *testing.T) {
	m := panes.NewModal(panes.StaticContent("body")).SetSize(60, 20).Open()
	m.Light = func(edge panes.ModalEdge, _ float32) panes.BorderCell {
		if edge == panes.ModalLeft {
			return panes.BorderCell{Color: lipgloss.Color("#FF0000")}
		}
		return panes.BorderCell{}
	}

	lines := strings.Split(panes.ModalBox(m), "\n")
	require.Greater(t, len(lines), 2)
	for _, line := range lines[1 : len(lines)-1] {
		assert.True(t, strings.HasPrefix(line, "\x1b[38;2;255;0;0m"), "a left rune was not lit: %q", line)
		assert.NotContains(t, line[len(line)-30:], "255;0;0", "the right side lit too")
	}
}

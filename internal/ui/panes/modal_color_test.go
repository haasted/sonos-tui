package panes_test

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
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

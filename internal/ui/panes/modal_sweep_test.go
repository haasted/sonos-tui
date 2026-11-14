package panes_test

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// Case: the box the light is drawn on, which must be the box that was there
// without it. The sweep hand-draws all four sides where lipgloss drew them
// before, and a box that changed shape as the light finished would make the
// modal jump at the end of its own entrance.
func TestModalSweep_TheBoxKeepsItsShape(t *testing.T) {
	for _, size := range [][2]int{{60, 20}, {100, 40}, {40, 12}} {
		m := openSweeping(size[0], size[1])

		swept := panes.ModalBox(m)
		rested := panes.ModalBox(panes.SetModalSweptAt(m, 2*panes.ModalSweepDuration))

		assert.Equal(t, lipgloss.Width(rested), lipgloss.Width(swept), "width at %v", size)
		assert.Equal(t, lipgloss.Height(rested), lipgloss.Height(swept), "height at %v", size)
		assert.Equal(t, plainLines(rested), plainLines(swept),
			"the light changed the box's text at %v, not only its colour", size)
	}
}

// Case: the trip itself, which must reach every side. A light that stopped
// at the corners would not be going round anything -- and the two vertical
// borders are the ones the bump never had to draw.
func TestModalSweep_TheLightReachesEverySide(t *testing.T) {
	for corner := range 4 {
		for _, clockwise := range []bool{true, false} {
			m := panes.SetModalSweep(openSweeping(60, 20), corner, clockwise)

			var top, bottom, sides bool
			for step := range 40 {
				at := time.Duration(float64(step) / 40 * float64(panes.ModalSweepDuration))
				lines := strings.Split(panes.ModalBox(panes.SetModalSweptAt(m, at)), "\n")

				for i, line := range lines {
					if !hasLitCell(line) {
						continue
					}
					switch i {
					case 0:
						top = true
					case len(lines) - 1:
						bottom = true
					default:
						sides = true
					}
				}
			}

			assert.True(t, top, "corner %d clockwise=%v never lit the top", corner, clockwise)
			assert.True(t, bottom, "corner %d clockwise=%v never lit the bottom", corner, clockwise)
			assert.True(t, sides, "corner %d clockwise=%v never lit a vertical side", corner, clockwise)
		}
	}
}

// Case: the end of the trip. The light goes once and stops, rather than
// circling a modal the reader is trying to read.
func TestModalSweep_ItGoesRoundOnceAndStops(t *testing.T) {
	m := openSweeping(60, 20)
	require.True(t, m.Sweeping(), "the light should be running as the modal opens")

	assert.True(t, panes.SetModalSweptAt(m, panes.ModalSweepDuration/2).Sweeping())
	assert.False(t, panes.SetModalSweptAt(m, panes.ModalSweepDuration).Sweeping(),
		"the light is still going after a full trip's worth of time")
}

// Case: a modal nobody opened. Its border rests, and it asks for no frames.
func TestModalSweep_AClosedModalDoesNotSweep(t *testing.T) {
	m := panes.NewModal(panes.StaticContent("body")).SetSize(60, 20)

	assert.False(t, m.Sweeping())
	assert.NotContains(t, panes.ModalBox(m), "\n\x1b[38;2;255;255;255m",
		"a modal that was never opened drew a light")
}

// Case: where the trip begins. Every opening looks deliberate on its own, and
// none of them look scripted together, which one corner every time would.
func TestModalSweep_StartsSomewhereDifferent(t *testing.T) {
	corners := map[int]bool{}
	directions := map[bool]bool{}

	for range 200 {
		corner, clockwise := panes.ModalSweepCorner(openSweeping(60, 20))
		require.GreaterOrEqual(t, corner, 0)
		require.Less(t, corner, 4, "a corner outside the four a rectangle has")
		corners[corner] = true
		directions[clockwise] = true
	}

	assert.Len(t, corners, 4, "200 openings never used all four corners")
	assert.Len(t, directions, 2, "200 openings never went both ways round")
}

// Case: opening, which must NOT start a trip. A light on every open is one
// the reader comes to expect, where the point is to be caught sight of.
func TestModalSweep_OpeningDoesNotStartTheLight(t *testing.T) {
	m := panes.NewModal(panes.StaticContent("body")).SetSize(60, 20).Open()

	assert.False(t, m.Sweeping(), "opening sent the light round on its own")
}

// Case: asking for another, which must roll again rather than replay the
// last trip.
func TestModalSweep_AskingAgainStartsTheLight(t *testing.T) {
	m := panes.SetModalSweptAt(openSweeping(60, 20), 2*panes.ModalSweepDuration)
	require.False(t, m.Sweeping())

	assert.True(t, m.StartSweep().Sweeping(), "asking again left the border resting")
}

// openSweeping is a modal with a light already running. Opening one does not
// start a trip -- when a light runs is the caller's business -- so the cases
// about the light itself ask for one.
func openSweeping(width, height int) panes.Modal {
	return panes.NewModal(panes.StaticContent("body")).SetSize(width, height).Open().StartSweep()
}

// hasLitCell reports a line holding a colour other than the border's resting
// amber, which is what a cell under the light is drawn in.
func hasLitCell(line string) bool {
	parts := strings.Split(line, "38;2;")
	for _, part := range parts[1:] {
		if !strings.HasPrefix(part, "251;191;36") {
			return true
		}
	}
	return false
}

func plainLines(s string) []string {
	out := strings.Split(s, "\n")
	for i, line := range out {
		var b strings.Builder
		skip := false
		for _, r := range line {
			switch {
			case r == '\x1b':
				skip = true
			case skip && r == 'm':
				skip = false
			case !skip:
				b.WriteRune(r)
			}
		}
		out[i] = b.String()
	}
	return out
}

// Case: a bump arriving during the opening light. The bump answers a
// keypress and the sweep only announces the box, so the reader's own answer
// must not be swallowed by the entrance.
func TestModalSweep_ABumpDuringTheLightStillShows(t *testing.T) {
	m := openSweeping(60, 20)
	require.True(t, m.Sweeping(), "this proves nothing unless the light is running")

	m.Light = func(panes.ModalEdge, float32) panes.BorderCell {
		return panes.BorderCell{Heavy: true}
	}

	assert.Contains(t, panes.ModalBox(m), "┏",
		"the bump's heavy glyphs were lost under the opening light")
}

// The light that runs once around a modal's border as it opens, announcing the
// box in the language the tab glint and the divider sweep already speak.
package panes

import (
	"image/color"
	"math/rand/v2"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/movy"
	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

// modalSweepDuration is one trip around the border. Timed rather than paced
// per cell, so a tall modal and a short one both take the same comfortable
// moment: the light is announcing the box, not measuring it.
const modalSweepDuration = 900 * time.Millisecond

// sweepCorner is where a trip begins, and the direction it runs.
type sweepCorner struct {
	at        int // the corner's place on the clockwise walk, in cells
	clockwise bool
}

// startSweep picks a corner and a direction at random. Every opening looks
// deliberate on its own and none of them look scripted together, which a
// light that always left the same corner would.
func startSweep() (sweepCorner, time.Time) {
	return sweepCorner{at: rand.IntN(4), clockwise: rand.IntN(2) == 0}, time.Now()
}

// sweeping reports the trip still running.
func (m Modal) sweeping() bool {
	return !m.sweptAt.IsZero() && time.Since(m.sweptAt) < modalSweepDuration
}

// Sweeping reports whether the opening light is still travelling, so a caller
// driving the frames knows to ask for another. It answers false for a modal
// that is closed: the light belongs to the box being there.
func (m Modal) Sweeping() bool { return m.open && m.sweeping() }

// sweepBurst is how lit the cell at one place on the perimeter is.
//
// The light runs from a tail's length before the first cell to a tail's length
// past the last, so it arrives from off the corner and leaves the same way
// rather than appearing and vanishing mid-border.
func (m Modal) sweepBurst(index, total int) float32 {
	if total <= 0 {
		return 0
	}

	// Measured from the corner the trip began at, and against the walk or
	// with it. The cells themselves are always laid out clockwise.
	along := index - m.sweep.at*total/4
	if !m.sweep.clockwise {
		along = -along
	}
	along = ((along % total) + total) % total

	progress := float32(time.Since(m.sweptAt)) / float32(modalSweepDuration)
	span := float32(total) + 2*flashsweep.GlintTail
	light := -flashsweep.GlintTail + clamp01(progress)*span

	return flashsweep.GlintBurst(float32(along) - light)
}

func clamp01(v float32) float32 {
	return min(max(v, 0), 1)
}

// sweptBorder draws all four sides with the light on them, wrapping content
// that was rendered without a border of its own.
//
// All four by hand, where the bump only ever needed the two horizontal ones:
// a light that stopped at the corners would not be going round anything. The
// content is rendered unbordered rather than having its border recoloured in
// place, for lit's reason -- a rendered line carries its own escapes, and
// picking them apart again is guesswork about how lipgloss wrote them.
func (m Modal) sweptBorder(inner string, width, rows int) string {
	border := lipgloss.RoundedBorder()
	fill := max(width-2, 0)
	total := 2*width + 2*rows

	// The clockwise walk: the top row, down the right, the bottom row back,
	// and up the left. Each side knows where it starts on that walk.
	lines := strings.Split(inner, "\n")
	var b strings.Builder

	b.WriteString(m.sweptRow(border.TopLeft, border.Top, border.TopRight, fill, 0, total))
	b.WriteString("\n")

	for i, line := range lines {
		right := width + i
		left := total - 1 - i
		b.WriteString(m.sweptCell(border.Left, left, total))
		b.WriteString(line)
		b.WriteString(m.sweptCell(border.Right, right, total))
		b.WriteString("\n")
	}

	// The bottom is walked right to left, so its runes take their places in
	// reverse: the corner under the right edge comes first.
	b.WriteString(m.sweptRowReversed(border.BottomRight, border.Bottom, border.BottomLeft, fill, width+rows, total))
	return b.String()
}

func (m Modal) sweptRow(left, mid, right string, fill, from, total int) string {
	var b strings.Builder
	b.WriteString(m.sweptCell(left, from, total))
	for i := range fill {
		b.WriteString(m.sweptCell(mid, from+1+i, total))
	}
	b.WriteString(m.sweptCell(right, from+fill+1, total))
	return b.String()
}

func (m Modal) sweptRowReversed(first, mid, last string, fill, from, total int) string {
	var b strings.Builder
	cells := make([]string, 0, fill+2)
	cells = append(cells, m.sweptCell(first, from, total))
	for i := range fill {
		cells = append(cells, m.sweptCell(mid, from+1+i, total))
	}
	cells = append(cells, m.sweptCell(last, from+fill+1, total))

	for i := len(cells) - 1; i >= 0; i-- {
		b.WriteString(cells[i])
	}
	return b.String()
}

// sweptCell draws one border rune at its place on the walk.
func (m Modal) sweptCell(glyph string, index, total int) string {
	burst := m.sweepBurst(index, total)
	if burst <= modalSweepVisibleAt {
		return modalBorderStyle().Render(glyph)
	}

	ramp := flashsweep.Ramp{
		Base:  flashsweep.FromColor(modalBorderColor()),
		Flash: flashsweep.FromColor(modalSweepPeak()),
	}
	lit := ramp.Base.Lerp(ramp.Flash, burst)
	return lipgloss.NewStyle().Foreground(rgb(lit)).Render(glyph)
}

// modalSweepVisibleAt is the burst below which a cell is left in the border's
// own colour. Below it the blend is a shade nobody can tell from the resting
// one, and painting it costs an escape sequence per cell for nothing.
const modalSweepVisibleAt float32 = 0.04

// modalSweepPeak brightens on a dark ground and deepens on a light one, the
// pair the arrow and the notice entrance already use. A light that flashes
// toward white on white erases itself.
func modalSweepPeak() color.Color {
	return theme.LightDark(lipgloss.Color("#1F2937"), lipgloss.Color("#FFFFFF"))
}

func rgb(v movy.V3) color.Color { return lipgloss.Color(v.ToRgb().String()) }

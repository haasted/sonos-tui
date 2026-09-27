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

// modalSweepDuration is one run from a corner to the one opposite. Timed
// rather than paced per cell, so a tall modal and a short one both take the
// same comfortable moment: the light is announcing the box, not measuring it.
const modalSweepDuration = 1500 * time.Millisecond

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

// StartSweep sends the light from a fresh corner in a fresh direction. The
// caller chooses when, so the effect can keep its own rhythm rather than
// being tied to the box appearing.
func (m Modal) StartSweep() Modal {
	m.sweep, m.sweptAt = startSweep()
	return m
}

// sweeping reports the trip still running.
func (m Modal) sweeping() bool {
	return !m.sweptAt.IsZero() && time.Since(m.sweptAt) < modalSweepDuration
}

// Sweeping reports whether the opening light is still travelling, so a caller
// driving the frames knows to ask for another. It answers false for a modal
// that is closed: the light belongs to the box being there.
func (m Modal) Sweeping() bool { return m.open && m.sweeping() }

// sweepWalk is the perimeter one trip is measured against: where it begins,
// how far round it goes, and how long the whole walk is.
type sweepWalk struct {
	start int // the clockwise place of the corner the trip began at
	reach int // how far the light travels, corner to the one opposite
	total int
}

// walk works out the trip for a box of this size.
//
// The corners are NOT evenly spaced round the perimeter unless the box is
// square, so they are read from the layout rather than from quarters of it:
// the top row occupies the first width cells, the right side the next rows,
// and so on. Opposite corners are then exactly half the perimeter apart,
// whatever shape the box is.
func (m Modal) walk(width, rows int) sweepWalk {
	corners := []int{0, max(width-1, 0), width + rows, max(2*width+rows-1, 0)}
	total := 2*width + 2*rows

	return sweepWalk{start: corners[m.sweep.at%len(corners)], reach: total / 2, total: total}
}

// sweepBurst is how lit the cell at one place on the perimeter is.
//
// The light runs from a tail's length before the first cell to a tail's
// length past the last, so it arrives from off the corner and leaves the same
// way rather than appearing and vanishing mid-border. It stops at the corner
// opposite the one it began at, having lit two of the four sides.
func (m Modal) sweepBurst(index int, w sweepWalk) float32 {
	if w.total <= 0 {
		return 0
	}

	// Measured from the corner the trip began at, and against the walk or
	// with it. The cells themselves are always laid out clockwise.
	along := index - w.start
	if !m.sweep.clockwise {
		along = -along
	}
	along = ((along % w.total) + w.total) % w.total

	// Nothing past the corner it is heading for. The light runs a tail's
	// length beyond so its trail drains off the end rather than being cut
	// mid-fade, and the two sides it was never on stay dark while it does.
	if along > w.reach {
		return 0
	}

	progress := float32(time.Since(m.sweptAt)) / float32(modalSweepDuration)
	span := float32(w.reach) + 2*flashsweep.GlintTail
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
	w := m.walk(width, rows)
	total := w.total

	// The clockwise walk: the top row, down the right, the bottom row back,
	// and up the left. Each side knows where it starts on that walk.
	lines := strings.Split(inner, "\n")
	var b strings.Builder

	b.WriteString(m.sweptRow(border.TopLeft, border.Top, border.TopRight, fill, 0, w))
	b.WriteString("\n")

	for i, line := range lines {
		right := width + i
		left := total - 1 - i
		b.WriteString(m.sweptCell(border.Left, left, w))
		b.WriteString(line)
		b.WriteString(m.sweptCell(border.Right, right, w))
		b.WriteString("\n")
	}

	// The bottom is walked right to left, so its runes take their places in
	// reverse: the corner under the right edge comes first.
	b.WriteString(m.sweptRowReversed(border.BottomRight, border.Bottom, border.BottomLeft, fill, width+rows, w))
	return b.String()
}

func (m Modal) sweptRow(left, mid, right string, fill, from int, w sweepWalk) string {
	var b strings.Builder
	b.WriteString(m.sweptCell(left, from, w))
	for i := range fill {
		b.WriteString(m.sweptCell(mid, from+1+i, w))
	}
	b.WriteString(m.sweptCell(right, from+fill+1, w))
	return b.String()
}

func (m Modal) sweptRowReversed(first, mid, last string, fill, from int, w sweepWalk) string {
	var b strings.Builder
	cells := make([]string, 0, fill+2)
	cells = append(cells, m.sweptCell(first, from, w))
	for i := range fill {
		cells = append(cells, m.sweptCell(mid, from+1+i, w))
	}
	cells = append(cells, m.sweptCell(last, from+fill+1, w))

	for i := len(cells) - 1; i >= 0; i-- {
		b.WriteString(cells[i])
	}
	return b.String()
}

// sweptCell draws one border rune at its place on the walk.
//
// The head of the light THICKENS the line as well as colouring it, the way
// the edge bump does. Colour alone was not enough here for the reason the
// bump found first: the border rests at amber and the light runs to white,
// and on a dark ground both read as bright, so the glint was a warm-to-cool
// shift rather than anything the eye catches.
func (m Modal) sweptCell(glyph string, index int, w sweepWalk) string {
	burst := m.sweepBurst(index, w)
	if burst <= modalSweepVisibleAt {
		return modalBorderStyle().Render(glyph)
	}

	if burst >= modalSweepHeavyAt {
		glyph = heavyGlyph(glyph)
	}

	ramp := flashsweep.Ramp{
		Base:  flashsweep.FromColor(modalBorderColor()),
		Flash: flashsweep.FromColor(modalSweepPeak()),
	}
	lit := ramp.Base.Lerp(ramp.Flash, burst)
	return lipgloss.NewStyle().Foreground(rgb(lit)).Bold(true).Render(glyph)
}

// heavyGlyph is the thicker form of a single border rune, and the glyph
// untouched when it is anything else.
func heavyGlyph(glyph string) string {
	runes := []rune(glyph)
	if len(runes) != 1 {
		return glyph
	}
	return string(heavy(runes[0]))
}

// modalSweepHeavyAt is where the line thickens, mirroring the edge bump's own
// threshold. Lower it to widen the bright head of the light, raise it to
// leave more of the trip as colour alone.
const modalSweepHeavyAt float32 = 0.35

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

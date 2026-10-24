package panes_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: a notice held open in the modal's bottom border, centred between the
// corners the border keeps.
func TestModalNotice_SitsInTheBottomBorder(t *testing.T) {
	m := hintedModal(panes.NoticeOpenFor(hint) + time.Second)

	bottom := lastLine(plainNotice(panes.ModalBox(m)))

	assert.Contains(t, bottom, hint.Text)
	assert.True(t, strings.HasPrefix(bottom, "╰") && strings.HasSuffix(bottom, "╯"),
		"the corners were lost: %q", bottom)
	assert.NotContains(t, plainNotice(panes.ModalBox(m)), hint.Text+"\n",
		"the notice is somewhere other than the border")
}

// Case: the notice running its course. It goes away on its own and leaves the
// border exactly as a modal that never had one.
func TestModalNotice_GoesAwayOnItsOwn(t *testing.T) {
	resting := panes.ModalBox(openModal())
	long := panes.NoticeOpenFor(hint) + panes.NoticeHoldFor(hint) + panes.NoticeFade

	m := hintedModal(long)

	assert.False(t, m.NoticeShowing())
	assert.Equal(t, resting, panes.ModalBox(m))
}

// Case: the border's own light starting while a notice holds the bottom edge.
// The notice owns the edge, as one owns the divider over its sweep.
func TestModalNotice_TheOpeningLightGivesWay(t *testing.T) {
	m := hintedModal(panes.NoticeOpenFor(hint) + time.Second)
	m = panes.SetModalSweptAt(panes.SetModalSweep(m.StartSweep(), 0, true), panes.ModalSweepDuration/2)
	require.True(t, m.Sweeping(), "the light must be running for this to mean anything")

	assert.Contains(t, lastLine(plainNotice(panes.ModalBox(m))), hint.Text)
}

// Case: opening the modal again. A notice from the last time it was open must
// not survive into the next.
func TestModalNotice_ReopeningClearsIt(t *testing.T) {
	m := hintedModal(panes.NoticeOpenFor(hint) + time.Second)

	m = m.Close().Open()

	assert.False(t, m.NoticeShowing())
}

var hint = panes.NoticeMsg{Level: panes.NoticeInfo, Text: "press ? for keyboard shortcuts", Entrance: panes.NoticeBloom}

// openModal is sized the way the queue is: fixed, the full width of the frame.
func openModal() panes.Modal {
	m := panes.NewModal(panes.StaticContent("body"))
	m.Mode = panes.ModalFixed
	return m.SetSize(70, 20).Open()
}

func hintedModal(elapsed time.Duration) panes.Modal {
	return panes.SetModalNoticeElapsed(openModal().Notify(hint), elapsed)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return lines[len(lines)-1]
}

// Case: a terminal reporting almost no height. The box has no room inside its
// border, and must draw what it can rather than panic.
func TestModal_DrawsInATerminalWithNoRoom(t *testing.T) {
	for _, rows := range []int{0, 1, 2} {
		m := panes.NewModal(panes.StaticContent("body")).SetSize(70, rows).Open()

		assert.NotPanics(t, func() { _ = panes.ModalBox(m) }, "%d rows", rows)
	}
}

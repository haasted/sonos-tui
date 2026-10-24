package panes_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: a fixed modal holding a short queue and then a long one. It is drawn
// at the size it says it is either way, so its bottom edge does not move.
func TestModalHeight_DrawnAtItsOwnSizeWhateverItHolds(t *testing.T) {
	for _, lines := range []int{1, 19, 40} {
		m := fixedModalHolding(lines)
		_, want := m.BoxSize()

		assert.Equal(t, want, rowsOf(panes.ModalBox(m)), "at rest, holding %d lines", lines)

		swept := panes.SetModalSweptAt(panes.SetModalSweep(m.StartSweep(), 0, true), panes.ModalSweepDuration/2)
		assert.Equal(t, want, rowsOf(panes.ModalBox(swept)), "while its light runs, holding %d lines", lines)

		noticed := panes.SetModalNoticeElapsed(m.Notify(hint), panes.NoticeOpenFor(hint)+time.Second)
		assert.Equal(t, want, rowsOf(panes.ModalBox(noticed)), "with a notice in its border, holding %d lines", lines)
	}
}

func fixedModalHolding(lines int) panes.Modal {
	body := strings.TrimSuffix(strings.Repeat("row\n", lines), "\n")
	m := panes.NewModal(panes.StaticContent(body))
	m.Mode = panes.ModalFixed
	return m.SetSize(80, 24).Open()
}

func rowsOf(s string) int { return len(strings.Split(s, "\n")) }

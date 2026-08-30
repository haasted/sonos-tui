package marquee_test

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/haasted/sonos-sandbox/internal/ui/marquee"
	"github.com/stretchr/testify/assert"
)

func TestLine_NeedsScroll(t *testing.T) {
	assert.False(t, marquee.NewLine("short", 10).NeedsScroll())
	assert.True(t, marquee.NewLine("a longer piece of text", 10).NeedsScroll())
}

func TestLine_Render_NoScrollNeeded(t *testing.T) {
	l := marquee.NewLine("short", 10)
	assert.Equal(t, "short", l.Render(lipgloss.NewStyle()))
}

func TestLine_Render_Windowed(t *testing.T) {
	l := marquee.NewLine("abcde", 3)
	assert.Equal(t, "abc", l.Render(lipgloss.NewStyle()))

	l = l.Advance()
	assert.Equal(t, "bcd", l.Render(lipgloss.NewStyle()))
}

func TestLine_Advance_Wraps(t *testing.T) {
	l := marquee.NewLine("abcde", 3)
	for range 20 {
		l = l.Advance()
		assert.NotPanics(t, func() { l.Render(lipgloss.NewStyle()) })
	}
}

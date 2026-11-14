package marquee_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/haasted/sonos-tui/internal/ui/marquee"
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

// Case: a temporarily narrowed line, which must clip without disturbing the
// scroll offset the full-width render depends on.
func TestRenderWidthClipsWithoutMovingTheScroll(t *testing.T) {
	l := marquee.NewLine("abcdefghijklmnop", 10)
	l = l.Advance().Advance()

	full := l.Render(lipgloss.NewStyle())
	narrow := l.RenderWidth(lipgloss.NewStyle(), 6)
	again := l.Render(lipgloss.NewStyle())

	if lipgloss.Width(narrow) != 6 {
		t.Fatalf("narrow render was %d wide, want 6", lipgloss.Width(narrow))
	}
	if full != again {
		t.Fatalf("full render changed across a narrow render: %q then %q", full, again)
	}
	if !strings.HasPrefix(full, narrow) {
		t.Fatalf("narrow render %q should be the head of the full render %q", narrow, full)
	}
}

// Case: a width at or above the built window, which must behave as Render does.
func TestRenderWidthAtOrAboveWindowMatchesRender(t *testing.T) {
	l := marquee.NewLine("short", 20)
	if got, want := l.RenderWidth(lipgloss.NewStyle(), 20), l.Render(lipgloss.NewStyle()); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got, want := l.RenderWidth(lipgloss.NewStyle(), 40), l.Render(lipgloss.NewStyle()); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Case: text that fits the narrowed width, which should render whole.
func TestRenderWidthLeavesFittingTextIntact(t *testing.T) {
	l := marquee.NewLine("abc", 20)
	if got := l.RenderWidth(lipgloss.NewStyle(), 10); got != "abc" {
		t.Fatalf("got %q, want %q", got, "abc")
	}
}

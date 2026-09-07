package panes_test

import (
	"strings"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a notice arriving, which must appear in the divider without adding a
// row — growing the frame is what this design exists to avoid.
func TestNotice_AppearsInTheDividerWithoutGrowingTheFrame(t *testing.T) {
	tabs := noticeTabs()
	before := strings.Count(tabs.View(), "\n")

	tabs = show(tabs, panes.NoticeWarn, "Soveværelse: no live updates")

	assert.Contains(t, tabs.View(), "Soveværelse: no live updates")
	assert.Equal(t, before, strings.Count(tabs.View(), "\n"), "a notice must not add a line")
}

// Case: each severity, which must be distinguishable by shape alone so the
// signal survives a monochrome terminal.
func TestNotice_EachLevelHasItsOwnGlyph(t *testing.T) {
	glyphs := map[panes.NoticeLevel]string{
		panes.NoticeInfo:  "ℹ",
		panes.NoticeWarn:  "⚠",
		panes.NoticeError: "✖",
	}

	seen := map[string]bool{}
	for level, glyph := range glyphs {
		view := show(noticeTabs(), level, "something happened").View()
		assert.Contains(t, view, glyph)
		seen[glyph] = true
	}
	assert.Len(t, seen, 3, "the three levels must not share a glyph")
}

// Case: a second notice arriving while the first still shows — latest wins, so
// the older one must be gone rather than queued behind it.
func TestNotice_LatestReplacesThePrevious(t *testing.T) {
	tabs := show(noticeTabs(), panes.NoticeWarn, "first problem")
	require.Contains(t, tabs.View(), "first problem")

	tabs = show(tabs, panes.NoticeError, "second problem")

	assert.Contains(t, tabs.View(), "second problem")
	assert.NotContains(t, tabs.View(), "first problem")
}

// Case: a message far longer than the divider, which must be clipped to the
// rule rather than widening it — and clipped by rune, since room names carry
// æ, ø and å.
func TestNotice_LongTextIsClippedToTheRule(t *testing.T) {
	tabs := noticeTabs()
	plain := lipglossWidth(tabs.View())

	tabs = show(tabs, panes.NoticeError, strings.Repeat("Soveværelse fejlede ", 20))

	view := tabs.View()
	assert.Equal(t, plain, lipglossWidth(view), "a long notice must not widen the divider")
	assert.Contains(t, view, "…", "clipping must be visible, not silent")
	assert.True(t, strings.ContainsRune(view, 'æ'), "clipping by byte would corrupt multi-byte runes")
}

// Case: a running divider sweep, which must yield the rule to a notice —
// two things animating one line would fight.
func TestNotice_SuppressesTheDividerSweep(t *testing.T) {
	swept := panes.ForceDividerSweep(noticeTabs(), false)
	require.NotEqual(t, swept.View(), noticeTabs().View(), "the sweep must be visibly running")

	withNotice := show(swept, panes.NoticeWarn, "taking the rule")

	assert.Contains(t, withNotice.View(), "taking the rule")
}

// Case: a tick from a notice that has already been replaced, which must not
// keep its successor alive — the son-5cl generation lesson.
func TestNotice_StaleTickIsIgnored(t *testing.T) {
	tabs := noticeTabs()

	first, firstCmd := tabs.Update(panes.NoticeMsg{Level: panes.NoticeInfo, Text: "first"})
	second, _ := first.Update(panes.NoticeMsg{Level: panes.NoticeInfo, Text: "second"})

	require.NotNil(t, firstCmd)
	stale := firstCmd()
	after, cmd := second.Update(stale)

	assert.Nil(t, cmd, "a tick from a replaced notice must not re-arm")
	assert.Contains(t, after.View(), "second", "and must not disturb the current notice")
}

func noticeTabs() panes.Tabs {
	tabs := panes.NewTabs([]panes.Tab{
		{Title: "Køkken", Content: panes.StaticContent("a")},
		{Title: "Spisestue", Content: panes.StaticContent("b")},
		{Title: "Soveværelse", Content: panes.StaticContent("c")},
	})
	tabs.Position = panes.Bottom
	tabs.HideBorder = true
	return tabs
}

func show(tabs panes.Tabs, level panes.NoticeLevel, text string) panes.Tabs {
	updated, _ := tabs.Update(panes.NoticeMsg{Level: level, Text: text})
	return updated
}

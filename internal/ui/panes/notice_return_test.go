package panes_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: a plain notice whose text has just gone, its rule fading back. The
// rule comes back over a moment rather than in one frame.
func TestNoticeReturn_TheRuleFadesBackInWhereTheTextWas(t *testing.T) {
	plain := noticeTabs().View()
	gone := panes.NoticeHoldFor(panes.NoticeMsg{}) + panes.NoticeFade

	tabs := panes.SetNoticeReturn(show(noticeTabs(), panes.NoticeInfo, "paused"), flashsweep.ReturnFade)
	tabs = panes.SetNoticeElapsed(tabs, gone+panes.NoticeReturnOf(tabs)/2)
	mid := tabs.View()

	assert.NotContains(t, plainNotice(mid), "paused", "the text is still drawn while the rule returns")
	assert.NotEqual(t, plain, mid, "the rule came back in one frame")
	assert.Equal(t, plainNotice(plain), plainNotice(mid), "the returning rule is not drawn from rule glyphs")
}

// Case: the return running its course. Once it has, the line is the plain
// divider again, colour and all.
func TestNoticeReturn_EndsOnThePlainRule(t *testing.T) {
	gone := panes.NoticeHoldFor(panes.NoticeMsg{}) + panes.NoticeFade
	for _, style := range flashsweep.ReturnStyles {
		tabs := panes.SetNoticeReturn(show(noticeTabs(), panes.NoticeInfo, "paused"), style)
		tabs = panes.SetNoticeElapsed(tabs, gone+panes.NoticeReturnOf(tabs))

		assert.Equal(t, noticeTabs().View(), tabs.View(), style.String())
	}
}

// Case: every style halfway back. The text is gone whichever plays, and the
// gap holds only rule glyphs, blanks and the stitch's dotted stages.
func TestNoticeReturn_EveryStyleDrawsOnlyTheRuleComingBack(t *testing.T) {
	gone := panes.NoticeHoldFor(panes.NoticeMsg{}) + panes.NoticeFade
	for _, style := range flashsweep.ReturnStyles {
		tabs := panes.SetNoticeReturn(show(noticeTabs(), panes.NoticeInfo, "paused"), style)
		mid := plainNotice(panes.SetNoticeElapsed(tabs, gone+panes.NoticeReturnOf(tabs)/2).View())

		assert.NotContains(t, mid, "paused", style.String())
		assert.Empty(t, strings.Trim(dividerLine(mid), "─ ┈╌"), "%s drew something other than the rule", style)
	}
}

// Case: a longer notice. More rule comes back, so it takes longer.
func TestNoticeReturn_ALongerNoticeTakesLongerToClose(t *testing.T) {
	short := show(noticeTabs(), panes.NoticeInfo, "paused")
	long := show(noticeTabs(), panes.NoticeInfo, "Køkken could not be reached, retrying")

	assert.Greater(t, panes.NoticeReturnOf(long), panes.NoticeReturnOf(short))
}

// Case: the fade before it, which is unchanged in length: the text is still
// on the rule right up to the end of the 900ms.
func TestNoticeReturn_TheTextFadeKeepsItsLength(t *testing.T) {
	gone := panes.NoticeHoldFor(panes.NoticeMsg{}) + panes.NoticeFade
	tabs := panes.SetNoticeElapsed(show(noticeTabs(), panes.NoticeInfo, "paused"), gone-50*time.Millisecond)

	assert.Contains(t, plainNotice(tabs.View()), "paused")
}

// Case: the ticks that drive it. They have to keep coming through the return,
// or the rule would freeze half faded until something else repainted.
func TestNoticeReturn_TicksCarryOnThroughTheReturn(t *testing.T) {
	gone := panes.NoticeHoldFor(panes.NoticeMsg{}) + panes.NoticeFade
	tabs, cmd := noticeTabs().Update(panes.NoticeMsg{Level: panes.NoticeInfo, Text: "paused"})
	require.NotNil(t, cmd)
	tick := cmd()

	returning := panes.SetNoticeElapsed(tabs, gone+panes.NoticeReturnOf(tabs)/2)
	_, again := returning.Update(tick)
	assert.NotNil(t, again, "the ticks stopped while the rule was still coming back")

	finished := panes.SetNoticeElapsed(tabs, gone+panes.NoticeReturnOf(tabs))
	_, stopped := finished.Update(tick)
	assert.Nil(t, stopped, "the ticks carried on after the rule was back")
}

// Case: a notice with an entrance. It already closes back into the rule, so it
// has no return of its own and ends when its collapse does.
func TestNoticeReturn_AnEntranceHasNoReturn(t *testing.T) {
	tabs, _ := noticeTabs().Update(panes.NoticeMsg{
		Level: panes.NoticeInfo, Text: "Køkken is back", Entrance: panes.NoticeMirror,
	})
	long := panes.NoticeHoldFor(panes.NoticeMsg{}) + panes.NoticeFade + 2*time.Second

	assert.Equal(t, noticeTabs().View(), panes.SetNoticeElapsed(tabs, long).View())
}

// dividerLine is the line of a view the rule is drawn on.
func dividerLine(view string) string {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "─") {
			return line
		}
	}
	return ""
}

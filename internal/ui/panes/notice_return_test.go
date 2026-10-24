package panes_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// Case: a plain notice whose text has just gone. The rule it hid comes back
// over a quarter second rather than in one frame.
func TestNoticeReturn_TheRuleFadesBackInWhereTheTextWas(t *testing.T) {
	plain := noticeTabs().View()
	gone := panes.NoticeHoldFor(panes.NoticeMsg{}) + panes.NoticeFade

	tabs := panes.SetNoticeElapsed(show(noticeTabs(), panes.NoticeInfo, "paused"), gone+panes.NoticeReturn/2)
	mid := tabs.View()

	assert.NotContains(t, plainNotice(mid), "paused", "the text is still drawn while the rule returns")
	assert.NotEqual(t, plain, mid, "the rule came back in one frame")
	assert.Equal(t, plainNotice(plain), plainNotice(mid), "the returning rule is not drawn from rule glyphs")
}

// Case: the return running its course. Once it has, the line is the plain
// divider again, colour and all.
func TestNoticeReturn_EndsOnThePlainRule(t *testing.T) {
	gone := panes.NoticeHoldFor(panes.NoticeMsg{}) + panes.NoticeFade
	tabs := panes.SetNoticeElapsed(show(noticeTabs(), panes.NoticeInfo, "paused"), gone+panes.NoticeReturn)

	assert.Equal(t, noticeTabs().View(), tabs.View())
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

	returning := panes.SetNoticeElapsed(tabs, gone+panes.NoticeReturn/2)
	_, again := returning.Update(tick)
	assert.NotNil(t, again, "the ticks stopped while the rule was still coming back")

	finished := panes.SetNoticeElapsed(tabs, gone+panes.NoticeReturn)
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

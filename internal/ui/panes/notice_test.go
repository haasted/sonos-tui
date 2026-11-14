package panes_test

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/haasted/sonos-tui/internal/ui/panes"
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

// Case: the text a notice carries, which is all of it. A level used to lead
// with a mark of its own, and the rule now holds a notice after every command:
// a glyph saying "this is information" in front of "paused" was noise.
func TestNotice_CarriesItsTextAndNoMark(t *testing.T) {
	for _, level := range []panes.NoticeLevel{panes.NoticeInfo, panes.NoticeWarn, panes.NoticeError} {
		view := plainNotice(show(noticeTabs(), level, "paused").View())

		line, found := lineHolding(view, "paused")
		assert.True(t, found, "the notice is not on the rule")

		// The rule is everything either side, so what is left when it is
		// trimmed away is the whole of what the notice drew. Compared rather
		// than searched: a search for the old glyphs would pass on any other.
		assert.Equal(t, " paused ", strings.Trim(line, string(noticeRuleGlyph)),
			"a notice must carry its text and nothing else")
	}
}

// noticeRuleGlyph is the character the divider is drawn from.
const noticeRuleGlyph = '─'

// lineHolding is the rendered line carrying want.
func lineHolding(view, want string) (string, bool) {
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, want) {
			return line, true
		}
	}
	return "", false
}

// Case: the severities, which must still be told apart now that no glyph does
// it. Colour is what is left, and the three must not share one.
func TestNotice_EachLevelLooksDifferent(t *testing.T) {
	seen := map[string]bool{}
	for _, level := range []panes.NoticeLevel{panes.NoticeInfo, panes.NoticeWarn, panes.NoticeError} {
		seen[show(noticeTabs(), level, "something happened").View()] = true
	}
	assert.Len(t, seen, 3, "the three levels must not draw the same")
}

// plainNotice drops the escapes, so a search for a glyph cannot match a colour
// code that happens to hold the same bytes.
func plainNotice(view string) string {
	return ansiEscape.ReplaceAllString(view, "")
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

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
// keep its successor alive, which is what generations are for.
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

// Case: a confirmation arriving while a refusal is on the rule. The refusal
// explains why nothing happened, and a confirmation would bury the one message
// the reader needs. Both are NoticeInfo, so the level cannot decide this.
func TestNotice_AConfirmationYieldsToWhatIsAlreadyThere(t *testing.T) {
	tabs := show(noticeTabs(), panes.NoticeInfo, "a station plays forward only")

	tabs = confirm(tabs, "next track")

	assert.Contains(t, tabs.View(), "a station plays forward only")
	assert.NotContains(t, tabs.View(), "next track")
}

// Case: a confirmation against a fault, which is the same rule at the other
// end of the scale.
func TestNotice_AConfirmationYieldsToAFault(t *testing.T) {
	tabs := show(noticeTabs(), panes.NoticeError, "Køkken: no answer")

	tabs = confirm(tabs, "paused")

	assert.Contains(t, tabs.View(), "Køkken: no answer")
}

// Case: one command after another, which must replace rather than queue behind
// each other. Yielding to everything would leave the second key unanswered.
func TestNotice_AConfirmationReplacesAnotherConfirmation(t *testing.T) {
	tabs := confirm(noticeTabs(), "paused")
	require.Contains(t, tabs.View(), "paused")

	tabs = confirm(tabs, "playing")

	assert.Contains(t, tabs.View(), "playing")
	assert.NotContains(t, tabs.View(), "paused")
}

// Case: anything that is not a confirmation, which keeps the old rule of the
// newest message winning. A fault must still interrupt a confirmation.
func TestNotice_AFaultStillTakesTheRuleFromAConfirmation(t *testing.T) {
	tabs := confirm(noticeTabs(), "next track")

	tabs = show(tabs, panes.NoticeError, "Køkken: no answer")

	assert.Contains(t, tabs.View(), "Køkken: no answer")
	assert.NotContains(t, tabs.View(), "next track")
}

// Case: a confirmation arriving to an empty rule, which is the ordinary case.
// Yielding must mean "to something", not "always".
func TestNotice_AConfirmationShowsOnAnEmptyRule(t *testing.T) {
	assert.Contains(t, confirm(noticeTabs(), "muted").View(), "muted")
}

// Case: how long each kind holds. A confirmation answers a key the reader just
// pressed and must be gone well before a refusal would be.
func TestNotice_AConfirmationHoldsForLessThanARefusal(t *testing.T) {
	brief := panes.NoticeHoldFor(panes.NoticeMsg{Confirmation: true})
	ordinary := panes.NoticeHoldFor(panes.NoticeMsg{})

	assert.Less(t, brief, ordinary, "a confirmation must go sooner than a refusal")
	assert.Less(t, brief, 2*time.Second, "a confirmation is read at a glance")
}

// Case: the entrance a confirmation arrives with. It appears many times a
// session, so it must not perform: an animation replaying on every keypress is
// what NoticeAppear exists to avoid.
func TestNotice_AConfirmationDoesNotPerform(t *testing.T) {
	tabs := confirm(noticeTabs(), "paused")
	before := tabs.View()

	tabs = confirm(tabs, "paused")

	assert.Equal(t, before, tabs.View(), "a repeat redrew the rule differently")
}

func confirm(tabs panes.Tabs, text string) panes.Tabs {
	updated, _ := tabs.Update(panes.NoticeMsg{Level: panes.NoticeInfo, Text: text, Confirmation: true})
	return updated
}

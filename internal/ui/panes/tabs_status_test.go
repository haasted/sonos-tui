package panes_test

import (
	"regexp"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/ui/panes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTabs_EveryTabCarriesAStatusGlyph(t *testing.T) {
	view := statusTabs(true, false, false).View()

	assert.Equal(t, 1, strings.Count(view, "⣶"), "the playing room should show the active-level glyph")
	assert.Equal(t, 2, strings.Count(view, "⣀"), "idle rooms must still show a glyph, not a blank")
}

func TestTabs_TogglingPlayingDoesNotMoveTabs(t *testing.T) {
	idle := lipgloss.Width(statusTabs(false, false, false).View())
	playing := lipgloss.Width(statusTabs(true, true, true).View())

	assert.Equal(t, idle, playing, "starting playback must swap the glyph without resizing the row")
}

func TestTabs_PlayingRoomReadsBrighterThanIdleRoom(t *testing.T) {
	view := statusTabs(false, true, false).View()

	playing := requireTabBrightness(t, view, "Kitchen")
	idle := requireTabBrightness(t, view, "Office")

	assert.Greater(t, playing, idle, "an unfocused playing room must stand out from an unfocused idle one")
}

func TestTabs_ActiveTabKeepsItsAccentWhetherPlayingOrNot(t *testing.T) {
	idle := requireTabForeground(t, statusTabs(false, false, false).View(), "Living Room")
	playing := requireTabForeground(t, statusTabs(true, false, false).View(), "Living Room")

	assert.Equal(t, idle, playing, "focus owns the colour; playing must not restyle the active tab")
}

func TestTabs_PlayingIsSignalledByShapeNotOnlyColour(t *testing.T) {
	playing := stripANSI(statusTabs(false, true, false).View())
	idle := stripANSI(statusTabs(false, false, false).View())

	assert.NotEqual(t, playing, idle, "the two states must differ once every colour escape is removed")
}

var (
	fgOfTab  = regexp.MustCompile(`38;2;(\d+);(\d+);(\d+)m[^\x1b]*%s`)
	ansiCode = regexp.MustCompile("\x1b\\[[0-9;]*m")
)

// statusTabs builds three rooms with tab 0 active, so the caller can set each
// room's playing state independently of which one has focus.
func statusTabs(living, kitchen, office bool) panes.Tabs {
	return panes.NewTabs([]panes.Tab{
		{Title: "Living Room", Content: panes.StaticContent("a"), Playing: living},
		{Title: "Kitchen", Content: panes.StaticContent("b"), Playing: kitchen},
		{Title: "Office", Content: panes.StaticContent("c"), Playing: office},
	})
}

// requireTabForeground returns the RGB triple applied to the named tab's label.
func requireTabForeground(t *testing.T, view, title string) [3]int {
	t.Helper()

	pattern := regexp.MustCompile(strings.Replace(fgOfTab.String(), "%s", regexp.QuoteMeta(title), 1))
	match := pattern.FindStringSubmatch(view)
	require.NotNil(t, match, "no foreground colour found for tab %q", title)

	var rgb [3]int
	for i, part := range match[1:] {
		for _, digit := range part {
			rgb[i] = rgb[i]*10 + int(digit-'0')
		}
	}
	return rgb
}

// requireTabBrightness sums a tab's colour channels, standing in for how
// strongly it reads against the terminal background.
func requireTabBrightness(t *testing.T, view, title string) int {
	t.Helper()

	rgb := requireTabForeground(t, view, title)
	return rgb[0] + rgb[1] + rgb[2]
}

func stripANSI(s string) string { return ansiCode.ReplaceAllString(s, "") }

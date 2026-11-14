package panes_test

import (
	"strings"
	"testing"

	"github.com/haasted/sonos-tui/internal/ui/panes"
	"github.com/stretchr/testify/assert"
)

func TestTabs_AddTab_AppendsWithoutChangingActive(t *testing.T) {
	tabs := panes.NewTabs([]panes.Tab{{Title: "First", Content: panes.StaticContent("a")}})
	tabs.Active = 0

	tabs = tabs.AddTab(panes.Tab{Title: "Second", Content: panes.StaticContent("b")})

	assert.Len(t, tabs.Items, 2)
	assert.Equal(t, "Second", tabs.Items[1].Title)
	assert.Equal(t, 0, tabs.Active)
}

func TestTabs_AddTab_OnEmptyTabs(t *testing.T) {
	tabs := panes.NewTabs(nil)

	tabs = tabs.AddTab(panes.Tab{Title: "Only", Content: panes.StaticContent("a")})

	assert.Len(t, tabs.Items, 1)
	assert.Equal(t, 0, tabs.Active)
}

func TestTabs_InsertTabAt_OnEmptyTabs(t *testing.T) {
	tabs := panes.NewTabs(nil)

	tabs = tabs.InsertTabAt(0, panes.Tab{Title: "Only", Content: panes.StaticContent("a")})

	assert.Len(t, tabs.Items, 1)
	assert.Equal(t, 0, tabs.Active, "the first tab ever inserted must become active, not skipped past")
}

func TestTabs_InsertTabAt_BeforeActive_ShiftsActive(t *testing.T) {
	tabs := panes.NewTabs([]panes.Tab{{Title: "B", Content: panes.StaticContent("b")}})
	tabs.Active = 0

	tabs = tabs.InsertTabAt(0, panes.Tab{Title: "A", Content: panes.StaticContent("a")})

	assert.Equal(t, []string{"A", "B"}, titles(tabs))
	assert.Equal(t, 1, tabs.Active, "Active must keep pointing at B after A is inserted before it")
}

func TestTabs_InsertTabAt_AfterActive_LeavesActiveUnchanged(t *testing.T) {
	tabs := panes.NewTabs([]panes.Tab{{Title: "A", Content: panes.StaticContent("a")}})
	tabs.Active = 0

	tabs = tabs.InsertTabAt(1, panes.Tab{Title: "B", Content: panes.StaticContent("b")})

	assert.Equal(t, []string{"A", "B"}, titles(tabs))
	assert.Equal(t, 0, tabs.Active, "Active must keep pointing at A when B is inserted after it")
}

func TestTabs_InsertTabAt_InMiddle_ShiftsActiveAtBoundary(t *testing.T) {
	tabs := panes.NewTabs([]panes.Tab{
		{Title: "A", Content: panes.StaticContent("a")},
		{Title: "C", Content: panes.StaticContent("c")},
	})
	tabs.Active = 1 // pointing at C

	tabs = tabs.InsertTabAt(1, panes.Tab{Title: "B", Content: panes.StaticContent("b")})

	assert.Equal(t, []string{"A", "B", "C"}, titles(tabs))
	assert.Equal(t, 2, tabs.Active, "Active must keep pointing at C, now shifted one slot right")
}

func titles(tabs panes.Tabs) []string {
	out := make([]string, len(tabs.Items))
	for i, tab := range tabs.Items {
		out[i] = tab.Title
	}
	return out
}

// Case: a speaker another one is driving, which must read as belonging to it
// rather than standing on its own -- and must not be brightened back by
// Playing, which a follower also is.
func TestTabs_AFollowerIsDimmerThanAPlayingRoom(t *testing.T) {
	tabs := panes.NewTabs([]panes.Tab{
		{Title: "Ovenpå", Playing: true, Content: panes.StaticContent("a")},
		{Title: "Køkken", Playing: true, Following: true, Content: panes.StaticContent("b")},
		{Title: "Spisestue", Content: panes.StaticContent("c")},
	})
	tabs.Active = 0
	tabs.Width = 60

	row := strings.Split(tabs.View(), "\n")[0]
	playing, following := styleAround(row, "Ovenpå"), styleAround(row, "Køkken")

	if following == playing {
		t.Fatalf("a follower is drawn like the room driving it: %q", following)
	}
	if following == styleAround(row, "Spisestue") {
		t.Fatalf("a follower is drawn like an idle room: %q", following)
	}
}

// Case: the follower the reader has selected. Focus wins outright, or
// selecting one would dim it out of view just as they reached for it.
func TestTabs_ASelectedFollowerStillReadsAsSelected(t *testing.T) {
	tabs := panes.NewTabs([]panes.Tab{
		{Title: "Ovenpå", Playing: true, Content: panes.StaticContent("a")},
		{Title: "Køkken", Following: true, Content: panes.StaticContent("b")},
	})
	tabs.Width = 60

	tabs.Active = 1
	selected := styleAround(strings.Split(tabs.View(), "\n")[0], "Køkken")

	tabs.Active = 0
	unselected := styleAround(strings.Split(tabs.View(), "\n")[0], "Køkken")

	if selected == unselected {
		t.Fatal("selecting a follower changed nothing about how it is drawn")
	}
}

// styleAround is the escape sequence a label is drawn with.
func styleAround(row, label string) string {
	for _, run := range strings.Split(row, "\x1b[") {
		if strings.Contains(run, label) {
			return strings.SplitN(run, "m", 2)[0]
		}
	}
	return ""
}

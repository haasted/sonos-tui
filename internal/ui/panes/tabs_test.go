package panes_test

import (
	"testing"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
	"github.com/stretchr/testify/assert"
)

func TestTabs_AddTab_AppendsWithoutChangingActive(t *testing.T) {
	tabs := panes.NewTabs([]panes.Tab{{Title: "First", Content: panes.StaticContent("a")}})
	tabs.Active = 0

	tabs = tabs.AddTab(panes.Tab{Title: "Second", Content: panes.StaticContent("b")})

	assert.Len(t, tabs.Tabs, 2)
	assert.Equal(t, "Second", tabs.Tabs[1].Title)
	assert.Equal(t, 0, tabs.Active)
}

func TestTabs_AddTab_OnEmptyTabs(t *testing.T) {
	tabs := panes.NewTabs(nil)

	tabs = tabs.AddTab(panes.Tab{Title: "Only", Content: panes.StaticContent("a")})

	assert.Len(t, tabs.Tabs, 1)
	assert.Equal(t, 0, tabs.Active)
}

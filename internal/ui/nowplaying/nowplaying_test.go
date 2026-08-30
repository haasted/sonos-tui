package nowplaying_test

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/nowplaying"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_View_RendersTrack(t *testing.T) {
	m := nowplaying.New(nowplaying.Track{
		Title:    "Fortunate Son",
		Artist:   "Creedence Clearwater Revival",
		Album:    "Willy and the Poor Boys",
		Elapsed:  58 * time.Second,
		Duration: 2*time.Minute + 21*time.Second,
		Playing:  true,
	})

	view := m.View()
	assert.Contains(t, view, "Fortunate Son")
	assert.Contains(t, view, "0:58 / 2:21")
}

func TestModel_TrackMsg_UpdatesView(t *testing.T) {
	m := nowplaying.New(nowplaying.Track{Title: "First Track"})

	updated, _ := m.Update(nowplaying.TrackMsg{Track: nowplaying.Track{
		Title:    "Second Track",
		Elapsed:  10 * time.Second,
		Duration: time.Minute,
	}})

	view := updated.View()
	assert.Contains(t, view, "Second Track")
	assert.NotContains(t, view, "First Track")
	assert.Contains(t, view, "0:10 / 1:00")
}

func TestModel_Elapsed_AdvancesWhilePlaying(t *testing.T) {
	m := nowplaying.New(nowplaying.Track{
		Title:    "Test",
		Duration: time.Minute,
		Playing:  true,
	})

	assert.Contains(t, m.View(), "0:00 / 1:00")
	time.Sleep(1100 * time.Millisecond)
	assert.Contains(t, m.View(), "0:01 / 1:00")
}

func TestModel_Elapsed_FrozenWhilePaused(t *testing.T) {
	m := nowplaying.New(nowplaying.Track{
		Title:    "Test",
		Elapsed:  30 * time.Second,
		Duration: time.Minute,
		Playing:  false,
	})

	assert.Contains(t, m.View(), "0:30 / 1:00")
	time.Sleep(1100 * time.Millisecond)
	assert.Contains(t, m.View(), "0:30 / 1:00")
}

func TestModel_Init_InvalidatesPreviousTickChain(t *testing.T) {
	m := nowplaying.New(nowplaying.Track{
		Title:    "This Title Is Definitely Long Enough To Need Scrolling",
		Duration: time.Minute,
	})

	staleCmd := m.Init()
	require.NotNil(t, staleCmd)

	currentCmd := m.Init()
	require.NotNil(t, currentCmd)

	viewBefore := m.View()

	staleMsg := staleCmd()
	mAfterStale, cmdAfterStale := m.Update(staleMsg)
	assert.Nil(t, cmdAfterStale, "a tick from a chain started by an earlier Init() must not perpetuate itself")
	assert.Equal(t, viewBefore, mAfterStale.View(), "a tick from a stale chain must not mutate scroll state")

	currentMsg := currentCmd()
	_, cmdAfterCurrent := m.Update(currentMsg)
	assert.NotNil(t, cmdAfterCurrent, "a tick from the chain started by the latest Init() must keep ticking")
}

const scrollingTitle = "This Title Is Definitely Long Enough To Need Scrolling"

func TestModel_ScrollTicksDoNotCrossBetweenModels(t *testing.T) {
	other := nowplaying.New(nowplaying.Track{Title: scrollingTitle})
	m := nowplaying.New(nowplaying.Track{Title: scrollingTitle})

	otherCmd := other.Init()
	require.NotNil(t, otherCmd, "a title this long should start a scroll chain")
	require.NotNil(t, m.Init())

	foreignTick := otherCmd()
	viewBefore := m.View()

	for range nowplaying.ScrollDelayTicks * 2 {
		var cmd tea.Cmd
		m, cmd = m.Update(foreignTick)
		assert.Nil(t, cmd, "a foreign model's tick must not re-arm this model's chain")
	}

	assert.Equal(t, viewBefore, m.View(), "a foreign model's ticks must not advance this model's marquee")
}

func TestModel_Elapsed_ClampsToDuration(t *testing.T) {
	m := nowplaying.New(nowplaying.Track{
		Title:    "Test",
		Elapsed:  59500 * time.Millisecond,
		Duration: time.Minute,
		Playing:  true,
	})

	time.Sleep(1100 * time.Millisecond)
	assert.Contains(t, m.View(), "1:00 / 1:00")
}

func TestModel_ErrorMsg_ShowsInView(t *testing.T) {
	m := nowplaying.New(nowplaying.Track{Title: "Test"})

	updated, _ := m.Update(nowplaying.ErrorMsg{Err: errors.New("speaker unreachable")})

	assert.Contains(t, updated.View(), "speaker unreachable")
}

func TestModel_TrackMsg_ClearsPriorError(t *testing.T) {
	m := nowplaying.New(nowplaying.Track{Title: "Test"})

	m, _ = m.Update(nowplaying.ErrorMsg{Err: errors.New("speaker unreachable")})
	require.Contains(t, m.View(), "speaker unreachable")

	m, _ = m.Update(nowplaying.TrackMsg{Track: nowplaying.Track{Title: "Test"}})

	assert.NotContains(t, m.View(), "speaker unreachable")
}

package nowplaying_test

import (
	"testing"
	"time"

	"github.com/haasted/sonos-sandbox/internal/ui/nowplaying"
	"github.com/stretchr/testify/assert"
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

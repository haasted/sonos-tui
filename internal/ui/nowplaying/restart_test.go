package nowplaying_test

import (
	"testing"
	"time"

	"github.com/haasted/sonos-tui/internal/ui/nowplaying"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a volume change, which must not fork the fade chain. Arming without
// rolling the generation first leaves the previous chain valid, so both survive
// the guard and both re-arm — and every further change adds another.
func TestRestart_AVolumeChangeDoesNotForkTheFadeChain(t *testing.T) {
	m := scrollingModel()
	_ = m.Init()
	before := nowplaying.FadeGen(m)

	m, _ = m.Update(nowplaying.VolumeMsg{Volume: 30})

	require.NotEqual(t, before, nowplaying.FadeGen(m), "the fade generation must roll")

	_, cmd := m.Update(nowplaying.NewFadeTick(before))
	assert.Nil(t, cmd, "a tick from the pre-change fade chain must not re-arm")
}

// Case: the marquee during a volume change, which must be left entirely alone.
// A held volume key emits a message every volumeDebounce, and that equals
// scrollInterval — so rolling the scroll generation here discards every scroll
// tick before it can fire and the title stops moving until the key is released.
func TestRestart_AVolumeChangeLeavesTheScrollChainRunning(t *testing.T) {
	m := scrollingModel()
	_ = m.Init()
	before := nowplaying.ScrollGen(m)

	m, _ = m.Update(nowplaying.VolumeMsg{Volume: 30})

	assert.Equal(t, before, nowplaying.ScrollGen(m), "volume must not touch the marquee's chain")

	_, cmd := m.Update(nowplaying.NewScrollTick(before))
	assert.NotNil(t, cmd, "the in-flight scroll tick must still be accepted")
}

// Case: a held volume key, which is a stream of changes at exactly the scroll
// cadence. The marquee must keep advancing throughout.
func TestRestart_AHeldVolumeKeyDoesNotStallTheMarquee(t *testing.T) {
	m := scrollingModel()
	_ = m.Init()
	for range nowplaying.ScrollDelayTicks {
		m, _ = m.Update(nowplaying.NewScrollTick(nowplaying.ScrollGen(m)))
	}
	before := nowplaying.VisibleTitle(m)

	for volume := range 5 {
		m, _ = m.Update(nowplaying.VolumeMsg{Volume: volume})
		m, _ = m.Update(nowplaying.NewScrollTick(nowplaying.ScrollGen(m)))
	}

	assert.NotEqual(t, before, nowplaying.VisibleTitle(m),
		"the title must still be travelling while the volume is held")
}

// Case: a new track, where everything genuinely changed and every chain should
// be replaced — the case restart exists for.
func TestRestart_ATrackChangeRollsEveryChain(t *testing.T) {
	m := scrollingModel()
	_ = m.Init()
	scroll, fade := nowplaying.ScrollGen(m), nowplaying.FadeGen(m)

	m, _ = m.Update(nowplaying.TrackMsg{Track: nowplaying.Track{
		Title: "Something Else Entirely, Also Too Long For The Window", Playing: true,
	}})

	assert.NotEqual(t, scroll, nowplaying.ScrollGen(m))
	assert.NotEqual(t, fade, nowplaying.FadeGen(m))

	_, cmd := m.Update(nowplaying.NewScrollTick(scroll))
	assert.Nil(t, cmd, "the previous track's scroll chain must be dead")
}

func scrollingModel() nowplaying.Model {
	return nowplaying.New(nowplaying.Track{
		Title:    "A Title Comfortably Longer Than The Window It Has To Fit Inside",
		Playing:  true,
		Duration: 200 * time.Second,
	})
}

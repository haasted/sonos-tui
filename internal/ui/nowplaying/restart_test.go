package nowplaying_test

import (
	"testing"
	"time"

	"github.com/haasted/sonos-sandbox/internal/ui/nowplaying"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a volume change while the marquee is scrolling. Arming a chain without
// rolling the generation first leaves the previous one valid, so both survive
// the guard and both re-arm — the marquee then runs at double speed, and faster
// again for every further change.
func TestRestart_AVolumeChangeDoesNotForkTheScrollChain(t *testing.T) {
	m := scrollingModel()
	_ = m.Init()
	before := nowplaying.ScrollGen(m)

	m, _ = m.Update(nowplaying.VolumeMsg{Volume: 30})

	require.NotEqual(t, before, nowplaying.ScrollGen(m), "the generation must roll")

	_, cmd := m.Update(nowplaying.NewScrollTick(before))
	assert.Nil(t, cmd, "a tick from the pre-change chain must not re-arm")
}

// Case: repeated volume changes, which are what a single held arrow key
// produces. Each one used to add another live chain.
func TestRestart_RepeatedVolumeChangesLeaveOneChain(t *testing.T) {
	m := scrollingModel()
	_ = m.Init()

	var stale []int
	for volume := range 5 {
		stale = append(stale, nowplaying.ScrollGen(m))
		m, _ = m.Update(nowplaying.VolumeMsg{Volume: volume})
	}

	for _, gen := range stale {
		_, cmd := m.Update(nowplaying.NewScrollTick(gen))
		assert.Nil(t, cmd, "generation %d should be dead", gen)
	}

	_, cmd := m.Update(nowplaying.NewScrollTick(nowplaying.ScrollGen(m)))
	assert.NotNil(t, cmd, "the current chain must still be alive")
}

// Case: the surviving chain, which must advance the marquee once per tick
// however many volume changes preceded it.
func TestRestart_ScrollSpeedIsUnaffectedByVolumeChanges(t *testing.T) {
	quiet, noisy := scrollingModel(), scrollingModel()
	_, _ = quiet.Init(), noisy.Init()
	for volume := range 5 {
		noisy, _ = noisy.Update(nowplaying.VolumeMsg{Volume: volume})
	}

	assert.Equal(t, advanceOnce(quiet), advanceOnce(noisy),
		"the marquee must travel the same distance per tick either way")
}

// advanceOnce delivers enough ticks to clear the scroll delay plus one, and
// returns the marquee window alone — comparing whole views would conflate the
// scroll position with the volume readout a VolumeMsg also raises.
func advanceOnce(m nowplaying.Model) string {
	for range nowplaying.ScrollDelayTicks + 1 {
		m, _ = m.Update(nowplaying.NewScrollTick(nowplaying.ScrollGen(m)))
	}
	return nowplaying.VisibleTitle(m)
}

func scrollingModel() nowplaying.Model {
	return nowplaying.New(nowplaying.Track{
		Title:    "A Title Comfortably Longer Than The Window It Has To Fit Inside",
		Playing:  true,
		Duration: 200 * time.Second,
	})
}

package nowplaying_test

import (
	"strings"
	"testing"

	"github.com/haasted/sonos-tui/internal/ui/nowplaying"
	"github.com/stretchr/testify/assert"
)

// Case: a stream, whose label must stand in for the position rather than
// appear beside it — a countdown against a zero duration is the thing being
// replaced.
func TestStreamLabel_ReplacesThePosition(t *testing.T) {
	m := nowplaying.New(nowplaying.Track{Title: "DR P1", Playing: true, StreamLabel: "live"})

	view := m.View()

	assert.Contains(t, view, "live")
	assert.NotContains(t, view, "0:00 / 0:00", "the meaningless clock must be gone, not merely joined")
}

// Case: an ordinary track, which must keep its clock and show no label.
func TestStreamLabel_AbsentForARealTrack(t *testing.T) {
	m := nowplaying.New(nowplaying.Track{
		Title: "Party", Playing: true, Duration: 199 * 1e9,
	})

	view := m.View()

	assert.Contains(t, view, "/ 3:19")
	assert.NotContains(t, view, "live")
	assert.NotContains(t, view, "radio")
}

// Case: the label's idle glint, which must brighten it and then settle back.
// The light is what says a stream is alive, since its clock cannot.
func TestStreamLabel_GlintsThenSettles(t *testing.T) {
	m := nowplaying.New(nowplaying.Track{Title: "DR P1", Playing: true, StreamLabel: "live"})
	settled := distinctForegrounds(m.View())

	m = nowplaying.SweepLabel(m)

	assert.Greater(t, distinctForegrounds(m.View()), settled,
		"a running glint must paint the label in more than one colour")
}

// Case: the frame width, which the label must not change — it shares its slot
// with the volume readout and the clock.
func TestStreamLabel_DoesNotChangeTheLayoutWidth(t *testing.T) {
	track := nowplaying.Track{Title: "DR P1", Playing: true}
	plain := nowplaying.New(track)
	track.StreamLabel = "live"
	labelled := nowplaying.New(track)

	assert.Equal(t, lineWidth(plain.View()), lineWidth(labelled.View()))
}

func distinctForegrounds(s string) int {
	seen := map[string]bool{}
	for _, part := range strings.Split(s, "\x1b[") {
		if index := strings.Index(part, "m"); index > 0 && strings.Contains(part[:index], "38;2;") {
			seen[part[:index]] = true
		}
	}
	return len(seen)
}

func lineWidth(view string) int {
	widest := 0
	for _, line := range strings.Split(view, "\n") {
		if n := len(line); n > widest {
			widest = n
		}
	}
	return widest
}

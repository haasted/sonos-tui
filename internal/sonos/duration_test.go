package sonos_test

import (
	"testing"
	"time"

	"github.com/haasted/sonos-tui/internal/sonos"
	"github.com/stretchr/testify/assert"
)

func TestParseDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"0:01:15", 75 * time.Second},
		{"1:02:03", time.Hour + 2*time.Minute + 3*time.Second},
		{"0:00:00", 0},
		{"", 0},
		{"garbage", 0},
		{"1:2", 0},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, sonos.ParseDuration(c.in), "input %q", c.in)
	}
}

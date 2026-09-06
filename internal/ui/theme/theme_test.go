package theme

import (
	"image/color"
	"testing"
)

// Case: colours either side of the HSL-lightness 0.5 threshold that isDark
// inherits from lipgloss, including the exact boundary.
func TestIsDarkFollowsHSLLightness(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   color.Color
		want bool
	}{
		{"black", rgb(0x00, 0x00, 0x00), true},
		{"white", rgb(0xFF, 0xFF, 0xFF), false},
		{"typical dark terminal", rgb(0x1E, 0x1E, 0x1E), true},
		{"typical light terminal", rgb(0xFA, 0xFA, 0xFA), false},
		{"mid grey just below 0.5", rgb(0x7F, 0x7F, 0x7F), true},
		{"mid grey at 0.5", rgb(0x80, 0x80, 0x80), false},
		{"saturated red is dark by lightness", rgb(0xC0, 0x1C, 0x28), true},
	} {
		if got := isDark(tc.in); got != tc.want {
			t.Errorf("%s: isDark = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Case: an absent background, which every caller may see if the terminal
// never answers the query.
func TestIsDarkTreatsUnknownAsDark(t *testing.T) {
	if !isDark(nil) {
		t.Fatal("a nil background must be treated as dark, matching lipgloss")
	}
}

func rgb(r, g, b uint8) color.Color { return color.RGBA{R: r, G: g, B: b, A: 0xFF} }

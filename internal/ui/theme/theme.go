package theme

import (
	"image/color"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/lucasb-eyer/go-colorful"
)

// background is the terminal's own background, or nil if it did not report one.
// Queried once at init; the same single query also decides LightDark, so adding
// Background costs no extra round trip to the terminal.
var background, _ = lipgloss.BackgroundColor(os.Stdin, os.Stdout)

// LightDark picks between a light-background and a dark-background colour.
var LightDark = lipgloss.LightDark(isDark(background))

// Background returns the terminal's background colour, or nil if unreported.
// Callers fading a colour toward the terminal's own backdrop need it; those
// that cannot handle nil should fall back to LightDark.
func Background() color.Color { return background }

// isDark mirrors lipgloss's own unexported isDarkColor: HSL lightness below
// 0.5, treating an unreadable or absent colour as dark.
func isDark(c color.Color) bool {
	if c == nil {
		return true
	}
	col, ok := colorful.MakeColor(c)
	if !ok {
		return true
	}
	_, _, l := col.Hsl()
	return l < 0.5
}

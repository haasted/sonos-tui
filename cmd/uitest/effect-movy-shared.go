package main

import (
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/movy"
	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

var movyStyle = lipgloss.NewStyle().Padding(1, 2)

func themedV3(light, dark string) movy.V3 {
	c := theme.LightDark(lipgloss.Color(light), lipgloss.Color(dark))
	r, g, b, _ := c.RGBA()
	return movy.FromRgb(movy.Rgb{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8)})
}

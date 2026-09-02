package main

import (
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/movy"
	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

var movyStyle = lipgloss.NewStyle().Padding(1, 2)

func themedV3(light, dark string) movy.V3 {
	return flashsweep.FromColor(theme.LightDark(lipgloss.Color(light), lipgloss.Color(dark)))
}

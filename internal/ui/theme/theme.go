package theme

import (
	"os"

	"charm.land/lipgloss/v2"
)

var LightDark = lipgloss.LightDark(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))

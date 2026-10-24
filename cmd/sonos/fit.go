package main

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
)

// The smallest terminal the program will draw in. The width is the now-playing
// panel's; the height is the smallest modal and its margin.
const (
	minTerminalWidth  = 54
	minTerminalHeight = 10
)

// preflight measures the terminal before the program starts. Refusing from
// inside it would quit before the terminal had answered its startup queries,
// and those answers would spill onto the shell's prompt.
func preflight() (string, bool) {
	width, height, err := term.GetSize(os.Stdout.Fd())
	if err != nil {
		return "", true // not a terminal, so nothing to measure
	}
	if width >= minTerminalWidth && height >= minTerminalHeight {
		return "", true
	}
	return tooSmallText(width, height), false
}

// tooSmallText says how much room the program needs, and how much it has.
func tooSmallText(width, height int) string {
	return fmt.Sprintf("sonos needs a terminal of at least %d×%d (this one is %d×%d)",
		minTerminalWidth, minTerminalHeight, width, height)
}

// fits reports whether the terminal is large enough to draw in. Before the
// first size arrives nothing is known, so nothing is refused.
func (m appModel) fits() bool {
	return !m.sized || (m.width >= minTerminalWidth && m.height >= minTerminalHeight)
}

// tooSmallView stands in for the whole UI while the terminal is too small,
// wrapped to its width and cut to its height.
func (m appModel) tooSmallView() string {
	text := tooSmallText(m.width, m.height)
	lines := strings.Split(lipgloss.NewStyle().Width(max(m.width, 1)).Render(text), "\n")
	return strings.Join(lines[:min(len(lines), max(m.height, 1))], "\n")
}

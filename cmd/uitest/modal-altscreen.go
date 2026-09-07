package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

// Floating modals are sized as a share of the terminal, then clamped, so they
// stay readable on a small window and stop growing on a very large one.
const (
	modalWidthShare  = 3 // divisor: two thirds of the terminal width
	modalHeightShare = 3
	modalMinWidth    = 34
	modalMaxWidth    = 76
	modalMinHeight   = 8
	modalMaxHeight   = 22
)

// modalMode is how the overlay occupies the screen. Both modes were kept
// deliberately: they suit different content, and the choice is a per-frame
// boolean, so one app can use each where it fits.
type modalMode int

const (
	// modeHover composites a sized box over the app's own frame, claiming no
	// alt screen. Keeps the terminal-native feel, but grows the inline frame.
	modeHover modalMode = iota

	// modeFull fills the terminal via the alt screen. Unlimited height and no
	// scroll risk, at the cost of blanking the screen while open.
	modeFull

	modeCount
)

func (m modalMode) String() string {
	return [...]string{"hover (no alt screen)", "full screen (alt screen)"}[m]
}

// altScreenModal is the son-911 prototype. Its content is throwaway — this
// exists to prove the mechanism, not the widget.
type altScreenModal struct {
	open    bool
	mode    modalMode
	resizes int // resizes seen while open, evidence for the resize question
	width   int
	height  int
}

// altScreen reports whether this mode needs the alternate buffer. Hovering
// does not: it composites into the frame Bubbletea already manages.
func (m altScreenModal) altScreen() bool { return m.mode == modeFull }

// cycle steps to the next mode.
func (m altScreenModal) cycle() altScreenModal {
	m.mode = (m.mode + 1) % modeCount
	return m
}

// toggle opens or closes the modal, resetting the resize count so each opening
// reports only its own.
func (m altScreenModal) toggle() altScreenModal {
	m.open = !m.open
	m.resizes = 0
	return m
}

// ViewOver composites the modal onto the app's own view.
//
// The alt screen starts blank, so in full mode the background is drawn again
// here and padded to the terminal — it is not the inline frame showing through.
// Hover mode keeps the app at its natural height instead, which is what makes
// the box sit over the existing frame rather than replace the screen.
func (m altScreenModal) ViewOver(background string) string {
	if m.width < modalMinWidth || m.height < modalMinHeight {
		return "terminal too small for the modal prototype"
	}

	backdrop := background
	if m.altScreen() {
		backdrop = lipgloss.NewStyle().Width(m.width).Height(m.height).Render(background)
	}

	box := m.box()
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(backdrop),
		lipgloss.NewLayer(box).
			X(max((lipgloss.Width(backdrop)-lipgloss.Width(box))/2, 0)).
			Y(max((lipgloss.Height(backdrop)-lipgloss.Height(box))/2, 0)).
			Z(1),
	).Render()
}

// box renders the modal itself at whichever size the current mode calls for.
func (m altScreenModal) box() string {
	width, height := m.width, m.height
	if m.mode != modeFull {
		width = clamp(m.width*(modalWidthShare-1)/modalWidthShare, modalMinWidth, modalMaxWidth)
		height = clamp(m.height*(modalHeightShare-1)/modalHeightShare, modalMinHeight, modalMaxHeight)
	}

	inner := height - 2 // the border's own two rows
	lines := []string{
		modalTitleStyle.Render("Modal prototype (son-911)"),
		"",
		fmt.Sprintf("terminal      %d×%d", m.width, m.height),
		fmt.Sprintf("modal         %d×%d", width, height),
		fmt.Sprintf("mode          %s", m.mode),
		fmt.Sprintf("resizes open  %d", m.resizes),
		"",
		modalHintStyle.Render("m switches hover/full screen · ? or esc closes"),
		"",
	}
	// Numbered filler, so anything surviving the close reads as recognisable
	// debris rather than plausible blank space.
	for i := len(lines); i < inner; i++ {
		lines = append(lines, modalFillStyle.Render(fmt.Sprintf("%3d %s", i, strings.Repeat("·", width-8))))
	}

	return modalBoxStyle.Width(width).Height(inner).Render(strings.Join(lines[:inner], "\n"))
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }

var (
	modalBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24")))

	modalTitleStyle = lipgloss.NewStyle().Bold(true).
			Foreground(theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24")))

	modalHintStyle = lipgloss.NewStyle().
			Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))

	modalFillStyle = lipgloss.NewStyle().Faint(true).
			Foreground(theme.LightDark(lipgloss.Color("#8A8A8A"), lipgloss.Color("#5A5A5A")))
)

package marquee

import "charm.land/lipgloss/v2"

type Line struct {
	text        string
	windowWidth int
	runes       []rune
	offset      int
}

func NewLine(text string, windowWidth int) Line {
	return Line{text: text, windowWidth: windowWidth, runes: []rune(text + "   ")}
}

func (l Line) NeedsScroll() bool {
	return lipgloss.Width(l.text) > l.windowWidth
}

func (l Line) Advance() Line {
	l.offset = (l.offset + 1) % len(l.runes)
	return l
}

// RenderWidth renders the line clipped to width rather than the window it was
// built with, leaving scroll position untouched so the caller can narrow the
// line temporarily and restore it without the text jumping.
func (l Line) RenderWidth(style lipgloss.Style, width int) string {
	if width >= l.windowWidth {
		return l.Render(style)
	}
	if lipgloss.Width(l.text) <= width {
		return style.Render(l.text)
	}
	doubled := append(append([]rune{}, l.runes...), l.runes...)
	return style.Render(string(doubled[l.offset : l.offset+width]))
}

func (l Line) Render(style lipgloss.Style) string {
	if !l.NeedsScroll() {
		return style.Render(l.text)
	}
	doubled := append(append([]rune{}, l.runes...), l.runes...)
	visible := string(doubled[l.offset : l.offset+l.windowWidth])
	return style.Render(visible)
}

package marquee

import "github.com/charmbracelet/lipgloss"

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

func (l Line) Render(style lipgloss.Style) string {
	if !l.NeedsScroll() {
		return style.Render(l.text)
	}
	doubled := append(append([]rune{}, l.runes...), l.runes...)
	visible := string(doubled[l.offset : l.offset+l.windowWidth])
	return style.Render(visible)
}

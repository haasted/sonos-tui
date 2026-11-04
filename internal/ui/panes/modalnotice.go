package panes

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// Notify sets a notice into the modal's bottom border, drawn exactly as Tabs
// draws one into its divider. The caller drives the frames, as it does the
// border's other lights.
func (m Modal) Notify(msg NoticeMsg) Modal {
	m.notice = notice{text: msg.Text, level: msg.Level, entrance: msg.Entrance,
		confirmation: msg.Confirmation, shownAt: time.Now(), rule: m.borderColor()}
	return m
}

// NoticeShowing reports whether any of the notice is still on the border.
func (m Modal) NoticeShowing() bool { return !m.notice.done() }

// NoticeInterval is how often the notice needs repainting while it shows.
func (m Modal) NoticeInterval() time.Duration { return m.notice.interval() }

// noticed draws the box with the notice in its bottom border. The notice owns
// that edge while it lasts, as one owns the divider: the opening light is not
// drawn, and a bump still answers on the top edge.
func (m Modal) noticed(body string, width, height int) string {
	lines := strings.Split(m.frameStyle().Width(width).Height(height).Render(body), "\n")
	drawn := lipgloss.Width(lines[0])
	if m.Light != nil {
		lines[0] = m.edge(ModalTop, drawn)
		m.sides(lines)
	}

	border := lipgloss.RoundedBorder()
	run := strings.Repeat(border.Bottom, max(drawn-2, 0))
	lines[len(lines)-1] = m.borderStyle().Render(border.BottomLeft) +
		m.notice.render(run) + m.borderStyle().Render(border.BottomRight)
	return strings.Join(lines, "\n")
}

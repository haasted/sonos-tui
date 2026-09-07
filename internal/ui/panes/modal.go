package panes

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

// ModalMode is how an open modal occupies the screen. Both are supported
// because they suit different content, and because the choice costs nothing:
// the alternate screen is a per-frame property in Bubbletea v2.
type ModalMode int

const (
	// ModalHover composites a sized box over the caller's own frame, claiming
	// no alternate screen. It keeps the surrounding UI visible, but grows the
	// inline frame, which pushes terminal scrollback up.
	ModalHover ModalMode = iota

	// ModalFullScreen fills the terminal via the alternate screen. Unlimited
	// height and no scroll cost, at the price of blanking the screen.
	ModalFullScreen
)

// Hovering modals are sized as a share of the terminal, then clamped, so they
// stay readable in a small window and stop growing in a large one. The upper
// bounds matter: a frame taller than the terminal has its top lines dropped by
// the renderer, which would silently cut off the modal's own first row.
const (
	modalWidthNumerator  = 2
	modalHeightNumerator = 2
	modalDenominator     = 3
	modalMinWidth        = 34
	modalMaxWidth        = 76
	modalMinHeight       = 8
	modalMaxHeight       = 22
)

// Modal is an overlay drawn on top of another view. It owns only its own
// framing, sizing and dismissal; what it contains, when it opens, and what the
// caller does underneath are all the caller's business.
type Modal struct {
	Content tea.Model // rendered inside the box; nil renders an empty modal
	Mode    ModalMode

	open   bool
	width  int // terminal width, not the box's
	height int
}

func NewModal(content tea.Model) Modal {
	return Modal{Content: content}
}

func (m Modal) IsOpen() bool { return m.open }

func (m Modal) Open() Modal {
	m.open = true
	return m
}

func (m Modal) Close() Modal {
	m.open = false
	return m
}

func (m Modal) Toggle() Modal {
	m.open = !m.open
	return m
}

// SetSize records the terminal's dimensions, which the modal sizes itself
// against. Callers should feed it every tea.WindowSizeMsg.
func (m Modal) SetSize(width, height int) Modal {
	m.width, m.height = width, height
	return m
}

// AltScreen reports whether the caller should set View.AltScreen this frame.
// Modal deliberately does not build the view itself: that would tie a reusable
// component to one program's rendering lifecycle.
func (m Modal) AltScreen() bool {
	return m.open && m.Mode == ModalFullScreen
}

// BoxSize is the modal's outer size, border included.
func (m Modal) BoxSize() (width, height int) {
	if m.Mode == ModalFullScreen {
		return m.width, m.height
	}
	// The terminal wins over the minimum: on a window narrower or shorter than
	// the floor, honouring the floor would push the modal off the screen, and
	// the renderer answers an oversized frame by dropping its top lines.
	width = min(clamp(m.width*modalWidthNumerator/modalDenominator, modalMinWidth, modalMaxWidth), m.width)
	height = min(clamp(m.height*modalHeightNumerator/modalDenominator, modalMinHeight, modalMaxHeight), m.height)
	return width, height
}

// Update dismisses the modal on esc and forwards everything else to its
// content. Callers must stop routing messages elsewhere while IsOpen: an open
// modal owns the keyboard, or the UI behind it keeps reacting unseen.
func (m Modal) Update(msg tea.Msg) (Modal, tea.Cmd) {
	if !m.open {
		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "esc" {
		return m.Close(), nil
	}
	if m.Content == nil {
		return m, nil
	}

	content, cmd := m.Content.Update(msg)
	m.Content = content
	return m, cmd
}

// ViewOver composites the modal onto the view behind it, centred.
//
// The two modes differ in what "behind" means. ModalFullScreen draws onto the
// alternate screen, which starts blank, so background is redrawn here padded to
// the terminal — the caller's inline frame does not show through it.
// ModalHover keeps background at its natural size, so the modal sits within the
// caller's own frame and the surrounding UI stays visible.
func (m Modal) ViewOver(background string) string {
	if !m.open {
		return background
	}

	backdrop := background
	if m.Mode == ModalFullScreen {
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

// box renders the border and the content inside it, clipped to the box so
// oversized content cannot push the frame past the terminal.
func (m Modal) box() string {
	width, height := m.BoxSize()

	var body string
	if m.Content != nil {
		body = m.Content.View().Content
	}

	inner := height - 2 // the border's own two rows
	if lines := strings.Split(body, "\n"); len(lines) > inner {
		body = strings.Join(lines[:inner], "\n")
	}

	// Width counts the border, Height does not — an asymmetry in lipgloss
	// that silently costs two columns if the box is sized as if they matched.
	return modalStyle.Width(width).Height(inner).Render(body)
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }

var modalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24"))).
	Padding(0, 1)

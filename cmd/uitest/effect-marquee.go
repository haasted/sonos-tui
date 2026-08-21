package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// marqueeText is scrolled across marqueeWidth — deliberately longer than
// the visible window, with trailing spaces so one loop doesn't run
// straight into the next. Kept to single-width characters on purpose: the
// window below slides by rune count, and a double-width rune (emoji, CJK)
// crossing the window edge would make the rendered box jitter a column
// wide, since terminal column width and rune count aren't the same thing.
const marqueeText = "★ Now scrolling: this text is wider than its box and loops forever, like a stock ticker. ★    "

const (
	marqueeWidth        = 28
	marqueeTickInterval = 120 * time.Millisecond
)

// marqueeEffect scrolls text that's wider than its display window,
// wrapping around to the start once the end is reached — for content too
// long to fit its allocated space.
type marqueeEffect struct {
	runes  []rune // marqueeText once; View doubles it to wrap the window for free
	offset int    // index into runes where the visible window starts
}

func newMarqueeEffect() marqueeEffect {
	return marqueeEffect{runes: []rune(marqueeText)}
}

type marqueeTickMsg time.Time

func marqueeTick() tea.Cmd {
	return tea.Tick(marqueeTickInterval, func(t time.Time) tea.Msg {
		return marqueeTickMsg(t)
	})
}

func (m marqueeEffect) Init() tea.Cmd {
	return marqueeTick()
}

func (m marqueeEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(marqueeTickMsg); !ok {
		return m, nil
	}
	m.offset = (m.offset + 1) % len(m.runes)
	return m, marqueeTick()
}

func (m marqueeEffect) View() string {
	// Doubling the text and sliding a window across it wraps around for
	// free — no special-casing the point where the window would otherwise
	// run past the end of the string.
	doubled := append(append([]rune{}, m.runes...), m.runes...)
	visible := string(doubled[m.offset : m.offset+marqueeWidth])
	return marqueeStyle.Render(visible)
}

var marqueeStyle = lipgloss.NewStyle().
	Bold(true).
	Padding(1, 3).
	Background(lipgloss.Color("#0B6E4F")).
	Foreground(lipgloss.Color("#FFFFFF"))

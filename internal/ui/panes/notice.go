package panes

import (
	"fmt"
	"image/color"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

// NoticeLevel is how severe a notice is. Each level carries a distinct glyph
// as well as a colour: colour alone fails on monochrome terminals and for
// colour-blind readers.
type NoticeLevel int

const (
	NoticeInfo NoticeLevel = iota
	NoticeWarn
	NoticeError
)

const (
	noticeHold     = 4 * time.Second        // fully lit before it starts to go
	noticeFade     = 900 * time.Millisecond // eased fade back to the plain rule
	noticeInterval = 40 * time.Millisecond  // repaint rate while fading
	noticeLead     = 4                      // rule drawn before the label starts
)

// NoticeMsg shows a notice. Sending one replaces whatever is currently
// showing: the newest problem is the one worth reading, and the log remains
// the complete record.
type NoticeMsg struct {
	Level NoticeLevel
	Text  string
}

// noticeTickMsg is one frame of a notice's fade.
type noticeTickMsg struct {
	gen int
}

var noticeGenerations atomic.Int64

// nextNoticeGen returns a generation unique across every notice in the
// process, so a tick from a replaced notice cannot drive its successor.
func nextNoticeGen() int { return int(noticeGenerations.Add(1)) }

func noticeTick(gen int) tea.Cmd {
	return tea.Tick(noticeInterval, func(time.Time) tea.Msg {
		return noticeTickMsg{gen: gen}
	})
}

// notice is the message currently set into the divider rule, if any.
type notice struct {
	text    string
	level   NoticeLevel
	shownAt time.Time // zero when nothing is showing
}

// strength is 1 while held, eased to 0 across the fade, and 0 once done.
func (n notice) strength() float64 {
	if n.shownAt.IsZero() {
		return 0
	}
	since := time.Since(n.shownAt)
	if since < noticeHold {
		return 1
	}
	left := noticeHold + noticeFade - since
	if left <= 0 {
		return 0
	}
	t := float64(left) / float64(noticeFade)
	return t * t * (3 - 2*t) // smoothstep, matching the volume and mute fades
}

// render sets the notice into a plain rule of the given width, leaving the
// rule visible either side so the line still reads as a divider.
func (n notice) render(run string) string {
	strength := n.strength()
	if strength <= 0 {
		return separatorStyle.Render(run)
	}

	width := lipgloss.Width(run)
	label := truncate(" "+noticeGlyph(n.level)+" "+n.text+" ", width-noticeLead)

	trailing := max(width-noticeLead-lipgloss.Width(label), 0)
	return separatorStyle.Render(strings.Repeat("─", noticeLead)) +
		noticeStyle(n.level, strength).Render(label) +
		separatorStyle.Render(strings.Repeat("─", trailing))
}

// truncate clips to a display width, marking the cut with an ellipsis. It
// walks runes rather than bytes: room names carry æ, ø and å, and slicing a
// multi-byte rune in half would corrupt the output rather than shorten it.
func truncate(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	if width < 1 {
		return ""
	}

	var (
		kept  []rune
		total int
	)
	for _, r := range s {
		w := lipgloss.Width(string(r))
		if total+w > width-1 { // leave a column for the ellipsis
			break
		}
		kept = append(kept, r)
		total += w
	}
	return string(kept) + "…"
}

func noticeGlyph(level NoticeLevel) string {
	switch level {
	case NoticeError:
		return "✖"
	case NoticeWarn:
		return "⚠"
	default:
		return "ℹ"
	}
}

// noticeStyle fades the level's colour toward the divider's resting tone, so
// the notice dissolves into the rule rather than blinking out of it.
func noticeStyle(level NoticeLevel, strength float64) lipgloss.Style {
	return lipgloss.NewStyle().Bold(strength > 0.5).
		Foreground(blendToward(separatorColor, noticeColor(level), strength))
}

func noticeColor(level NoticeLevel) color.Color {
	switch level {
	case NoticeError:
		return theme.LightDark(lipgloss.Color("#B91C1C"), lipgloss.Color("#F87171"))
	case NoticeWarn:
		return theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24"))
	default:
		return theme.LightDark(lipgloss.Color("#1D4ED8"), lipgloss.Color("#93C5FD"))
	}
}

// blendToward mixes from -> to as amount goes 0 -> 1, so a notice dissolves
// into the rule instead of blinking out of it.
func blendToward(from, to color.Color, amount float64) color.Color {
	fr, fg, fb, _ := from.RGBA()
	tr, tg, tb, _ := to.RGBA()
	mix := func(a, b uint32) int {
		return int(float64(a>>8) + (float64(b>>8)-float64(a>>8))*amount)
	}
	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X",
		mix(fr, tr), mix(fg, tg), mix(fb, tb)))
}

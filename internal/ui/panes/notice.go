package panes

import (
	"fmt"
	"image/color"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

// NoticeLevel is how severe a notice is. It shows as a colour, and as how long
// the notice holds: a confirmation goes in a moment where a fault stays, and it
// yields the rule rather than taking it.
//
// Each level carried a glyph as well, so that the severity read without colour.
// That was dropped: the rule now carries a notice after every command, and a
// mark saying "this is information" in front of the word "paused" was noise on
// the message a reader sees most. Where a level has to read without colour, the
// words can carry it -- a fault that begins "could not ..." needs no mark.
type NoticeLevel int

const (
	NoticeInfo NoticeLevel = iota
	NoticeWarn
	NoticeError
)

const (
	noticeHold     = 4 * time.Second         // fully lit before it starts to go
	noticeBrief    = 1200 * time.Millisecond // a confirmation's hold, which is read at a glance
	noticeFade     = 900 * time.Millisecond  // eased fade back to the plain rule
	noticeInterval = 40 * time.Millisecond   // repaint rate while fading
	noticeFramed   = 16 * time.Millisecond   // repaint rate while a notice is opening or collapsing
	noticeLead     = 4                       // shortest run of rule left either side of the label
	noticeRule     = '─'                     // the glyph the divider is drawn from
)

// NoticeEntrance is how a notice arrives. The zero value, NoticeAppear, is no
// animation at all — a warning should not perform, since the thing it warns
// about is what deserves the attention.
type NoticeEntrance int

const (
	NoticeAppear NoticeEntrance = iota
	NoticeMirror
	NoticePrism
	NoticeBloom
	NoticeSplit
)

// style maps an entrance onto the component that draws it.
func (e NoticeEntrance) style() flashsweep.RevealStyle {
	switch e {
	case NoticePrism:
		return flashsweep.RevealPrism
	case NoticeBloom:
		return flashsweep.RevealBloom
	case NoticeSplit:
		return flashsweep.RevealSplit
	default:
		return flashsweep.RevealMirror
	}
}

// NoticeMsg shows a notice. Sending one replaces whatever is currently
// showing: the newest problem is the one worth reading, and the log remains
// the complete record.
type NoticeMsg struct {
	Level NoticeLevel
	Text  string

	// Entrance opens the notice out of the rule and collapses it back into the
	// rule when it goes, rather than simply appearing and fading. Reserve it
	// for a notice nobody asked for, which has to earn its glance.
	Entrance NoticeEntrance

	// Confirmation says this only answers a key the reader just pressed. Such a
	// notice holds briefly and yields: it replaces another confirmation and
	// nothing else, so an error or a refusal keeps the rule it is on.
	//
	// One field rather than a duration and a priority, because the two always
	// travel together here and no other pairing of them has a reader.
	Confirmation bool
}

// noticeTickMsg is one frame of a notice's fade.
type noticeTickMsg struct {
	gen int
}

var noticeGenerations atomic.Int64

// nextNoticeGen returns a generation unique across every notice in the
// process, so a tick from a replaced notice cannot drive its successor.
func nextNoticeGen() int { return int(noticeGenerations.Add(1)) }

func noticeTick(gen int, every time.Duration) tea.Cmd {
	return tea.Tick(every, func(time.Time) tea.Msg {
		return noticeTickMsg{gen: gen}
	})
}

// interval is how often this notice needs repainting: an entrance moves per
// frame, where a plain fade is only a colour easing over most of a second.
func (n notice) interval() time.Duration {
	if n.entrance == NoticeAppear {
		return noticeInterval
	}
	return noticeFramed
}

// notice is the message currently set into the divider rule, if any.
type notice struct {
	text         string
	level        NoticeLevel
	entrance     NoticeEntrance
	confirmation bool
	shownAt      time.Time // zero when nothing is showing
}

// hold is how long this notice stays fully lit. A confirmation goes quickly:
// the reader pressed the key and already knows what they asked for.
func (n notice) hold() time.Duration {
	if n.confirmation {
		return noticeBrief
	}
	return noticeHold
}

// blocks reports that an arriving notice must be dropped rather than shown. A
// confirmation is the least important thing the rule can carry, so it never
// takes the rule from a refusal, a fault or a hint that is still on screen.
func (n notice) blocks(arriving NoticeMsg) bool {
	return arriving.Confirmation && !n.confirmation && n.strength() > 0
}

// strength is 1 while opening and held, eased to 0 across the exit, and 0 once
// done. An entrance shifts the whole schedule later rather than eating into
// the hold: the text is only readable once it has finished opening.
func (n notice) strength() float64 {
	if n.shownAt.IsZero() {
		return 0
	}
	since := time.Since(n.shownAt)

	lit := n.openDuration() + n.hold()
	if since < lit {
		return 1
	}

	left := lit + noticeFade - since
	if left <= 0 {
		return 0
	}
	t := float64(left) / float64(noticeFade)
	return t * t * (3 - 2*t) // smoothstep, matching the volume and mute fades
}

// label is the notice as it wants to be drawn, before any truncation. The
// spaces are the gap between the text and the rule it parts.
func (n notice) label() string {
	return " " + n.text + " "
}

// openDuration is how long the entrance runs, and zero when there is none. It
// is measured on the untruncated label: a narrow terminal only shortens what is
// drawn, so the opening finishes early rather than being cut off.
func (n notice) openDuration() time.Duration {
	if n.entrance == NoticeAppear {
		return 0
	}
	return n.reveal(1).Duration(n.label())
}

// reveal configures the component for this notice, with the divider's own rule
// as the fill so the text parts the rule and the rule closes over it again.
func (n notice) reveal(strength float64) flashsweep.Reveal {
	return flashsweep.Reveal{
		Style:     n.entrance.style(),
		Fill:      noticeRule,
		FillColor: flashsweep.FromColor(separatorColor),
		Ramp: flashsweep.Ramp{
			Base:  flashsweep.FromColor(blendToward(separatorColor, noticeColor(n.level), strength)),
			Flash: flashsweep.FromColor(blendToward(separatorColor, noticeFlash, strength)),
		},
	}
}

// render centres the notice in a plain rule of the given width, leaving the
// rule visible either side so the line still reads as a divider.
//
// Centred rather than set near the left edge, for two reasons. It gives the
// line an axis of its own instead of a ragged start that lines up with nothing
// above it, and an entrance opens from the middle of the text — so a centred
// label opens from the middle of the divider, where a left-set one would
// appear to burst from an arbitrary point a third of the way across.
func (n notice) render(run string) string {
	strength := n.strength()
	if strength <= 0 {
		return separatorStyle.Render(run)
	}

	width := lipgloss.Width(run)
	label := truncate(n.label(), width-noticeLead)

	// Centred where there is room, and otherwise pushed as far left as the
	// minimum lead allows: a long notice keeps its text rather than losing
	// characters to a margin it cannot afford.
	lead := max((width-lipgloss.Width(label))/2, noticeLead)
	trailing := max(width-lead-lipgloss.Width(label), 0)

	return separatorStyle.Render(strings.Repeat(string(noticeRule), lead)) +
		n.body(label, strength) +
		separatorStyle.Render(strings.Repeat(string(noticeRule), trailing))
}

// body draws the label itself: plainly for a notice that just appears, and
// otherwise mid-opening, held open, or mid-collapse.
func (n notice) body(label string, strength float64) string {
	if n.entrance == NoticeAppear {
		return noticeStyle(n.level, strength).Render(label)
	}

	var (
		r     = n.reveal(strength)
		since = time.Since(n.shownAt)
		open  = n.openDuration()
	)

	switch {
	case since < open:
		return r.Open(label, float32(since)/float32(open))
	case since < open+noticeHold:
		return r.Open(label, 1)
	default:
		return r.Close(label, float32(since-open-noticeHold)/float32(noticeFade))
	}
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

// noticeStyle fades the level's colour toward the divider's resting tone, so
// the notice dissolves into the rule rather than blinking out of it.
func noticeStyle(level NoticeLevel, strength float64) lipgloss.Style {
	return lipgloss.NewStyle().Bold(strength > 0.5).
		Foreground(blendToward(separatorColor, noticeColor(level), strength))
}

// noticeFlash is the hot end of an entrance. It brightens on a dark ground and
// deepens on a light one: flashing toward white on white erases the text.
var noticeFlash = theme.LightDark(lipgloss.Color("#0F172A"), lipgloss.Color("#FFFFFF"))

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

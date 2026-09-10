// Package panes holds reusable Bubbletea UI components shared across the
// real app and the cmd/uitest playground.
package panes

import (
	"image/color"
	"math/rand/v2"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

// Tab is one entry in a Tabs component: a header label plus the model
// rendered while it's active.
type Tab struct {
	Title   string
	Content tea.Model
	Playing bool // drives the status glyph; kept in sync by whoever owns the content
}

// Position controls whether the header row renders above or below the
// active tab's content.
type Position int

const (
	Top Position = iota
	Bottom
)

type Tabs struct {
	Items      []Tab
	Active     int
	Position   Position // zero value (Top) renders headers above the content
	Width      int      // 0 = unconstrained; otherwise headers scroll to fit, see headerRow
	HideBorder bool     // zero value (false) keeps the rounded border; true leaves only the divider bar between tabs and content

	sweepGen   int
	sweepStart time.Time

	dividerGen     int
	dividerStart   time.Time   // zero between sweeps
	dividerReverse bool        // this sweep travels right to left
	dividerColor   color.Color // picked once per sweep, so it does not strobe per frame

	notice    notice
	noticeGen int // guards fade ticks, so a replaced notice cannot drive its successor
}

const (
	tabSweepDuration = 620 * time.Millisecond
	tabSweepInterval = 22 * time.Millisecond
)

const (
	dividerSweepDuration = 1100 * time.Millisecond // one traversal of the divider
	dividerSweepInterval = 33 * time.Millisecond   // frame rate while sweeping
	dividerSweepMinGap   = 10 * time.Second        // shortest rest between sweeps
	dividerSweepMaxGap   = 15 * time.Second        // longest rest between sweeps
)

// DividerSweepColors is the palette a divider sweep draws from, one picked at
// random per sweep. Replace it to change the look; a one-element slice fixes
// the colour. Each entry adapts to the terminal background, since the sweep
// has to stand out against a light grey divider as well as a dark one.
var DividerSweepColors = []color.Color{
	theme.LightDark(lipgloss.Color("#6B46C1"), lipgloss.Color("#A5B4FC")), // indigo
	theme.LightDark(lipgloss.Color("#0E7490"), lipgloss.Color("#67E8F9")), // cyan
	theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24")), // amber
	theme.LightDark(lipgloss.Color("#9D174D"), lipgloss.Color("#F9A8D4")), // rose
	theme.LightDark(lipgloss.Color("#15803D"), lipgloss.Color("#86EFAC")), // green
}

var dividerGenerations atomic.Int64

func nextDividerGen() int { return int(dividerGenerations.Add(1)) }

// dividerScheduleMsg fires when a divider sweep is due to begin.
type dividerScheduleMsg struct {
	gen int
}

// dividerTickMsg is one animation frame of a running divider sweep.
type dividerTickMsg struct {
	gen int
}

// scheduleDividerSweep waits a random rest, then starts the next sweep.
func scheduleDividerSweep(gen int) tea.Cmd {
	gap := dividerSweepMinGap +
		time.Duration(rand.Int64N(int64(dividerSweepMaxGap-dividerSweepMinGap)))
	return tea.Tick(gap, func(time.Time) tea.Msg {
		return dividerScheduleMsg{gen: gen}
	})
}

func dividerTick(gen int) tea.Cmd {
	return tea.Tick(dividerSweepInterval, func(time.Time) tea.Msg {
		return dividerTickMsg{gen: gen}
	})
}

// beginDividerSweep rolls this sweep's direction and colour. Both are fixed
// for its duration: re-rolling in the render path would strobe every frame.
func (t *Tabs) beginDividerSweep() {
	t.dividerStart = time.Now()
	t.dividerReverse = rand.IntN(2) == 0
	t.dividerColor = pickDividerColor()
}

// pickDividerColor chooses from the palette, falling back to the divider's own
// tone so an emptied palette simply yields no visible sweep.
func pickDividerColor() color.Color {
	if len(DividerSweepColors) == 0 {
		return separatorColor
	}
	return DividerSweepColors[rand.IntN(len(DividerSweepColors))]
}

// dividerProgress reports how far the current sweep has travelled, as 0-1,
// and whether one is running at all.
func (t Tabs) dividerProgress() (float32, bool) {
	if t.dividerStart.IsZero() {
		return 0, false
	}
	elapsed := time.Since(t.dividerStart)
	if elapsed >= dividerSweepDuration {
		return 0, false
	}
	return float32(elapsed) / float32(dividerSweepDuration), true
}

var tabSweepGenerations atomic.Int64

func nextTabSweepGen() int { return int(tabSweepGenerations.Add(1)) }

type tabSweepTickMsg struct {
	gen int
}

func tabSweepTick(gen int) tea.Cmd {
	return tea.Tick(tabSweepInterval, func(time.Time) tea.Msg {
		return tabSweepTickMsg{gen: gen}
	})
}

func (t Tabs) activate() (Tabs, tea.Cmd) {
	initCmd := t.Items[t.Active].Content.Init()

	t.sweepGen = nextTabSweepGen()
	t.sweepStart = time.Now()

	return t, tea.Batch(initCmd, tabSweepTick(t.sweepGen))
}

func (t Tabs) sweepProgress() (float32, bool) {
	if t.sweepStart.IsZero() {
		return 0, false
	}
	elapsed := time.Since(t.sweepStart)
	if elapsed >= tabSweepDuration {
		return 0, false
	}
	return float32(elapsed) / float32(tabSweepDuration), true
}

// NewTabs builds a Tabs component, starting on the first tab.
//
// The divider generation is assigned here rather than in Init because Init has
// a value receiver: anything it assigns is written to a copy and lost, so a
// generation rolled there would never reach the model that has to recognise it.
func NewTabs(tabs []Tab) Tabs {
	return Tabs{Items: tabs, dividerGen: nextDividerGen()}
}

func (t Tabs) AddTab(tab Tab) Tabs {
	t.Items = append(t.Items, tab)
	return t
}

func (t Tabs) InsertTabAt(index int, tab Tab) Tabs {
	if index < 0 {
		index = 0
	}
	if index > len(t.Items) {
		index = len(t.Items)
	}
	hadTabs := len(t.Items) > 0

	t.Items = append(t.Items, Tab{})
	copy(t.Items[index+1:], t.Items[index:])
	t.Items[index] = tab

	if hadTabs && index <= t.Active {
		t.Active++
	}
	return t
}

// RemoveTabAt drops one tab, keeping Active pointing at the same tab where it
// can and clamping to the last tab when the removed one was at the end.
func (t Tabs) RemoveTabAt(index int) Tabs {
	if index < 0 || index >= len(t.Items) {
		return t
	}

	items := make([]Tab, 0, len(t.Items)-1)
	items = append(items, t.Items[:index]...)
	items = append(items, t.Items[index+1:]...)
	t.Items = items

	if index < t.Active {
		t.Active--
	}
	if t.Active >= len(t.Items) {
		t.Active = max(len(t.Items)-1, 0)
	}
	return t
}

// Init starts the divider's idle sweep timer and forwards to the initially
// active tab's content. The timer starts even with no tabs yet: cmd/sonos
// builds its Tabs empty and fills it as speakers are discovered.
func (t Tabs) Init() tea.Cmd {
	// Schedules against the generation NewTabs already assigned. A Tabs built
	// as a literal has generation 0 and schedules with 0, which Update still
	// accepts — degraded to a cross-talk risk between Tabs, but not silent.
	cmds := []tea.Cmd{scheduleDividerSweep(t.dividerGen)}
	if len(t.Items) > 0 {
		cmds = append(cmds, t.Items[t.Active].Content.Init())
	}
	return tea.Batch(cmds...)
}

// Update switches tabs on navigation keys, otherwise forwards to the active tab's content.
func (t Tabs) Update(msg tea.Msg) (Tabs, tea.Cmd) {
	switch msg := msg.(type) {
	case NoticeMsg:
		t.notice = notice{text: msg.Text, level: msg.Level, shownAt: time.Now()}
		t.noticeGen = nextNoticeGen()
		return t, noticeTick(t.noticeGen)

	case noticeTickMsg:
		if msg.gen != t.noticeGen {
			return t, nil
		}
		if t.notice.strength() <= 0 {
			t.notice = notice{}
			return t, nil
		}
		return t, noticeTick(msg.gen)

	case dividerScheduleMsg:
		if msg.gen != t.dividerGen {
			return t, nil
		}
		t.beginDividerSweep()
		return t, dividerTick(msg.gen)

	case dividerTickMsg:
		if msg.gen != t.dividerGen {
			return t, nil
		}
		if _, sweeping := t.dividerProgress(); sweeping {
			return t, dividerTick(msg.gen)
		}
		t.dividerStart = time.Time{}
		return t, scheduleDividerSweep(msg.gen)
	}

	if len(t.Items) == 0 {
		return t, nil
	}

	if tick, ok := msg.(tabSweepTickMsg); ok {
		if tick.gen != t.sweepGen {
			return t, nil
		}
		if _, sweeping := t.sweepProgress(); sweeping {
			return t, tabSweepTick(tick.gen)
		}
		return t, nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch key := keyMsg.String(); key {
		case "right", "tab":
			t.Active = (t.Active + 1) % len(t.Items)
			return t.activate()
		case "left", "shift+tab":
			t.Active = (t.Active - 1 + len(t.Items)) % len(t.Items)
			return t.activate()
		case "0":
			t.Active = len(t.Items) - 1
			return t.activate()
		default:
			if idx, ok := tabIndexFromKey(key); ok && idx < len(t.Items) {
				t.Active = idx
				return t.activate()
			}
		}
	}

	content, cmd := t.Items[t.Active].Content.Update(msg)
	t.Items[t.Active].Content = content
	return t, cmd
}

// tabIndexFromKey maps digit keys "1"-"9" to zero-based tab indices.
func tabIndexFromKey(key string) (int, bool) {
	if len(key) != 1 || key[0] < '1' || key[0] > '9' {
		return 0, false
	}
	return int(key[0] - '1'), true
}

func (t Tabs) View() string {
	if len(t.Items) == 0 {
		return ""
	}

	row := t.headerRow()

	// Drop the border edge facing the tab row — a plain rule stands in for
	// it instead, so the tabs and content read as one connected panel
	// rather than two separate boxes with a gap between them. HideBorder
	// drops the other three edges too, leaving just that divider bar, and
	// also trims the padding on that same divider-facing side, so content
	// sits flush against the divider instead of leaving a blank line.
	box := contentStyle
	if t.HideBorder {
		box = box.BorderTop(false).BorderRight(false).BorderBottom(false).BorderLeft(false)
		if t.Position == Bottom {
			box = box.PaddingBottom(0)
		} else {
			box = box.PaddingTop(0)
		}
	} else if t.Position == Bottom {
		box = box.BorderBottom(false)
	} else {
		box = box.BorderTop(false)
	}
	content := box.Render(t.Items[t.Active].Content.View().Content)

	run := strings.Repeat("─", max(lipgloss.Width(row), lipgloss.Width(content)))
	divider := separatorStyle.Render(run)

	// A notice owns the rule while it lasts: the sweep is decorative, and two
	// things animating the same line at once would fight.
	if t.notice.strength() > 0 {
		return t.joinRows(row, t.notice.render(run), content)
	}

	if progress, sweeping := t.dividerProgress(); sweeping {
		if t.dividerReverse {
			progress = 1 - progress
		}
		divider = flashsweep.Glint(run, progress, dividerRamp(t.dividerColor))
	}

	return t.joinRows(row, divider, content)
}

// joinRows stacks the three bands in whichever order Position calls for.
func (t Tabs) joinRows(row, divider, content string) string {
	if t.Position == Bottom {
		return lipgloss.JoinVertical(lipgloss.Left, content, divider, row)
	}
	return lipgloss.JoinVertical(lipgloss.Left, row, divider, content)
}

// headerRow renders the tab labels, windowed around Active with ‹/›
// scroll indicators when they don't all fit within Width. Width <= 0 means
// unconstrained: every tab renders, as before Width existed.
func (t Tabs) headerRow() string {
	labels := make([]string, len(t.Items))
	widths := make([]int, len(t.Items))
	for i, tab := range t.Items {
		label := tabGlyph(tab.Playing) + " " + tab.Title

		// Focus wins the colour outright, so the active tab reads the same
		// whether or not it is playing; among the rest, playing rooms sit a
		// step brighter than idle ones.
		style := inactiveTabStyle
		switch {
		case i == t.Active:
			style = activeTabStyle
			if progress, sweeping := t.sweepProgress(); sweeping {
				label = flashsweep.Glint(label, progress, tabSweepFg)
			}
		case tab.Playing:
			style = playingTabStyle
		}

		labels[i] = style.Render(label)
		widths[i] = lipgloss.Width(labels[i])
	}

	full := lipgloss.JoinHorizontal(lipgloss.Top, labels...)
	if t.Width <= 0 || lipgloss.Width(full) <= t.Width {
		return full
	}

	// Doesn't fit: reserve room for both indicators up front (simpler than
	// discovering after the fact that only one side ended up clipped, at
	// the cost of occasionally not using a sliver of space we didn't need
	// to reserve), then grow a window outward from Active, alternating
	// sides, until neither side has room left to add its next tab.
	indicatorWidth := lipgloss.Width(inactiveTabStyle.Render("‹"))
	budget := t.Width - 2*indicatorWidth
	if budget < widths[t.Active] {
		budget = widths[t.Active] // always show at least the active tab
	}

	lo, hi := t.Active, t.Active
	width := widths[t.Active]
	tryRight := true
	for lo > 0 || hi < len(t.Items)-1 {
		grew := false
		if tryRight && hi < len(t.Items)-1 && width+widths[hi+1] <= budget {
			hi++
			width += widths[hi]
			grew = true
		} else if !tryRight && lo > 0 && width+widths[lo-1] <= budget {
			lo--
			width += widths[lo]
			grew = true
		}
		if !grew {
			// Preferred side was out of room or exhausted — fall back to
			// whichever side still has room, if either does.
			if hi < len(t.Items)-1 && width+widths[hi+1] <= budget {
				hi++
				width += widths[hi]
			} else if lo > 0 && width+widths[lo-1] <= budget {
				lo--
				width += widths[lo]
			} else {
				break
			}
		}
		tryRight = !tryRight
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, labels[lo:hi+1]...)
	if lo > 0 {
		row = inactiveTabStyle.Render("‹") + row
	}
	if hi < len(t.Items)-1 {
		row += inactiveTabStyle.Render("›")
	}
	return row
}

// Braille cells, read as a level meter to match the equalizer in the room
// view. Both are reliably one column wide, unlike ▶ and its emoji variant.
const (
	playingGlyph = "⣶"
	idleGlyph    = "⣀"
)

// tabGlyph is the status slot every tab carries. Idle gets its own glyph
// rather than a blank, so starting or stopping playback cannot reflow the row.
func tabGlyph(playing bool) string {
	if playing {
		return playingGlyph
	}
	return idleGlyph
}

// dividerRamp blends the configured sweep colour against the divider's resting
// tone, so the light fades into the line rather than replacing it.
func dividerRamp(flash color.Color) flashsweep.Ramp {
	if flash == nil {
		flash = separatorColor
	}
	return flashsweep.Ramp{
		Base:  flashsweep.FromColor(separatorColor),
		Flash: flashsweep.FromColor(flash),
	}
}

// Colors go through theme.LightDark so the palette stays legible whether the
// user's terminal background is light or dark, rather than assuming one or the other.
var (
	activeTabAccent = theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24"))

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Foreground(activeTabAccent)

	tabSweepFg = flashsweep.Ramp{
		Base:  flashsweep.FromColor(activeTabAccent),
		Flash: flashsweep.FromColor(theme.LightDark(lipgloss.Color("#F59E0B"), lipgloss.Color("#FFFFFF"))),
	}

	inactiveTabStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))

	// A step more contrast than inactiveTabStyle, so a playing room reads
	// from the corner of the eye without competing with the active tab's
	// accent. Deliberately still grey: colour belongs to focus alone.
	playingTabStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(theme.LightDark(lipgloss.Color("#1F2937"), lipgloss.Color("#E5E7EB")))

	separatorColor = theme.LightDark(lipgloss.Color("#D0D0D0"), lipgloss.Color("#4A4A4A"))

	separatorStyle = lipgloss.NewStyle().Foreground(separatorColor)

	contentStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.LightDark(lipgloss.Color("#D0D0D0"), lipgloss.Color("#4A4A4A"))).
			Padding(1, 2)
)

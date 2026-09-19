package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
	"github.com/haasted/sonos-sandbox/internal/ui/movy"
)

const (
	revealFPS          = 60
	revealHold         = 4 * time.Second        // panes.noticeHold
	revealCollapse     = 900 * time.Millisecond // matches panes.noticeFade, the exit it replaces
	revealGap          = 900 * time.Millisecond // pause before the loop replays
	revealLead         = 4                      // panes.noticeLead: shortest run of rule either side
	revealDefaultWidth = 72
)

// revealTextFlag overrides the label, so a style can be judged on the string it
// will really carry rather than only on the one it was tuned against.
var revealTextFlag = flag.String("reveal-text", " press ? for keyboard shortcuts ",
	"text for the `reveal` layout to open")

// revealBlurbs explain each style in the demo's own words. The package names
// them; a caption has room to say what a name cannot.
var revealBlurbs = map[flashsweep.RevealStyle]string{
	flashsweep.RevealMirror: "Line folded — two wavefronts leave the centre",
	flashsweep.RevealPrism:  "same geometry, hue travelling with the light",
	flashsweep.RevealBloom:  "bursts out fully lit, then cools into place",
	flashsweep.RevealSplit:  "runes start stacked at the centre and slide apart",
}

type noticeRevealEffect struct {
	start time.Time
	text  string
	width int
	speed float32
	solo  int  // index into flashsweep.RevealStyles, or -1 for all of them
	full  bool // play hold and fade too, not just the entrance
}

func newNoticeRevealEffect() noticeRevealEffect {
	return noticeRevealEffect{
		start: time.Now(),
		text:  *revealTextFlag,
		width: revealDefaultWidth,
		speed: flashsweep.DefaultRevealSpeed,
		solo:  -1,
	}
}

type noticeRevealTickMsg time.Time

func noticeRevealTick() tea.Cmd {
	return tea.Tick(time.Second/revealFPS, func(t time.Time) tea.Msg { return noticeRevealTickMsg(t) })
}

func (m noticeRevealEffect) Init() tea.Cmd { return noticeRevealTick() }

func (m noticeRevealEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyPressMsg:
		return m.key(msg.String())

	case noticeRevealTickMsg:
		if time.Since(m.start) > m.cycle() {
			m.start = time.Now()
		}
		return m, noticeRevealTick()
	}
	return m, nil
}

func (m noticeRevealEffect) key(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case " ":
		m.start = time.Now()
	case "f":
		m.full = !m.full
		m.start = time.Now()
	case "+", "=":
		m.speed = min(m.speed+4, 80)
		m.start = time.Now()
	case "-", "_":
		m.speed = max(m.speed-4, 4)
		m.start = time.Now()
	case "0":
		m.solo = -1
	case "1", "2", "3", "4":
		if i := int(key[0] - '1'); i < len(flashsweep.RevealStyles) {
			m.solo = i
		}
	}
	return m, nil
}

func (m noticeRevealEffect) View() tea.View {
	progress, closing, strength := m.phase()

	var b strings.Builder
	b.WriteString("\n")
	for i, style := range flashsweep.RevealStyles {
		if m.solo >= 0 && m.solo != i {
			continue
		}
		fmt.Fprintf(&b, "  %s  %s\n",
			revealNameStyle().Render(style.String()), revealBlurbStyle().Render(revealBlurbs[style]))
		b.WriteString(m.rule(style, progress, closing, strength) + "\n\n")
	}
	b.WriteString(revealBlurbStyle().Render(m.status()) + "\n")
	return tea.NewView(b.String())
}

// phase places the elapsed time in the lifecycle: how far the current movement
// has run, whether that movement is the collapse, and how lit the notice still
// is. Strength follows the collapse, so the label cools as it retreats.
func (m noticeRevealEffect) phase() (progress float32, closing bool, strength float32) {
	elapsed := time.Since(m.start)
	entrance := m.entrance()

	if !m.full || elapsed <= entrance+revealHold {
		return float32(elapsed) / float32(entrance), false, 1
	}

	collapsed := float32(elapsed-entrance-revealHold) / float32(revealCollapse)
	return collapsed, true, 1 - min(collapsed, 1)
}

// entrance is how long the opening runs. Every style shares one clock, so the
// stacked rows stay comparable even where a style paces itself differently.
func (m noticeRevealEffect) entrance() time.Duration {
	return m.reveal(flashsweep.RevealMirror, 1).Duration(m.text)
}

func (m noticeRevealEffect) cycle() time.Duration {
	d := m.entrance()
	if m.full {
		d += revealHold + revealCollapse
	}
	return d + revealGap
}

// reveal configures the component for one style. Fill is the divider's own rule
// rune, so the text parts the rule on the way in and the rule closes over it on
// the way out — no snap back from letters to rule at the end.
func (m noticeRevealEffect) reveal(style flashsweep.RevealStyle, strength float32) flashsweep.Reveal {
	rule := themedV3("#D0D0D0", "#4A4A4A") // panes.separatorColor
	info := themedV3("#1D4ED8", "#93C5FD") // panes.noticeColor(NoticeInfo)

	return flashsweep.Reveal{
		Style:     style,
		Speed:     m.speed,
		Fill:      '─',
		FillColor: rule,
		Ramp: flashsweep.Ramp{
			Base:  rule.Lerp(info, strength),
			Flash: rule.Lerp(movy.WhiteF, strength),
		},
	}
}

// rule sets the label into a divider, laid out the way panes.notice.render
// lays out a real one.
func (m noticeRevealEffect) rule(style flashsweep.RevealStyle, progress float32, closing bool, strength float32) string {
	r := m.reveal(style, strength)

	label := r.Open(m.text, progress)
	if closing {
		label = r.Close(m.text, progress)
	}

	lead := max((m.width-lipgloss.Width(m.text))/2, revealLead)
	trailing := max(m.width-lead-lipgloss.Width(m.text), 0)

	return ruleStyle().Render(strings.Repeat("─", lead)) +
		label +
		ruleStyle().Render(strings.Repeat("─", trailing))
}

func (m noticeRevealEffect) status() string {
	mode := "entrance only"
	if m.full {
		mode = "full lifecycle (entrance · hold · collapse)"
	}

	return fmt.Sprintf("  space replay · f %s · +/- speed %.0f (%.0fms) · 1-4 solo · 0 all · q quit · -reveal-text sets the label",
		mode, m.speed, float64(m.entrance())/float64(time.Millisecond))
}

func ruleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(themedColor("#D0D0D0", "#4A4A4A"))
}

func revealNameStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(themedColor("#374151", "#E5E7EB"))
}

func revealBlurbStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(themedColor("#9CA3AF", "#6B7280"))
}

func layoutNoticeReveal() {
	if _, err := tea.NewProgram(newNoticeRevealEffect()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

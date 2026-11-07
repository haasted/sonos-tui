package main

import (
	"fmt"
	"image/color"
	"math/rand"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/flashsweep"
)

const (
	returnFPS          = 60
	returnEmpty        = 600 * time.Millisecond // the gap shown empty before the rule returns
	returnWhole        = 900 * time.Millisecond // the rule shown whole before the loop replays
	returnDefaultGap   = 28                     // about the width of a short notice
	returnDefaultWidth = 72
)

// returnBlurbs say what each style does, in the demo's own words.
var returnBlurbs = map[flashsweep.ReturnStyle]string{
	flashsweep.ReturnFade:     "the whole gap fades up at once, as today",
	flashsweep.ReturnCloseIn:  "regrows from both ends and meets in the middle",
	flashsweep.ReturnOpenOut:  "grows from the middle out to both ends",
	flashsweep.ReturnSweep:    "drawn from one end to the other; the side is random",
	flashsweep.ReturnDissolve: "cells fill in a scattered order",
	flashsweep.ReturnGlint:    "fades up while a light runs along it",
	flashsweep.ReturnStitch:   "dotted, then dashed, then solid",
}

// ruleReturnEffect loops every return style side by side, plus one row that
// picks a style at random each time round, as a notice would.
type ruleReturnEffect struct {
	start  time.Time
	width  int
	gap    int
	solo   int   // index into flashsweep.ReturnStyles, or -1 for all
	seed   int64 // the dissolve's order and the sweep's side, new each loop
	random flashsweep.ReturnStyle
}

type ruleReturnTickMsg time.Time

func newRuleReturnEffect() ruleReturnEffect {
	m := ruleReturnEffect{width: returnDefaultWidth, gap: returnDefaultGap, solo: -1}
	return m.replay()
}

func ruleReturnTick() tea.Cmd {
	return tea.Tick(time.Second/returnFPS, func(t time.Time) tea.Msg { return ruleReturnTickMsg(t) })
}

func (m ruleReturnEffect) Init() tea.Cmd { return ruleReturnTick() }

func (m ruleReturnEffect) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyPressMsg:
		return m.key(msg.String())
	case ruleReturnTickMsg:
		if time.Since(m.start) > returnEmpty+flashsweep.ReturnDuration(m.gap)+returnWhole {
			m = m.replay()
		}
		return m, ruleReturnTick()
	}
	return m, nil
}

func (m ruleReturnEffect) key(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case " ":
		m = m.replay()
	case "+", "=":
		m.gap = min(m.gap+4, max(m.width-12, 4))
		m = m.replay()
	case "-", "_":
		m.gap = max(m.gap-4, 2)
		m = m.replay()
	case "0":
		m.solo = -1
	case "1", "2", "3", "4", "5", "6", "7":
		if i := int(key[0] - '1'); i < len(flashsweep.ReturnStyles) {
			m.solo = i
		}
	}
	return m, nil
}

// replay starts the loop again, with a new seed and a new random style.
func (m ruleReturnEffect) replay() ruleReturnEffect {
	m.start = time.Now()
	m.seed = rand.Int63()
	m.random = flashsweep.ReturnStyles[rand.Intn(len(flashsweep.ReturnStyles))]
	return m
}

// progress is how far the rule is back: 0 while the gap shows empty, then
// rising over the return's duration, which grows with the gap.
func (m ruleReturnEffect) progress() float32 {
	since := time.Since(m.start) - returnEmpty
	if since <= 0 {
		return 0
	}
	return min(float32(since)/float32(flashsweep.ReturnDuration(m.gap)), 1)
}

func (m ruleReturnEffect) View() tea.View {
	progress := m.progress()

	var b strings.Builder
	b.WriteString("\n")
	for i, style := range flashsweep.ReturnStyles {
		if m.solo >= 0 && m.solo != i {
			continue
		}
		fmt.Fprintf(&b, "  %d %s  %s\n", i+1,
			revealNameStyle().Render(style.String()), revealBlurbStyle().Render(returnBlurbs[style]))
		b.WriteString("  " + m.line(style, progress) + "\n\n")
	}
	if m.solo < 0 {
		fmt.Fprintf(&b, "  %s  %s\n", revealNameStyle().Render("random"),
			revealBlurbStyle().Render("this time: "+m.random.String()+", as a notice would pick"))
		b.WriteString("  " + m.line(m.random, progress) + "\n\n")
	}
	fmt.Fprintf(&b, "  %s\n", revealBlurbStyle().Render(fmt.Sprintf(
		"gap %d cells, returns in %v · space replay · +/- gap · 1-7 one style · 0 all · q quit",
		m.gap, flashsweep.ReturnDuration(m.gap))))
	return tea.NewView(b.String())
}

// line is a divider with a gap in its middle, the rule coming back into it.
func (m ruleReturnEffect) line(style flashsweep.ReturnStyle, progress float32) string {
	width := max(m.width-4, m.gap+2)
	lead := (width - m.gap) / 2
	trail := width - m.gap - lead

	rule := lipgloss.NewStyle().Foreground(returnRuleColor())
	r := flashsweep.RuleReturn{
		Style:    style,
		Rule:     '─',
		Color:    flashsweep.FromColor(returnRuleColor()),
		Backdrop: flashsweep.FromColor(themedColor("#FFFFFF", "#1E1E1E")),
		Flash:    flashsweep.FromColor(themedColor("#0F172A", "#FFFFFF")),
		Seed:     m.seed,
	}
	return rule.Render(strings.Repeat("─", lead)) + r.Draw(m.gap, progress) + rule.Render(strings.Repeat("─", trail))
}

// returnRuleColor is the divider's resting colour, as the tab rule draws it.
func returnRuleColor() color.Color { return themedColor("#C4C4C4", "#5A5A5A") }

func layoutRuleReturn() {
	if _, err := tea.NewProgram(newRuleReturnEffect()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

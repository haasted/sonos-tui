package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// tabsRootModel is the outermost Bubbletea model for the "tabs" layout —
// wraps panes.Tabs with a q/ctrl+c quit binding so the playground has
// something runnable. It also drives a spinner next to one random tab, to
// demo a tab whose title updates independently of which tab is active.
type tabsRootModel struct {
	tabs         panes.Tabs
	spinner      spinner.Model
	spinnerTab   int
	spinnerStyle int // index into spinnerStyles, cycled with "d"
}

// Braille reference for hand-building bar-style spinner frames (used by
// Equalizer, Bars and Jitter below). Each cell is 2 columns x 4 rows of
// dots that fill bottom-up, so a cell can represent two independent bar
// heights from 0 (empty) to 4 (full):
//
//	left-only,  height 0-4: ⠀ ⡀ ⡄ ⡆ ⡇
//	right-only, height 0-4: ⠀ ⢀ ⢠ ⢰ ⢸
//
//	combined grid, rows = left height, columns = right height:
//	⠀ ⢀ ⢠ ⢰ ⢸
//	⡀ ⣀ ⣠ ⣰ ⣸
//	⡄ ⣄ ⣤ ⣴ ⣼
//	⡆ ⣆ ⣦ ⣶ ⣾
//	⡇ ⣇ ⣧ ⣷ ⣿

// Full Braille Patterns Unicode block, U+2800-U+28FF (256 chars, every
// possible combination of the 8 dots — the bar-height table above is just
// the 25 of these where each column fills contiguously from the bottom).
// Row = codepoint high nibble of the low byte, column = low nibble, e.g.
// row E0s column 6 is U+28E6 = ⣦.
//
//	     0 1 2 3 4 5 6 7 8 9 A B C D E F
//	00s  ⠀ ⠁ ⠂ ⠃ ⠄ ⠅ ⠆ ⠇ ⠈ ⠉ ⠊ ⠋ ⠌ ⠍ ⠎ ⠏
//	10s  ⠐ ⠑ ⠒ ⠓ ⠔ ⠕ ⠖ ⠗ ⠘ ⠙ ⠚ ⠛ ⠜ ⠝ ⠞ ⠟
//	20s  ⠠ ⠡ ⠢ ⠣ ⠤ ⠥ ⠦ ⠧ ⠨ ⠩ ⠪ ⠫ ⠬ ⠭ ⠮ ⠯
//	30s  ⠰ ⠱ ⠲ ⠳ ⠴ ⠵ ⠶ ⠷ ⠸ ⠹ ⠺ ⠻ ⠼ ⠽ ⠾ ⠿
//	40s  ⡀ ⡁ ⡂ ⡃ ⡄ ⡅ ⡆ ⡇ ⡈ ⡉ ⡊ ⡋ ⡌ ⡍ ⡎ ⡏
//	50s  ⡐ ⡑ ⡒ ⡓ ⡔ ⡕ ⡖ ⡗ ⡘ ⡙ ⡚ ⡛ ⡜ ⡝ ⡞ ⡟
//	60s  ⡠ ⡡ ⡢ ⡣ ⡤ ⡥ ⡦ ⡧ ⡨ ⡩ ⡪ ⡫ ⡬ ⡭ ⡮ ⡯
//	70s  ⡰ ⡱ ⡲ ⡳ ⡴ ⡵ ⡶ ⡷ ⡸ ⡹ ⡺ ⡻ ⡼ ⡽ ⡾ ⡿
//	80s  ⢀ ⢁ ⢂ ⢃ ⢄ ⢅ ⢆ ⢇ ⢈ ⢉ ⢊ ⢋ ⢌ ⢍ ⢎ ⢏
//	90s  ⢐ ⢑ ⢒ ⢓ ⢔ ⢕ ⢖ ⢗ ⢘ ⢙ ⢚ ⢛ ⢜ ⢝ ⢞ ⢟
//	A0s  ⢠ ⢡ ⢢ ⢣ ⢤ ⢥ ⢦ ⢧ ⢨ ⢩ ⢪ ⢫ ⢬ ⢭ ⢮ ⢯
//	B0s  ⢰ ⢱ ⢲ ⢳ ⢴ ⢵ ⢶ ⢷ ⢸ ⢹ ⢺ ⢻ ⢼ ⢽ ⢾ ⢿
//	C0s  ⣀ ⣁ ⣂ ⣃ ⣄ ⣅ ⣆ ⣇ ⣈ ⣉ ⣊ ⣋ ⣌ ⣍ ⣎ ⣏
//	D0s  ⣐ ⣑ ⣒ ⣓ ⣔ ⣕ ⣖ ⣗ ⣘ ⣙ ⣚ ⣛ ⣜ ⣝ ⣞ ⣟
//	E0s  ⣠ ⣡ ⣢ ⣣ ⣤ ⣥ ⣦ ⣧ ⣨ ⣩ ⣪ ⣫ ⣬ ⣭ ⣮ ⣯
//	F0s  ⣰ ⣱ ⣲ ⣳ ⣴ ⣵ ⣶ ⣷ ⣸ ⣹ ⣺ ⣻ ⣼ ⣽ ⣾ ⣿

// spinnerStyles are every named spinner bubbles/spinner ships, in display order.
var spinnerStyles = []struct {
	name string
	spin spinner.Spinner
}{
	{"Dot", spinner.Dot},
	{"Equalizer", spinner.Spinner{
		Frames: []string{"⣠", "⣶", "⣧"},
		FPS:    200 * time.Millisecond,
	}},
	// Bars is a wider take on the same idea: 3 braille cells side by side
	// (6 half-width columns total), each column's dot count standing in
	// for a bar height (1-4), phase-shifted so they bounce like an EQ
	// rather than blinking together.
	{"Bars", spinner.Spinner{
		Frames: []string{"⣠⣾⣦", "⣴⣷⣄", "⣾⣦⣠", "⣷⣄⣴", "⣦⣠⣾", "⣄⣴⣷"},
		FPS:    time.Second / 8,
	}},
	// Jitter is the same 6-bar layout as Bars, but each bar jumps to an
	// unrelated height every frame instead of rippling through a wave —
	// closer to how a real audio level meter looks, all bars moving
	// independently rather than in a traveling pattern.
	{"Jitter", spinner.Spinner{
		Frames: []string{"⣦⣾⣤", "⣇⣴⣸", "⣠⣠⣆", "⣄⣷⣸", "⣠⣾⣇", "⣿⣄⣠", "⣆⣶⣼", "⣸⣼⣇"},
		FPS:    time.Second / 6,
	}},
	{"Line", spinner.Line},
	{"MiniDot", spinner.MiniDot},
	{"Jump", spinner.Jump},
	{"Pulse", spinner.Pulse},
	{"Points", spinner.Points},
	{"Globe", spinner.Globe},
	{"Moon", spinner.Moon},
	{"Monkey", spinner.Monkey},
	{"Meter", spinner.Meter},
	{"Hamburger", spinner.Hamburger},
	{"Ellipsis", spinner.Ellipsis},
}

var roomNames = []string{
	"Kitchen",
	"Living Room",
	"Bedroom",
	"Bathroom",
	"Office",
	"Dining Room",
	"Garage",
	"Patio",
}

func newTabsRootModel(pos panes.Position) tabsRootModel {
	tabs := make([]panes.Tab, len(roomNames))
	for i, room := range roomNames {
		content := fmt.Sprintf("Nothing playing yet in the %s.", room)
		tabs[i] = panes.Tab{Title: room, Content: panes.StaticContent(content)}
	}
	t := panes.NewTabs(tabs)
	t.Position = pos

	s := spinner.New()
	s.Spinner = spinnerStyles[0].spin

	m := tabsRootModel{tabs: t, spinner: s, spinnerTab: rand.Intn(len(tabs))}
	m.updateSpinnerTabTitle()
	return m
}

// updateSpinnerTabTitle stamps the current spinner frame and style name onto
// the chosen tab's title, so it animates regardless of which tab is active
// and the style is identifiable while cycling with "d".
func (m *tabsRootModel) updateSpinnerTabTitle() {
	name := spinnerStyles[m.spinnerStyle].name
	m.tabs.Tabs[m.spinnerTab].Title = fmt.Sprintf("%s %s (%s)", roomNames[m.spinnerTab], m.spinner.View(), name)
}

func (m tabsRootModel) Init() tea.Cmd {
	return tea.Batch(m.tabs.Init(), m.spinner.Tick)
}

func (m tabsRootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "d":
			m.spinnerStyle = (m.spinnerStyle + 1) % len(spinnerStyles)
			m.spinner.Spinner = spinnerStyles[m.spinnerStyle].spin
			m.updateSpinnerTabTitle()
			return m, nil
		}
	}

	if _, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.updateSpinnerTabTitle()
		return m, cmd
	}

	tabs, cmd := m.tabs.Update(msg)
	m.tabs = tabs
	return m, cmd
}

func (m tabsRootModel) View() string {
	return m.tabs.View() + "\n\ntab/shift+tab or ←/→ to switch tabs · 1-8 to jump, 0 for last · d to cycle spinner style · q to quit\n"
}

func layoutTabs() {
	runTabs(panes.Top)
}

// layoutTabsBottom is the same component with Position: Bottom, to compare side by side with layoutTabs.
func layoutTabsBottom() {
	runTabs(panes.Bottom)
}

func runTabs(pos panes.Position) {
	if _, err := tea.NewProgram(newTabsRootModel(pos)).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

package main

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-tui/internal/ui/flashsweep"
)

const (
	// spinnerFrameRate is how often the demo repaints. Every proposal is drawn
	// from one chain at this rate, and each picks its own frame from the clock,
	// so twenty spinners cost one ticker rather than twenty.
	spinnerFrameRate = 40 * time.Millisecond

	spinnerNameColumn = 11 // widest proposal name, plus a space
	spinnerNoteColumn = 30 // what is left for the note beside it
	spinnerColumns    = 2  // proposals side by side
	spinnerGap        = 4

	spinnerSpeedStep = 0.25
	spinnerSpeedMin  = 0.25
	spinnerSpeedMax  = 4
)

// spinnerProposal is one candidate for the now-playing spinner.
type spinnerProposal struct {
	name  string
	about string
	spin  spinner.Spinner

	// tint colours the frame from the time it has been running, for a proposal
	// whose movement is in the colour rather than in the glyph. Nil leaves the
	// frame in the panel's own colour.
	tint func(elapsed time.Duration) color.Color
}

// spinnerFamily groups proposals that are variations on one idea, so a reader
// compares like with like rather than reading a list of twenty.
type spinnerFamily struct {
	title string
	about string
	items []spinnerProposal
}

// spinnerFamilies are the proposals. Every frame is a single cell unless the
// demo says otherwise: the now-playing panel derives the indent of the artist
// and album rows from the width of whatever glyph is showing, and the paused
// and muted glyphs are one cell. A wider spinner therefore shifts those two
// rows every time playback stops.
//
// On fonts, which lipgloss.Width cannot answer: a glyph the terminal font does
// not cover is drawn by a substituted one at another optical size, so it looks
// wrong and differs between machines. Checked with
// `fc-list ":charset=<hex>" family` on macOS, every block, geometric and
// dingbat glyph below is carried by a monospace font -- and BRAILLE IS NOT. The
// family the panel already uses is the least covered of the five here, and it
// renders correctly all the same, so treat the check as a comparison and not a
// verdict.
var spinnerFamilies = []spinnerFamily{
	{
		title: "BRAILLE BARS",
		about: "two bars in one cell, the family the current spinner belongs to",
		items: []spinnerProposal{
			{name: "equalizer", about: "what the panel uses today",
				spin: spinner.Spinner{Frames: []string{"⣠", "⣶", "⣧"}, FPS: 200 * time.Millisecond}},
			{name: "pulse", about: "both bars together",
				spin: spinner.Spinner{Frames: []string{"⣀", "⣤", "⣶", "⣿", "⣶", "⣤"}, FPS: 120 * time.Millisecond}},
			{name: "seesaw", about: "one bar up, the other down",
				spin: spinner.Spinner{Frames: []string{"⣸", "⣴", "⣦", "⣇", "⣦", "⣴"}, FPS: 130 * time.Millisecond}},
			{name: "vu", about: "quick up, slow down",
				spin: spinner.Spinner{Frames: []string{"⣿", "⣾", "⣷", "⣶", "⣦", "⣤", "⣄", "⣀"}, FPS: 110 * time.Millisecond}},
			{name: "fill", about: "fills from the left",
				spin: spinner.Spinner{Frames: []string{"⡇", "⣇", "⣧", "⣷", "⣿", "⣷", "⣧", "⣇"}, FPS: 110 * time.Millisecond}},
		},
	},
	{
		title: "BRAILLE DOTS",
		about: "the same family, moving as dots rather than as bars",
		items: []spinnerProposal{
			{name: "orbit", about: "one dot around the rim",
				spin: spinner.Spinner{Frames: []string{"⠁", "⠂", "⠄", "⡀", "⢀", "⠠", "⠐", "⠈"}, FPS: 100 * time.Millisecond}},
			{name: "classic", about: "the one most terminals use",
				spin: spinner.Spinner{Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}, FPS: 90 * time.Millisecond}},
			{name: "blob", about: "a heavier shape going round",
				spin: spinner.Spinner{Frames: []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}, FPS: 100 * time.Millisecond}},
			{name: "grow", about: "fills dot by dot",
				spin: spinner.Spinner{Frames: []string{"⠁", "⠃", "⠇", "⡇", "⣇", "⣧", "⣷", "⣿", "⣷", "⣧", "⣇", "⡇", "⠇", "⠃"}, FPS: 80 * time.Millisecond}},
			{name: "bounce", about: "a line falling and rising",
				spin: spinner.Spinner{Frames: []string{"⠉", "⠒", "⠤", "⣀", "⠤", "⠒"}, FPS: 130 * time.Millisecond}},
			{name: "sparkle", about: "single dots at random",
				spin: spinner.Spinner{Frames: []string{"⠁", "⠠", "⠄", "⢀", "⠈", "⡀", "⠂", "⠐"}, FPS: 90 * time.Millisecond}},
		},
	},
	{
		title: "BLOCKS",
		about: "block drawing characters, heavier than braille and less dotty",
		items: []spinnerProposal{
			{name: "stack", about: "a bar growing from the floor",
				spin: spinner.Spinner{Frames: []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█", "▇", "▆", "▅", "▄", "▃", "▂"}, FPS: 70 * time.Millisecond}},
			{name: "slide", about: "the same, from the left",
				spin: spinner.Spinner{Frames: []string{"▏", "▎", "▍", "▌", "▋", "▊", "▉", "█", "▉", "▊", "▋", "▌", "▍", "▎"}, FPS: 70 * time.Millisecond}},
			{name: "shade", about: "darkening and lightening",
				spin: spinner.Spinner{Frames: []string{"░", "▒", "▓", "█", "▓", "▒"}, FPS: 140 * time.Millisecond}},
			{name: "quadrant", about: "a quarter block, turning",
				spin: spinner.Spinner{Frames: []string{"▘", "▝", "▗", "▖"}, FPS: 150 * time.Millisecond}},
			{name: "halves", about: "half the cell, turning",
				spin: spinner.Spinner{Frames: []string{"▌", "▀", "▐", "▄"}, FPS: 150 * time.Millisecond}},
		},
	},
	{
		title: "GEOMETRY",
		about: "shapes outside both families, which read as an icon rather than a meter",
		items: []spinnerProposal{
			{name: "clock", about: "a circle filling by quarters",
				spin: spinner.Spinner{Frames: []string{"◐", "◓", "◑", "◒"}, FPS: 150 * time.Millisecond}},
			{name: "arc", about: "a quarter arc going round",
				spin: spinner.Spinner{Frames: []string{"◜", "◝", "◞", "◟"}, FPS: 150 * time.Millisecond}},
			{name: "corners", about: "a triangle in each corner",
				spin: spinner.Spinner{Frames: []string{"◢", "◣", "◤", "◥"}, FPS: 150 * time.Millisecond}},
			{name: "beat", about: "a dot swelling and shrinking",
				spin: spinner.Spinner{Frames: []string{"·", "∙", "•", "●", "•", "∙"}, FPS: 130 * time.Millisecond}},
			{name: "star", about: "points appearing and going",
				spin: spinner.Spinner{Frames: []string{"✶", "✷", "✸", "✹", "✺", "✹", "✸", "✷"}, FPS: 110 * time.Millisecond}},
		},
	},
	{
		title: "COLOUR RATHER THAN SHAPE",
		about: "the effect library driving the colour; some of these barely move at all",
		items: []spinnerProposal{
			{name: "breath", about: "still shape, breathing colour",
				spin: spinner.Spinner{Frames: []string{"⣿"}, FPS: time.Second},
				tint: func(d time.Duration) color.Color {
					return flashsweep.BreathAt(d, spinnerRest(), spinnerLit())
				}},
			{name: "lit bars", about: "today's spinner, breathing",
				spin: spinner.Spinner{Frames: []string{"⣠", "⣶", "⣧"}, FPS: 200 * time.Millisecond},
				tint: func(d time.Duration) color.Color {
					return flashsweep.BreathAt(d, spinnerRest(), spinnerLit())
				}},
			{name: "heartbeat", about: "a breath three times as fast",
				spin: spinner.Spinner{Frames: []string{"●"}, FPS: time.Second},
				tint: func(d time.Duration) color.Color {
					return flashsweep.BreathAt(d*3, spinnerRest(), spinnerLit())
				}},
			{name: "ember", about: "shape and colour out of step",
				spin: spinner.Spinner{Frames: []string{"⣀", "⣤", "⣶", "⣿", "⣶", "⣤"}, FPS: 120 * time.Millisecond},
				tint: func(d time.Duration) color.Color {
					return flashsweep.BreathAt(d*2, spinnerEmberRest(), spinnerEmberLit())
				}},
		},
	},
}

type spinnerDemo struct {
	start time.Time
	speed float64
}

func newSpinnerDemo() spinnerDemo {
	return spinnerDemo{start: time.Now(), speed: 1}
}

type spinnerTickMsg time.Time

func spinnerTick() tea.Cmd {
	return tea.Tick(spinnerFrameRate, func(t time.Time) tea.Msg { return spinnerTickMsg(t) })
}

func (m spinnerDemo) Init() tea.Cmd { return spinnerTick() }

func (m spinnerDemo) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "+", "=":
			m.speed = min(m.speed+spinnerSpeedStep, spinnerSpeedMax)
		case "-", "_":
			m.speed = max(m.speed-spinnerSpeedStep, spinnerSpeedMin)
		}
		return m, nil
	case spinnerTickMsg:
		return m, spinnerTick()
	}
	return m, nil
}

func (m spinnerDemo) View() tea.View {
	elapsed := time.Duration(float64(time.Since(m.start)) * m.speed)

	var b strings.Builder
	b.WriteString("\n  " + spinnerTitle().Render("spinner proposals") +
		spinnerNote().Render(fmt.Sprintf("   %d candidates · %.2fx", spinnerCount(), m.speed)) + "\n")
	for _, family := range spinnerFamilies {
		b.WriteString("\n  " + spinnerFamilyStyle().Render(family.title) +
			spinnerNote().Render("  "+family.about) + "\n")
		b.WriteString(m.familyRows(family, elapsed))
	}

	b.WriteString("\n  " + spinnerNote().Render(
		"every frame is one cell: a wider one shifts the panel's lower rows when playback stops") + "\n")
	b.WriteString("  " + spinnerNote().Render("+/- speed · q quit") + "\n")
	return tea.NewView(b.String())
}

// familyRows lays one family out in columns, so a family of six is three lines
// rather than six and the whole set fits a terminal.
func (m spinnerDemo) familyRows(family spinnerFamily, elapsed time.Duration) string {
	var b strings.Builder
	for i := 0; i < len(family.items); i += spinnerColumns {
		b.WriteString("  ")
		for j := i; j < min(i+spinnerColumns, len(family.items)); j++ {
			if j > i {
				b.WriteString(strings.Repeat(" ", spinnerGap))
			}
			b.WriteString(m.cell(family.items[j], elapsed))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// cell is one proposal: its frame now, its name, and a warning where the frame
// is wider than the one cell the panel has room for.
func (m spinnerDemo) cell(p spinnerProposal, elapsed time.Duration) string {
	frame := p.frameAt(elapsed)

	style := spinnerGlyphStyle()
	if p.tint != nil {
		style = lipgloss.NewStyle().Foreground(p.tint(elapsed))
	}

	name := p.name
	if w := lipgloss.Width(frame); w != 1 {
		name += fmt.Sprintf("(%d)", w) // too wide for the panel; see the header
	}

	// Padded to a fixed width rather than to its own: the notes differ in
	// length, and a cell as wide as its contents leaves the second column
	// starting somewhere new on every row.
	return style.Render(frame) + "  " +
		spinnerNameStyle().Render(padRight(name, spinnerNameColumn)) +
		spinnerNote().Render(padRight(spinnerClip(p.about, spinnerNoteColumn), spinnerNoteColumn))
}

// spinnerClip shortens a note to the column, marking the cut.
func spinnerClip(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	kept := []rune(s)[:max(width-1, 0)]
	return string(kept) + "…"
}

// frameAt picks the frame from the clock rather than from a ticker of its own.
// One chain drives every proposal, and a proposal's own FPS still decides how
// fast it moves.
func (p spinnerProposal) frameAt(elapsed time.Duration) string {
	if len(p.spin.Frames) == 0 {
		return " "
	}
	return p.spin.Frames[int(elapsed/p.spin.FPS)%len(p.spin.Frames)]
}

func spinnerCount() int {
	n := 0
	for _, family := range spinnerFamilies {
		n += len(family.items)
	}
	return n
}

func spinnerTitle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(themedColor("#B45309", "#FBBF24"))
}

func spinnerFamilyStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(themedColor("#1D4ED8", "#93C5FD"))
}

func spinnerNameStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(themedColor("#1F2937", "#E5E7EB"))
}

func spinnerGlyphStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(themedColor("#1F2937", "#E5E7EB"))
}

func spinnerNote() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(themedColor("#9CA3AF", "#6B7280"))
}

func spinnerRest() color.Color { return themedColor("#C4C4C4", "#4A4A4A") }

func spinnerLit() color.Color { return themedColor("#0F172A", "#FFFFFF") }

func spinnerEmberRest() color.Color { return themedColor("#C4C4C4", "#4A4A4A") }

func spinnerEmberLit() color.Color { return themedColor("#B45309", "#FBBF24") }

func layoutSpinners() {
	if _, err := tea.NewProgram(newSpinnerDemo()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "bubbletea error: %v\n", err)
		os.Exit(1)
	}
}

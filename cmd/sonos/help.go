package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/haasted/sonos-sandbox/internal/ui/theme"
)

// helpKey is one row of the help popup: the keys that do a thing, and the
// thing. Deliberately a literal table rather than generated from the
// bindings, which live across speakerModel, appModel and panes.Tabs — see
// TestHelp_ListsEveryKeyTheAppHandles for the guard against it drifting.
type helpKey struct {
	keys string
	what string
}

const (
	helpKeyColumn = 10 // width of the key column, so descriptions line up
	helpColumnGap = 4
)

var (
	helpSectionStyle = lipgloss.NewStyle().Bold(true).
				Foreground(theme.LightDark(lipgloss.Color("#B45309"), lipgloss.Color("#FBBF24")))

	helpKeyStyle = lipgloss.NewStyle().
			Foreground(theme.LightDark(lipgloss.Color("#1F2937"), lipgloss.Color("#E5E7EB")))

	helpWhatStyle = lipgloss.NewStyle().
			Foreground(theme.LightDark(lipgloss.Color("#4A4A4A"), lipgloss.Color("#9B9B9B")))
)

// helpSections groups the bindings the way a reader looks for them, rather
// than by which type happens to own them. Two columns so the whole table fits
// a short terminal: a help popup that clips is worse than useless, and the App
// section holding "?" and "q" is the part that would go first.
var helpSections = [2][]struct {
	title string
	keys  []helpKey
}{
	{
		{"Playback", []helpKey{
			{"p  k", "play / pause"},
			{"j", "previous track"},
			{"l", "next track"},
		}},
		{"Sound", []helpKey{
			{"↑  ↓", "volume"},
			{"m", "mute"},
		}},
	},
	{
		{"Rooms", []helpKey{
			{"→  tab", "next room"},
			{"←  ⇧tab", "previous room"},
			{"1 … 9", "jump to room"},
			{"0", "last room"},
		}},
		{"App", []helpKey{
			{"esc", "close"},
			{"q", "quit"},
		}},
	},
}

// helpContent renders the shortcut table as modal content.
type helpContent struct{}

var _ tea.Model = helpContent{}

func (helpContent) Init() tea.Cmd { return nil }

func (h helpContent) Update(tea.Msg) (tea.Model, tea.Cmd) { return h, nil }

func (helpContent) View() tea.View {
	columns := make([]string, len(helpSections))
	for i, sections := range helpSections {
		var b strings.Builder
		for j, section := range sections {
			if j > 0 {
				b.WriteString("\n")
			}
			b.WriteString(helpSectionStyle.Render(section.title) + "\n")
			for _, key := range section.keys {
				b.WriteString(helpKeyStyle.Render(padRight(key.keys, helpKeyColumn)) +
					helpWhatStyle.Render(key.what) + "\n")
			}
		}
		columns[i] = strings.TrimRight(b.String(), "\n")
	}

	return tea.NewView(lipgloss.JoinHorizontal(lipgloss.Top,
		columns[0], strings.Repeat(" ", helpColumnGap), columns[1]))
}

// padRight pads to a display width, so the descriptions line up whatever the
// keys are. lipgloss.Width rather than len: the arrow glyphs are multi-byte.
func padRight(s string, width int) string {
	if pad := width - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s + " "
}

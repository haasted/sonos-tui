package nowplaying

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/haasted/sonos-sandbox/internal/ui/marquee"
	"github.com/haasted/sonos-sandbox/pkg/ui/spinners"
)

type Track struct {
	Title    string
	Artist   string
	Album    string
	Elapsed  time.Duration
	Duration time.Duration
	Playing  bool
}

type TrackMsg struct {
	Track Track
}

type ErrorMsg struct {
	Err error
}

var generations atomic.Int64

func nextGeneration() int {
	return int(generations.Add(1))
}

type scrollTickMsg struct {
	gen int
}

func scrollTick(gen int) tea.Cmd {
	return tea.Tick(scrollInterval, func(time.Time) tea.Msg {
		return scrollTickMsg{gen: gen}
	})
}

const positionTickInterval = time.Second

type positionTickMsg struct {
	gen int
}

func positionTick(gen int) tea.Cmd {
	return tea.Tick(positionTickInterval, func(time.Time) tea.Msg {
		return positionTickMsg{gen: gen}
	})
}

type Model struct {
	spinner spinner.Model
	playing bool

	title  marquee.Line
	artist marquee.Line
	album  marquee.Line

	baseline   time.Duration
	receivedAt time.Time
	duration   time.Duration

	ticksWaited int
	scrollGen   *int
	posGen      *int

	err error
}

func New(t Track) Model {
	s := spinner.New()
	s.Spinner = spinners.Equalizer

	m := Model{spinner: s, scrollGen: new(int), posGen: new(int)}
	return m.applyTrack(t)
}

func (m Model) applyTrack(t Track) Model {
	metaWindowWidth := titleRowWidth - lipgloss.Width(m.spinner.View()) - 1

	m.playing = t.Playing
	m.title = marquee.NewLine(t.Title, longTitleWindowWidth)
	m.artist = marquee.NewLine(t.Artist, metaWindowWidth)
	m.album = marquee.NewLine(t.Album, metaWindowWidth)
	m.baseline = t.Elapsed
	m.receivedAt = time.Now()
	m.duration = t.Duration
	m.ticksWaited = 0
	m.err = nil
	return m
}

func (m Model) elapsed() time.Duration {
	e := m.baseline
	if m.playing {
		e += time.Since(m.receivedAt)
	}
	if m.duration > 0 && e > m.duration {
		e = m.duration
	}
	return e
}

func (m Model) needsScroll() bool {
	return m.title.NeedsScroll() || m.artist.NeedsScroll() || m.album.NeedsScroll()
}

func (m Model) startCmds() tea.Cmd {
	var cmds []tea.Cmd
	if m.playing {
		cmds = append(cmds, m.spinner.Tick, positionTick(*m.posGen))
	}
	if m.needsScroll() {
		cmds = append(cmds, scrollTick(*m.scrollGen))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m Model) Init() tea.Cmd {
	m.restartTicks()
	return m.startCmds()
}

func (m Model) restartTicks() {
	*m.scrollGen = nextGeneration()
	*m.posGen = nextGeneration()
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TrackMsg:
		m = m.applyTrack(msg.Track)
		m.restartTicks()
		return m, m.startCmds()
	case ErrorMsg:
		m.err = msg.Err
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case positionTickMsg:
		if msg.gen != *m.posGen || !m.playing {
			return m, nil
		}
		return m, positionTick(msg.gen)
	case scrollTickMsg:
		if msg.gen != *m.scrollGen {
			return m, nil
		}
		if m.ticksWaited < scrollDelayTicks {
			m.ticksWaited++
			return m, scrollTick(msg.gen)
		}
		if m.title.NeedsScroll() {
			m.title = m.title.Advance()
		}
		if m.artist.NeedsScroll() {
			m.artist = m.artist.Advance()
		}
		if m.album.NeedsScroll() {
			m.album = m.album.Advance()
		}
		return m, scrollTick(msg.gen)
	}
	return m, nil
}

func (m Model) View() string {
	spinnerGlyph := m.spinner.View()
	if !m.playing {
		spinnerGlyph = pausedGlyph
	}
	left := lipgloss.JoinHorizontal(lipgloss.Top, spinnerGlyph, " ", m.title.Render(trackStyle))
	position := formatPosition(m.elapsed(), m.duration)

	gap := titleRowWidth - lipgloss.Width(left) - lipgloss.Width(position)
	if gap < 1 {
		gap = 1
	}
	titleLine := left + strings.Repeat(" ", gap) + position

	indent := strings.Repeat(" ", lipgloss.Width(spinnerGlyph)+1)
	lines := []string{
		titleLine,
		indent + m.artist.Render(artistStyle),
		indent + m.album.Render(albumStyle),
	}
	if m.err != nil {
		lines = append(lines, errorStyle.Render("⚠ "+m.err.Error()))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func formatPosition(elapsed, duration time.Duration) string {
	return timeStyle.Render(formatDuration(elapsed) + " / " + formatDuration(duration))
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d/time.Minute), int((d%time.Minute)/time.Second))
}

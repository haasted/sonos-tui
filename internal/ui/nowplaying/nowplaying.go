package nowplaying

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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
	Muted    bool
	Volume   int
}

type TrackMsg struct {
	Track Track
}

// VolumeMsg reports volume and mute without claiming anything about the
// track. Commands that cannot move playback send this instead of a TrackMsg,
// whose Elapsed would reset the clock to a stale position.
type VolumeMsg struct {
	Volume int
	Muted  bool

	// Show forces the readout even when the level is unchanged, for arriving
	// at a speaker's tab rather than adjusting it.
	Show bool
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

const fadeTickInterval = 40 * time.Millisecond

type fadeTickMsg struct {
	gen int
}

func fadeTick(gen int) tea.Cmd {
	return tea.Tick(fadeTickInterval, func(time.Time) tea.Msg {
		return fadeTickMsg{gen: gen}
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

	muted   bool
	mutedAt time.Time // when mute last switched on, driving the flash fade

	volume            int
	volumeKnown       bool      // suppresses the readout on the first track, which has no previous level
	volumeRising      bool      // direction of the last change, picking the arrow
	volumeDirectional bool      // false when shown on arrival, where no arrow applies
	volumeShownAt     time.Time // when the readout last appeared, driving its fade

	fadeGen *int // one ticker drives both the mute and volume fades

	title  marquee.Line
	artist marquee.Line
	album  marquee.Line

	baseline   time.Duration
	receivedAt time.Time
	duration   time.Duration

	ticksWaited int
	scrollGen   *int
	posGen      *int
}

func New(track Track) Model {
	s := spinner.New()
	s.Spinner = spinners.Equalizer

	m := Model{spinner: s, scrollGen: new(int), posGen: new(int), fadeGen: new(int)}
	return m.applyTrack(track)
}

func (m Model) applyTrack(track Track) Model {
	metaWindowWidth := titleRowWidth - lipgloss.Width(m.spinner.View()) - 1

	if m.volumeKnown && track.Volume != m.volume {
		m.volumeDirectional = true
		m.volumeRising = track.Volume > m.volume
		m.volumeShownAt = time.Now()
	}
	m.volume = track.Volume
	m.volumeKnown = true

	if track.Muted && !m.muted {
		m.mutedAt = time.Now()
	}
	if !track.Muted {
		m.mutedAt = time.Time{}
	}
	m.muted = track.Muted
	m.playing = track.Playing
	m.title = marquee.NewLine(track.Title, longTitleWindowWidth)
	m.artist = marquee.NewLine(track.Artist, metaWindowWidth)
	m.album = marquee.NewLine(track.Album, metaWindowWidth)
	m.baseline = track.Elapsed
	m.receivedAt = time.Now()
	m.duration = track.Duration
	m.ticksWaited = 0
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

// muteFlash is 1 while the flash is held, easing to 0 across the fade.
func (m Model) muteFlash() float64 {
	return easedFlash(m.mutedAt, muteFlashHold, muteFlashFade)
}

// easedFlash holds at 1 for hold, then smoothsteps to 0 across fade.
func easedFlash(since time.Time, hold, fade time.Duration) float64 {
	if since.IsZero() {
		return 0
	}
	elapsed := time.Since(since)
	if elapsed < hold {
		return 1
	}
	left := hold + fade - elapsed
	if left <= 0 {
		return 0
	}
	t := float64(left) / float64(fade)
	return t * t * (3 - 2*t)
}

// applyVolume records a volume/mute update, leaving the clock and the track
// untouched. Show makes the readout appear even without a change.
func (m Model) applyVolume(msg VolumeMsg) Model {
	if changed := m.volumeKnown && msg.Volume != m.volume; changed || msg.Show {
		m.volumeDirectional = changed
		m.volumeRising = msg.Volume >= m.volume
		m.volumeShownAt = time.Now()
	}
	m.volume = msg.Volume
	m.volumeKnown = true

	if msg.Muted && !m.muted {
		m.mutedAt = time.Now()
	}
	if !msg.Muted {
		m.mutedAt = time.Time{}
	}
	m.muted = msg.Muted
	return m
}

// volumeFlash is 1 while the readout is held, easing to 0 across its fade.
func (m Model) volumeFlash() float64 {
	return easedFlash(m.volumeShownAt, volumeHold, volumeFade)
}

// fading reports whether any transient overlay still needs redrawing.
func (m Model) fading() bool {
	return m.muteFlash() > 0 || m.volumeFlash() > 0
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
	if m.fading() {
		cmds = append(cmds, fadeTick(*m.fadeGen))
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
	*m.fadeGen = nextGeneration()
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case TrackMsg:
		m = m.applyTrack(msg.Track)
		m.restartTicks()
		return m, m.startCmds()
	case VolumeMsg:
		m = m.applyVolume(msg)
		return m, m.startCmds()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case fadeTickMsg:
		if msg.gen != *m.fadeGen || !m.fading() {
			return m, nil
		}
		return m, fadeTick(msg.gen)
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
	if m.muted {
		spinnerGlyph = muteGlyphStyle(m.muteFlash()).Render(mutedGlyph)
	}
	left := lipgloss.JoinHorizontal(lipgloss.Top, spinnerGlyph, " ", m.title.Render(trackStyle))
	position := formatPosition(m.elapsed(), m.duration)

	gap := titleRowWidth - lipgloss.Width(left) - lipgloss.Width(position)
	if gap < 1 {
		gap = 1
	}
	titleLine := left + strings.Repeat(" ", gap) + position

	indent := strings.Repeat(" ", lipgloss.Width(spinnerGlyph)+1)

	albumLine := indent + m.album.Render(albumStyle)
	if strength := m.volumeFlash(); strength > 0 {
		readout := volumeStyle(strength).Render(m.volumeReadout())
		narrowed := titleRowWidth - lipgloss.Width(indent) - volumeSlotWidth
		albumLine = indent + m.album.RenderWidth(albumStyle, narrowed)
		gap := max(titleRowWidth-lipgloss.Width(albumLine)-lipgloss.Width(readout), 1)
		albumLine += strings.Repeat(" ", gap) + readout
	}

	lines := []string{
		titleLine,
		indent + m.artist.Render(artistStyle),
		albumLine,
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// volumeReadout is the level, prefixed with a direction arrow only when the
// volume actually moved. Arriving at a speaker shows the level alone: nothing
// is going up or down, so an arrow would be a lie.
func (m Model) volumeReadout() string {
	if !m.volumeDirectional {
		return fmt.Sprintf("%d", m.volume)
	}
	arrow := volumeDownGlyph
	if m.volumeRising {
		arrow = volumeUpGlyph
	}
	return fmt.Sprintf("%s %d", arrow, m.volume)
}

func formatPosition(elapsed, duration time.Duration) string {
	return timeStyle.Render(formatDuration(elapsed) + " / " + formatDuration(duration))
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d/time.Minute), int((d%time.Minute)/time.Second))
}

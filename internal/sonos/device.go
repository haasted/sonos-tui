package sonos

import (
	"log/slog"
	"sync"
)

// eventChanBuffer bounds how many events can queue for a Device before
// HandleEvent starts dropping them. Small on purpose: AVTransport events
// fire on track/state changes, not continuously.
const eventChanBuffer = 8

// Device wraps a Client with the identity and last-known state of one
// physical Sonos speaker. Client stays a stateless SOAP transport; Device
// adds the "what do we currently believe about this speaker" layer on top.
type Device struct {
	Speaker

	client *SonosClient

	mu    sync.RWMutex
	state DeviceState

	events chan Event
}

// DeviceState is a point-in-time snapshot of what Device knows about the
// speaker, without going to the network.
type DeviceState struct {
	Transport TransportInfo
	Current   TrackMetadata
	RelTime   string
}

// Event reports that a speaker's state changed. Transport and Current are
// the freshly applied state, lifted out for cheap consumption; Raw carries
// everything else the speaker sent (next track, queue source, play mode).
//
// Deliberately free of any UI framework types — a Bubbletea program can
// forward these as tea.Msg values directly, without this package knowing
// anything about Bubbletea.
type Event struct {
	Speaker   Speaker
	Transport TransportInfo
	Current   TrackMetadata
	Raw       AVTransportEvent
}

// NewDevice creates a Device for the given discovered speaker.
func NewDevice(speaker Speaker) *Device {
	return NewDeviceWithClient(speaker, NewClient(speaker.IP))
}

// NewDeviceWithClient creates a Device using a pre-built Client, e.g. one
// pointed at a test server instead of the speaker's real IP.
func NewDeviceWithClient(speaker Speaker, client *SonosClient) *Device {
	return &Device{
		Speaker: speaker,
		client:  client,
		events:  make(chan Event, eventChanBuffer),
	}
}

// Events returns the stream of state changes reported by the speaker,
// populated once this Device is registered with an EventManager. Never
// closed: a receiver can range over it for the lifetime of the process.
func (d *Device) Events() <-chan Event {
	return d.events
}

// HandleEvent applies a GENA NOTIFY body to this Device's state and
// publishes the result on Events.
//
// LastChange usually omits RelativeTimePosition (see AVTransportEvent), so
// taking the event at face value would snap the reported position back to
// zero on every play/pause. One GetPositionInfo call fills it in instead —
// which also picks up seeks performed from another controller, since those
// arrive as an event with no position of their own.
func (d *Device) HandleEvent(body []byte) error {
	parsed, err := ParseAVTransportEvent(body)
	if err != nil {
		return err
	}

	relTime := parsed.RelTime
	if relTime == "" {
		info, err := d.client.GetPositionInfo()
		if err != nil {
			slog.Warn("fetching position after event", "speaker", d.Speaker, "err", err)
		} else {
			relTime = info.RelTime
		}
	}

	d.mu.Lock()
	if parsed.TransportState != "" {
		d.state.Transport.State = parsed.TransportState
	}
	d.state.Current = parsed.Current
	d.state.RelTime = relTime
	snapshot := d.state
	d.mu.Unlock()

	event := Event{
		Speaker:   d.Speaker,
		Transport: snapshot.Transport,
		Current:   snapshot.Current,
		Raw:       parsed,
	}

	// Never block: this runs on the listener's request goroutine, and a
	// consumer that has stopped receiving (the UI shutting down, say)
	// would otherwise wedge it and stall the listener's own shutdown.
	// State is already applied above, so a dropped event only costs a
	// redraw, not correctness.
	select {
	case d.events <- event:
	default:
		slog.Warn("event channel full — dropping event", "speaker", d.Speaker)
	}

	return nil
}

// State returns a copy of the last-known state. It never touches the
// network — call Refresh first to populate it.
func (d *Device) State() DeviceState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

// Refresh fetches the current transport state from the speaker.
func (d *Device) Refresh() error {
	info, err := d.client.GetTransportInfo()
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.state.Transport = *info
	d.mu.Unlock()
	return nil
}

func (d *Device) RefreshTrack() error {
	info, err := d.client.GetPositionInfo()
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.state.Current = TrackMetadata{
		URI:         info.TrackURI,
		Duration:    info.TrackDuration,
		Title:       info.Title,
		Artist:      info.Artist,
		Album:       info.Album,
		AlbumArtURI: info.AlbumArtURI,
	}
	d.state.RelTime = info.RelTime
	d.mu.Unlock()
	return nil
}

// Play starts or resumes playback.
func (d *Device) Play() error {
	if err := d.client.Play(); err != nil {
		return err
	}
	d.mu.Lock()
	d.state.Transport.State = "PLAYING"
	d.mu.Unlock()
	return nil
}

// Pause pauses playback.
func (d *Device) Pause() error {
	if err := d.client.Pause(); err != nil {
		return err
	}
	d.mu.Lock()
	d.state.Transport.State = "PAUSED_PLAYBACK"
	d.mu.Unlock()
	return nil
}

func (d *Device) Next() error {
	if err := d.client.Next(); err != nil {
		return err
	}
	d.mu.Lock()
	d.state.Transport.State = "PLAYING"
	d.mu.Unlock()
	return nil
}

func (d *Device) Previous() error {
	if err := d.client.Previous(); err != nil {
		return err
	}
	d.mu.Lock()
	d.state.Transport.State = "PLAYING"
	d.mu.Unlock()
	return nil
}

package sonos

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
)

const eventChanBuffer = 8

// Device wraps a Client with the identity and last-known state of one
// physical Sonos speaker. Client stays a stateless SOAP transport; Device
// adds the "what do we currently believe about this speaker" layer on top.
type Device struct {
	Speaker

	client *SonosClient

	mu    sync.RWMutex
	state DeviceState

	queueGen atomic.Int64 // guards queue reads, so a slow one cannot overwrite a newer

	events chan Event
}

// DeviceState is a point-in-time snapshot of what Device knows about the
// speaker, without going to the network.
type DeviceState struct {
	Transport TransportInfo
	Current   TrackMetadata
	RelTime   string

	Volume int
	Muted  bool

	// Source is what is loaded on the speaker — a queue, a station, a line-in.
	// Distinct from Current, which is whatever is coming out of it right now.
	Source SourceInfo

	Track  int // 1-based position in the queue, as the speaker counts it
	Tracks int // how many the speaker says the queue holds

	// Queue is the speaker's queue as it was last read. A slice, so copying
	// DeviceState copies a header rather than the items -- which means every
	// copy shares one backing array, and the queue must always be replaced
	// wholesale rather than edited in place.
	Queue []QueueItem

	// QueueKnown says the queue above was read, as against not read yet. An
	// empty queue is a real answer and cannot be told from a missing one by
	// length, nor by QueueUpdateID, whose zero is also a real value.
	QueueKnown bool

	// QueueUpdateID is the speaker's own version number for its queue. It
	// rises whenever the queue changes, so a copy read at a lower id is
	// stale. Zero until an event or a Browse has reported one.
	QueueUpdateID int
}

// Live reports playback with no meaningful duration: a broadcast, or a station
// between tracks. It asks about the DURATION rather than the source kind,
// because an algorithmic station can play discrete tracks whose clock works
// perfectly well — and it does not ask whether playback is running, since a
// paused stream's clock is no more meaningful than a playing one's.
//
// Requiring a known source is what keeps a speaker whose state has not loaded
// yet from being reported as a stream: everything is zero at that point,
// including the duration.
func (s DeviceState) Live() bool {
	return s.Source.URI != "" &&
		ParseDuration(s.Current.Duration) == 0 &&
		s.Source.Kind() != SourceTrack
}

// Playing reports whether the speaker is actively playing. Every other
// transport state — paused, stopped, transitioning — counts as idle.
func (s DeviceState) Playing() bool {
	return s.Transport.State == transportPlaying
}

// EventKind says which service produced an event. Volume events carry no
// playback position, so consumers must not treat them as a position update.
type EventKind int

const (
	EventAVTransport EventKind = iota
	EventRenderingControl
	EventContentDirectory
)

type Event struct {
	Kind      EventKind
	Speaker   Speaker
	Transport TransportInfo
	Current   TrackMetadata
	Raw       AVTransportEvent

	// Directory is set on a ContentDirectory event only. It says which
	// containers changed, and carries no playback state at all.
	Directory ContentDirectoryEvent
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

func (d *Device) Events() <-chan Event {
	return d.events
}

func (d *Device) HandleAVTransportEvent(body []byte) error {
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
	if parsed.Source.URI != "" {
		// LastChange omits unchanged variables, so an event that says nothing
		// about the source must not erase what we already know.
		d.state.Source = parsed.Source
	}
	d.state.Current = parsed.Current
	d.state.Track = parsed.Track
	d.state.Tracks = parsed.NumberOfTracks
	d.state.RelTime = relTime
	snapshot := d.state
	d.mu.Unlock()

	event := Event{
		Kind:      EventAVTransport,
		Speaker:   d.Speaker,
		Transport: snapshot.Transport,
		Current:   snapshot.Current,
		Raw:       parsed,
	}

	select {
	case d.events <- event:
	default:
		slog.Warn("event channel full — dropping event", "speaker", d.Speaker)
	}

	return nil
}

// HandleRenderingControlEvent applies a volume/mute event and publishes it.
// Unlike AVTransport it never refetches position: volume changes say nothing
// about playback progress.
func (d *Device) HandleRenderingControlEvent(body []byte) error {
	parsed, err := ParseRenderingControlEvent(body)
	if err != nil {
		return err
	}

	d.mu.Lock()
	if parsed.HasVolume {
		d.state.Volume = parsed.Volume
	}
	if parsed.HasMute {
		d.state.Muted = parsed.Muted
	}
	snapshot := d.state
	d.mu.Unlock()

	event := Event{
		Kind:      EventRenderingControl,
		Speaker:   d.Speaker,
		Transport: snapshot.Transport,
		Current:   snapshot.Current,
	}

	select {
	case d.events <- event:
	default:
		slog.Warn("event channel full — dropping event", "speaker", d.Speaker)
	}

	return nil
}

// HandleContentDirectoryEvent records which containers changed and publishes
// the fact. It fetches nothing: the event says the queue moved, not what it
// now holds, and a Browse for every speaker on every change is a cost the
// caller should choose rather than inherit.
func (d *Device) HandleContentDirectoryEvent(body []byte) error {
	parsed, err := ParseContentDirectoryEvent(body)
	if err != nil {
		return err
	}

	d.mu.Lock()
	if updateID, named := parsed.QueueUpdate(); named {
		d.state.QueueUpdateID = updateID
	}
	snapshot := d.state
	d.mu.Unlock()

	event := Event{
		Kind:      EventContentDirectory,
		Speaker:   d.Speaker,
		Transport: snapshot.Transport,
		Current:   snapshot.Current,
		Directory: parsed,
	}

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

func (d *Device) RefreshVolume() error {
	var (
		wg      sync.WaitGroup
		volume  int
		muted   bool
		volErr  error
		muteErr error
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		volume, volErr = d.client.GetVolume()
	}()
	go func() {
		defer wg.Done()
		muted, muteErr = d.client.GetMute()
	}()
	wg.Wait()

	if err := errors.Join(volErr, muteErr); err != nil {
		return err
	}

	d.mu.Lock()
	d.state.Volume = volume
	d.state.Muted = muted
	d.mu.Unlock()
	return nil
}

// Queue reads the speaker's playback queue. Deliberately not stored on
// DeviceState: nothing renders it yet, and a queue that went stale in state
// would be worse than one fetched when asked for.
func (d *Device) Queue() (Queue, error) {
	return d.client.GetQueue()
}

// RefreshQueue reads the queue and stores it. Guarded by a generation, because
// a queue can be announced twice within a second while a read takes about a
// tenth of one: two reads are then in flight, and the slower one must not
// overwrite the newer answer it raced.
func (d *Device) RefreshQueue() error {
	generation := d.queueGen.Add(1)

	queue, err := d.client.GetQueue()
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if generation != d.queueGen.Load() {
		slog.Debug("dropping a queue read a newer one overtook", "speaker", d.Speaker)
		return nil
	}
	d.state.Queue = queue.Items
	d.state.QueueUpdateID = queue.UpdateID
	d.state.QueueKnown = true
	return nil
}

func (d *Device) RefreshAll() error {
	refreshers := []func() error{d.Refresh, d.RefreshTrack, d.RefreshVolume}
	errs := make([]error, len(refreshers))

	var wg sync.WaitGroup
	wg.Add(len(refreshers))
	for i, refresh := range refreshers {
		go func() {
			defer wg.Done()
			errs[i] = refresh()
		}()
	}
	wg.Wait()

	return errors.Join(errs...)
}

func (d *Device) AdjustVolume(adjustment int) (int, error) {
	d.mu.RLock()
	muted := d.state.Muted
	d.mu.RUnlock()

	if muted {
		if err := d.SetMuted(false); err != nil {
			return 0, fmt.Errorf("adjust volume: %w", err)
		}
	}

	volume, err := d.client.SetRelativeVolume(adjustment)
	if err != nil {
		return 0, fmt.Errorf("adjust volume: %w", err)
	}

	d.mu.Lock()
	d.state.Volume = volume
	d.mu.Unlock()
	return volume, nil
}

func (d *Device) SetMuted(muted bool) error {
	if err := d.client.SetMute(muted); err != nil {
		return fmt.Errorf("set mute: %w", err)
	}
	d.mu.Lock()
	d.state.Muted = muted
	d.mu.Unlock()
	return nil
}

func (d *Device) ToggleMute() error {
	d.mu.RLock()
	muted := d.state.Muted
	d.mu.RUnlock()
	return d.SetMuted(!muted)
}

// Play starts or resumes playback.
func (d *Device) Play() error {
	if err := d.client.Play(); err != nil {
		return fmt.Errorf("play: %w", err)
	}
	d.mu.Lock()
	d.state.Transport.State = transportPlaying
	d.mu.Unlock()
	return nil
}

// Pause pauses playback.
func (d *Device) Pause() error {
	if err := d.client.Pause(); err != nil {
		return fmt.Errorf("pause: %w", err)
	}
	d.mu.Lock()
	d.state.Transport.State = "PAUSED_PLAYBACK"
	d.mu.Unlock()
	return nil
}

func (d *Device) Next() error {
	if err := d.client.Next(); err != nil {
		return fmt.Errorf("next track: %w", err)
	}
	d.mu.Lock()
	d.state.Transport.State = transportPlaying
	d.mu.Unlock()
	return nil
}

func (d *Device) Previous() error {
	if err := d.client.Previous(); err != nil {
		return fmt.Errorf("previous track: %w", err)
	}
	d.mu.Lock()
	d.state.Transport.State = transportPlaying
	d.mu.Unlock()
	return nil
}

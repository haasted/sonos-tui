package sonos

import (
	"errors"
	"log/slog"
	"sync"
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
}

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

func (d *Device) Events() <-chan Event {
	return d.events
}

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
			return 0, err
		}
	}

	volume, err := d.client.SetRelativeVolume(adjustment)
	if err != nil {
		return 0, err
	}

	d.mu.Lock()
	d.state.Volume = volume
	d.mu.Unlock()
	return volume, nil
}

func (d *Device) SetMuted(muted bool) error {
	if err := d.client.SetMute(muted); err != nil {
		return err
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

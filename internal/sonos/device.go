package sonos

import "sync"

// Device wraps a Client with the identity and last-known state of one
// physical Sonos speaker. Client stays a stateless SOAP transport; Device
// adds the "what do we currently believe about this speaker" layer on top.
type Device struct {
	Speaker

	client *SonosClient

	mu    sync.RWMutex
	state DeviceState
}

// DeviceState is a point-in-time snapshot of what Device knows about the
// speaker, without going to the network.
type DeviceState struct {
	Transport TransportInfo
}

// NewDevice creates a Device for the given discovered speaker.
func NewDevice(speaker Speaker) *Device {
	return NewDeviceWithClient(speaker, NewClient(speaker.IP))
}

// NewDeviceWithClient creates a Device using a pre-built Client, e.g. one
// pointed at a test server instead of the speaker's real IP.
func NewDeviceWithClient(speaker Speaker, client *SonosClient) *Device {
	return &Device{Speaker: speaker, client: client}
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

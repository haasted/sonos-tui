package sonos

import (
	"encoding/xml"
	"fmt"
)

// serviceURNs are the UPnP service XML namespaces used both as SOAPAction
// URNs and as the xmlns:u attribute on every SOAP body in this file.
const (
	avtransportService = "urn:schemas-upnp-org:service:AVTransport:1"
	renderingService   = "urn:schemas-upnp-org:service:RenderingControl:1"
)

// --- AVTransport (playback control) ------------------------------------------

const avtransportPath = "/MediaRenderer/AVTransport/Control"

func (c *SonosClient) avTransport(action, body string) ([]byte, error) {
	return c.Do(avtransportPath, avtransportService, action, body)
}

// Play starts or resumes playback.
func (c *SonosClient) Play() error {
	_, err := c.avTransport("Play", fmt.Sprintf(
		`<u:Play xmlns:u="%s">
      <InstanceID>0</InstanceID>
      <Speed>1</Speed>
    </u:Play>`, avtransportService))
	return err
}

// Pause pauses playback.
func (c *SonosClient) Pause() error {
	_, err := c.avTransport("Pause", simplePayload(avtransportService, "Pause"))
	return err
}

// Stop stops playback.
func (c *SonosClient) Stop() error {
	_, err := c.avTransport("Stop", simplePayload(avtransportService, "Stop"))
	return err
}

// Next skips to the next track.
func (c *SonosClient) Next() error {
	_, err := c.avTransport("Next", simplePayload(avtransportService, "Next"))
	return err
}

// Previous goes back to the previous track.
func (c *SonosClient) Previous() error {
	_, err := c.avTransport("Previous", simplePayload(avtransportService, "Previous"))
	return err
}

// TransportInfo holds the playback state returned by GetTransportInfo.
type TransportInfo struct {
	State  string // "PLAYING", "PAUSED_PLAYBACK", "STOPPED"
	Status string // "OK"
	Speed  string // "1"
}

// simplePayload builds a SOAP body for actions whose only argument is
// InstanceID, e.g. <u:Pause xmlns:u="service"><InstanceID>0</InstanceID></u:Pause>.
func simplePayload(service, action string) string {
	return fmt.Sprintf(`<u:%[2]s xmlns:u="%[1]s"><InstanceID>0</InstanceID></u:%[2]s>`, service, action)
}

// GetTransportInfo returns the current playback state of the speaker.
func (c *SonosClient) GetTransportInfo() (*TransportInfo, error) {
	data, err := c.avTransport("GetTransportInfo", simplePayload(avtransportService, "GetTransportInfo"))
	if err != nil {
		return nil, err
	}

	// A dotted path on XMLName only matches the document's actual root
	// element, not a descent path — put the path on plain fields instead,
	// like GetVolume/GetMute do below.
	var resp struct {
		State  string `xml:"Body>GetTransportInfoResponse>CurrentTransportState"`
		Status string `xml:"Body>GetTransportInfoResponse>CurrentTransportStatus"`
		Speed  string `xml:"Body>GetTransportInfoResponse>CurrentSpeed"`
	}
	if err := xml.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing GetTransportInfo response: %w", err)
	}
	return &TransportInfo{
		State:  resp.State,
		Status: resp.Status,
		Speed:  resp.Speed,
	}, nil
}

// TrackInfo holds the current track metadata returned by GetPositionInfo.
type TrackInfo struct {
	Track         string // Queue position
	TrackDuration string // HH:MM:SS
	TrackURI      string // x-sonos-http:..., x-rincon-stream:..., etc.
	RelTime       string // Current position HH:MM:SS
	Title         string // From DIDL-Lite metadata
	Artist        string
	Album         string
	AlbumArtURI   string // Relative URL — prepend speaker BaseURL
}

// GetPositionInfo returns the current track position and metadata.
func (c *SonosClient) GetPositionInfo() (*TrackInfo, error) {
	data, err := c.avTransport("GetPositionInfo", simplePayload(avtransportService, "GetPositionInfo"))
	if err != nil {
		return nil, err
	}

	var resp struct {
		Track         string `xml:"Track"`
		TrackDuration string `xml:"TrackDuration"`
		TrackMetaData string `xml:"TrackMetaData"`
		TrackURI      string `xml:"TrackURI"`
		RelTime       string `xml:"RelTime"`
	}
	// Strip the outer SOAP envelope to get to the response element.
	var envelope struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    struct {
			Response struct {
				Track         string `xml:"Track"`
				TrackDuration string `xml:"TrackDuration"`
				TrackMetaData string `xml:"TrackMetaData"`
				TrackURI      string `xml:"TrackURI"`
				RelTime       string `xml:"RelTime"`
			} `xml:"GetPositionInfoResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("parsing GetPositionInfo response: %w", err)
	}
	resp = envelope.Body.Response

	// TrackMetaData is XML-escaped DIDL-Lite XML, same shape events.go's
	// AVTransportEvent parses out of LastChange — shared via parseDIDLTrack.
	meta := parseDIDLTrack(resp.TrackMetaData)

	return &TrackInfo{
		Track:         resp.Track,
		TrackDuration: resp.TrackDuration,
		TrackURI:      resp.TrackURI,
		RelTime:       resp.RelTime,
		Title:         meta.Title,
		Artist:        meta.Artist,
		Album:         meta.Album,
		AlbumArtURI:   meta.AlbumArtURI,
	}, nil
}

// --- RenderingControl (volume & mute) ----------------------------------------

const renderingPath = "/MediaRenderer/RenderingControl/Control"

func (c *SonosClient) rendering(action, body string) ([]byte, error) {
	return c.Do(renderingPath, renderingService, action, body)
}

// GetVolume returns the current Master volume (0–100).
func (c *SonosClient) GetVolume() (int, error) {
	data, err := c.rendering("GetVolume", fmt.Sprintf(
		`<u:GetVolume xmlns:u="%s">
      <InstanceID>0</InstanceID>
      <Channel>Master</Channel>
    </u:GetVolume>`, renderingService))
	if err != nil {
		return 0, err
	}

	var resp struct {
		Volume int `xml:"Body>GetVolumeResponse>CurrentVolume"`
	}
	if err := xml.Unmarshal(data, &resp); err != nil {
		return 0, fmt.Errorf("parsing GetVolume response: %w", err)
	}
	return resp.Volume, nil
}

// SetVolume sets the Master volume (0–100).
func (c *SonosClient) SetVolume(level int) error {
	if level < 0 || level > 100 {
		return fmt.Errorf("volume must be between 0 and 100, got %d", level)
	}
	_, err := c.rendering("SetVolume", fmt.Sprintf(
		`<u:SetVolume xmlns:u="%s">
      <InstanceID>0</InstanceID>
      <Channel>Master</Channel>
      <DesiredVolume>%d</DesiredVolume>
    </u:SetVolume>`, renderingService, level))
	return err
}

func (c *SonosClient) SetRelativeVolume(adjustment int) (int, error) {
	data, err := c.rendering("SetRelativeVolume", fmt.Sprintf(
		`<u:SetRelativeVolume xmlns:u="%s">
      <InstanceID>0</InstanceID>
      <Channel>Master</Channel>
      <Adjustment>%d</Adjustment>
    </u:SetRelativeVolume>`, renderingService, adjustment))
	if err != nil {
		return 0, err
	}

	var resp struct {
		Volume int `xml:"Body>SetRelativeVolumeResponse>NewVolume"`
	}
	if err := xml.Unmarshal(data, &resp); err != nil {
		return 0, fmt.Errorf("parsing SetRelativeVolume response: %w", err)
	}
	return resp.Volume, nil
}

// GetMute returns true if the speaker is muted.
func (c *SonosClient) GetMute() (bool, error) {
	data, err := c.rendering("GetMute", fmt.Sprintf(
		`<u:GetMute xmlns:u="%s">
      <InstanceID>0</InstanceID>
      <Channel>Master</Channel>
    </u:GetMute>`, renderingService))
	if err != nil {
		return false, err
	}

	var resp struct {
		Mute int `xml:"Body>GetMuteResponse>CurrentMute"`
	}
	if err := xml.Unmarshal(data, &resp); err != nil {
		return false, fmt.Errorf("parsing GetMute response: %w", err)
	}
	return resp.Mute == 1, nil
}

// SetMute mutes or unmutes the speaker.
func (c *SonosClient) SetMute(muted bool) error {
	val := 0
	if muted {
		val = 1
	}
	_, err := c.rendering("SetMute", fmt.Sprintf(
		`<u:SetMute xmlns:u="%s">
      <InstanceID>0</InstanceID>
      <Channel>Master</Channel>
      <DesiredMute>%d</DesiredMute>
    </u:SetMute>`, renderingService, val))
	return err
}

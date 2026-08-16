package sonos

import (
	"encoding/xml"
	"fmt"
	"html"
)

// --- AVTransport (playback control) ------------------------------------------

const (
	avtransportPath    = "/MediaRenderer/AVTransport/Control"
	avtransportService = "urn:schemas-upnp-org:service:AVTransport:1"
)

func (c *Client) avTransport(action, body string) ([]byte, error) {
	return c.Do(avtransportPath, avtransportService, action, body)
}

// Play starts or resumes playback.
func (c *Client) Play() error {
	_, err := c.avTransport("Play",
		`<u:Play xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <InstanceID>0</InstanceID>
      <Speed>1</Speed>
    </u:Play>`)
	return err
}

// Pause pauses playback.
func (c *Client) Pause() error {
	_, err := c.avTransport("Pause",
		`<u:Pause xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <InstanceID>0</InstanceID>
    </u:Pause>`)
	return err
}

// Stop stops playback.
func (c *Client) Stop() error {
	_, err := c.avTransport("Stop",
		`<u:Stop xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <InstanceID>0</InstanceID>
    </u:Stop>`)
	return err
}

// Next skips to the next track.
func (c *Client) Next() error {
	_, err := c.avTransport("Next",
		`<u:Next xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <InstanceID>0</InstanceID>
    </u:Next>`)
	return err
}

// Previous goes back to the previous track.
func (c *Client) Previous() error {
	_, err := c.avTransport("Previous",
		`<u:Previous xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <InstanceID>0</InstanceID>
    </u:Previous>`)
	return err
}

// TransportInfo holds the playback state returned by GetTransportInfo.
type TransportInfo struct {
	State  string // "PLAYING", "PAUSED_PLAYBACK", "STOPPED"
	Status string // "OK"
	Speed  string // "1"
}

// GetTransportInfo returns the current playback state of the speaker.
func (c *Client) GetTransportInfo() (*TransportInfo, error) {
	data, err := c.avTransport("GetTransportInfo",
		`<u:GetTransportInfo xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <InstanceID>0</InstanceID>
    </u:GetTransportInfo>`)
	if err != nil {
		return nil, err
	}

	var resp struct {
		XMLName xml.Name `xml:"Envelope>Body>GetTransportInfoResponse"`
		State   string   `xml:"CurrentTransportState"`
		Status  string   `xml:"CurrentTransportStatus"`
		Speed   string   `xml:"CurrentSpeed"`
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
func (c *Client) GetPositionInfo() (*TrackInfo, error) {
	data, err := c.avTransport("GetPositionInfo",
		`<u:GetPositionInfo xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
      <InstanceID>0</InstanceID>
    </u:GetPositionInfo>`)
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

	info := &TrackInfo{
		Track:         resp.Track,
		TrackDuration: resp.TrackDuration,
		TrackURI:      resp.TrackURI,
		RelTime:       resp.RelTime,
	}

	// TrackMetaData is XML-escaped DIDL-Lite XML — decode and parse it.
	if resp.TrackMetaData != "" && resp.TrackMetaData != "NOT_IMPLEMENTED" {
		didl := html.UnescapeString(resp.TrackMetaData)
		var didlDoc struct {
			Item struct {
				Title       string `xml:"title"`
				Creator     string `xml:"creator"`
				Album       string `xml:"album"`
				AlbumArtURI string `xml:"albumArtURI"`
			} `xml:"item"`
		}
		if err := xml.Unmarshal([]byte(didl), &didlDoc); err == nil {
			info.Title = html.UnescapeString(didlDoc.Item.Title)
			info.Artist = html.UnescapeString(didlDoc.Item.Creator)
			info.Album = html.UnescapeString(didlDoc.Item.Album)
			info.AlbumArtURI = didlDoc.Item.AlbumArtURI
		}
	}

	return info, nil
}

// --- RenderingControl (volume & mute) ----------------------------------------

const (
	renderingPath    = "/MediaRenderer/RenderingControl/Control"
	renderingService = "urn:schemas-upnp-org:service:RenderingControl:1"
)

func (c *Client) rendering(action, body string) ([]byte, error) {
	return c.Do(renderingPath, renderingService, action, body)
}

// GetVolume returns the current Master volume (0–100).
func (c *Client) GetVolume() (int, error) {
	data, err := c.rendering("GetVolume",
		`<u:GetVolume xmlns:u="urn:schemas-upnp-org:service:RenderingControl:1">
      <InstanceID>0</InstanceID>
      <Channel>Master</Channel>
    </u:GetVolume>`)
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
func (c *Client) SetVolume(level int) error {
	if level < 0 || level > 100 {
		return fmt.Errorf("volume must be between 0 and 100, got %d", level)
	}
	_, err := c.rendering("SetVolume", fmt.Sprintf(
		`<u:SetVolume xmlns:u="urn:schemas-upnp-org:service:RenderingControl:1">
      <InstanceID>0</InstanceID>
      <Channel>Master</Channel>
      <DesiredVolume>%d</DesiredVolume>
    </u:SetVolume>`, level))
	return err
}

// GetMute returns true if the speaker is muted.
func (c *Client) GetMute() (bool, error) {
	data, err := c.rendering("GetMute",
		`<u:GetMute xmlns:u="urn:schemas-upnp-org:service:RenderingControl:1">
      <InstanceID>0</InstanceID>
      <Channel>Master</Channel>
    </u:GetMute>`)
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
func (c *Client) SetMute(muted bool) error {
	val := 0
	if muted {
		val = 1
	}
	_, err := c.rendering("SetMute", fmt.Sprintf(
		`<u:SetMute xmlns:u="urn:schemas-upnp-org:service:RenderingControl:1">
      <InstanceID>0</InstanceID>
      <Channel>Master</Channel>
      <DesiredMute>%d</DesiredMute>
    </u:SetMute>`, val))
	return err
}

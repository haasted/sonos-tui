package sonos

import (
	"context"
	"encoding/xml"
	"fmt"
	"strconv"
)

// serviceURNs are the UPnP service XML namespaces used both as SOAPAction
// URNs and as the xmlns:u attribute on every SOAP body in this file.
const (
	avtransportService = "urn:schemas-upnp-org:service:AVTransport:1"
	renderingService   = "urn:schemas-upnp-org:service:RenderingControl:1"
)

// --- AVTransport (playback control) ------------------------------------------

const avtransportPath = "/MediaRenderer/AVTransport/Control"

func (c *SonosClient) avTransport(ctx context.Context, action string, payload any) ([]byte, error) {
	return c.Do(ctx, avtransportPath, avtransportService, action, payload)
}

// Play starts or resumes playback.
func (c *SonosClient) Play(ctx context.Context) error {
	_, err := c.avTransport(ctx, "Play", playAction{
		XMLName:   xml.Name{Local: "u:Play"},
		Namespace: avtransportService,
		Speed:     1,
	})
	return err
}

// Pause pauses playback.
func (c *SonosClient) Pause(ctx context.Context) error {
	_, err := c.avTransport(ctx, "Pause", instancePayload(avtransportService, "Pause"))
	return err
}

// Stop stops playback.
func (c *SonosClient) Stop(ctx context.Context) error {
	_, err := c.avTransport(ctx, "Stop", instancePayload(avtransportService, "Stop"))
	return err
}

// Next skips to the next track.
func (c *SonosClient) Next(ctx context.Context) error {
	_, err := c.avTransport(ctx, "Next", instancePayload(avtransportService, "Next"))
	return err
}

// Previous goes back to the previous track.
func (c *SonosClient) Previous(ctx context.Context) error {
	_, err := c.avTransport(ctx, "Previous", instancePayload(avtransportService, "Previous"))
	return err
}

// seekUnitTrackNr addresses Seek's Target as a 1-based position in the
// current queue, rather than a time offset or an absolute track URI.
const seekUnitTrackNr = "TRACK_NR"

// SeekTrack moves playback to the track at position within the queue.
func (c *SonosClient) SeekTrack(ctx context.Context, position int) error {
	if position < 1 {
		return fmt.Errorf("seeking in the queue: position %d is not a track", position)
	}
	_, err := c.avTransport(ctx, "Seek", seekAction{
		XMLName:   xml.Name{Local: "u:Seek"},
		Namespace: avtransportService,
		Unit:      seekUnitTrackNr,
		Target:    strconv.Itoa(position),
	})
	return err
}

// transportPlaying is the one TransportState that counts as playing.
const transportPlaying = "PLAYING"

// TransportInfo holds the playback state returned by GetTransportInfo.
type TransportInfo struct {
	State  string // "PLAYING", "PAUSED_PLAYBACK", "STOPPED"
	Status string // "OK"
	Speed  string // "1"
}

// GetTransportInfo returns the current playback state of the speaker.
func (c *SonosClient) GetTransportInfo(ctx context.Context) (*TransportInfo, error) {
	data, err := c.avTransport(ctx, "GetTransportInfo", instancePayload(avtransportService, "GetTransportInfo"))
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
func (c *SonosClient) GetPositionInfo(ctx context.Context) (*TrackInfo, error) {
	data, err := c.avTransport(ctx, "GetPositionInfo", instancePayload(avtransportService, "GetPositionInfo"))
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

func (c *SonosClient) rendering(ctx context.Context, action string, payload any) ([]byte, error) {
	return c.Do(ctx, renderingPath, renderingService, action, payload)
}

// GetVolume returns the current Master volume (0–100).
func (c *SonosClient) GetVolume(ctx context.Context) (int, error) {
	data, err := c.rendering(ctx, "GetVolume", channelAction{
		XMLName:   xml.Name{Local: "u:GetVolume"},
		Namespace: renderingService,
		Channel:   masterChannel,
	})
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
func (c *SonosClient) SetVolume(ctx context.Context, level int) error {
	if level < 0 || level > 100 {
		return fmt.Errorf("volume must be between 0 and 100, got %d", level)
	}
	_, err := c.rendering(ctx, "SetVolume", setVolumeAction{
		XMLName:   xml.Name{Local: "u:SetVolume"},
		Namespace: renderingService,
		Channel:   masterChannel,
		Desired:   level,
	})
	return err
}

func (c *SonosClient) SetRelativeVolume(ctx context.Context, adjustment int) (int, error) {
	data, err := c.rendering(ctx, "SetRelativeVolume", setRelativeVolumeAction{
		XMLName:    xml.Name{Local: "u:SetRelativeVolume"},
		Namespace:  renderingService,
		Channel:    masterChannel,
		Adjustment: adjustment,
	})
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
func (c *SonosClient) GetMute(ctx context.Context) (bool, error) {
	data, err := c.rendering(ctx, "GetMute", channelAction{
		XMLName:   xml.Name{Local: "u:GetMute"},
		Namespace: renderingService,
		Channel:   masterChannel,
	})
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
func (c *SonosClient) SetMute(ctx context.Context, muted bool) error {
	val := 0
	if muted {
		val = 1
	}
	_, err := c.rendering(ctx, "SetMute", setMuteAction{
		XMLName:   xml.Name{Local: "u:SetMute"},
		Namespace: renderingService,
		Channel:   masterChannel,
		Desired:   val,
	})
	return err
}

// --- GroupRenderingControl (the group as one speaker) ------------------------
//
// Volume stays per-speaker, deliberately: each room is as loud as its own
// listener wants. Mute does not, because a muted room is one that has been
// told to be quiet, and that is an instruction about the music, not the room.

const groupRenderingService = "urn:schemas-upnp-org:service:GroupRenderingControl:1"
const groupRenderingPath = "/MediaRenderer/GroupRenderingControl/Control"

func (c *SonosClient) groupRendering(ctx context.Context, action string, payload any) ([]byte, error) {
	return c.Do(ctx, groupRenderingPath, groupRenderingService, action, payload)
}

// GetGroupMute reports whether the group is muted. A speaker on its own is a
// group of one, so this answers for it too.
func (c *SonosClient) GetGroupMute(ctx context.Context) (bool, error) {
	data, err := c.groupRendering(ctx, "GetGroupMute",
		instancePayload(groupRenderingService, "GetGroupMute"))
	if err != nil {
		return false, err
	}

	var resp struct {
		Mute int `xml:"Body>GetGroupMuteResponse>CurrentMute"`
	}
	if err := xml.Unmarshal(data, &resp); err != nil {
		return false, fmt.Errorf("parsing GetGroupMute response: %w", err)
	}
	return resp.Mute == 1, nil
}

// SetGroupMute mutes or unmutes every speaker in the group at once.
func (c *SonosClient) SetGroupMute(ctx context.Context, muted bool) error {
	val := 0
	if muted {
		val = 1
	}
	_, err := c.groupRendering(ctx, "SetGroupMute", setGroupMuteAction{
		XMLName:   xml.Name{Local: "u:SetGroupMute"},
		Namespace: groupRenderingService,
		Desired:   val,
	})
	return err
}

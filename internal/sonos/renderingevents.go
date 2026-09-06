package sonos

import (
	"encoding/xml"
	"fmt"
	"html"
	"log/slog"
)

// masterChannel is the mixed output the UI cares about. RenderingControl also
// reports per-driver channels (LF, RF, ...) whose values differ from it.
const masterChannel = "Master"

// RenderingControlEvent is the volume and mute state carried by a
// RenderingControl LastChange, narrowed to the Master channel.
type RenderingControlEvent struct {
	Volume    int
	Muted     bool
	HasVolume bool // false when the event carried no Master volume
	HasMute   bool // false when the event carried no Master mute
}

var _ slog.LogValuer = RenderingControlEvent{}

func (e RenderingControlEvent) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("volume", e.Volume),
		slog.Bool("muted", e.Muted),
	)
}

// channelVal is a state variable reported once per audio channel, e.g.
// <Volume channel="Master" val="32"/>.
type channelVal struct {
	Channel string `xml:"channel,attr"`
	Val     string `xml:"val,attr"`
}

// ParseRenderingControlEvent parses a GENA NOTIFY body from RenderingControl's
// event service. Its LastChange uses a different namespace and shape from
// AVTransport's: Volume and Mute repeat once per channel, so both are filtered
// to Master rather than taking whichever element appears first.
func ParseRenderingControlEvent(body []byte) (RenderingControlEvent, error) {
	var envelope struct {
		Property struct {
			LastChange struct {
				InnerXML string `xml:",innerxml"`
			} `xml:"LastChange"`
		} `xml:"property"`
	}
	if err := xml.Unmarshal(body, &envelope); err != nil {
		return RenderingControlEvent{}, fmt.Errorf("parsing propertyset envelope: %w", err)
	}
	lastChange := envelope.Property.LastChange.InnerXML

	// Same escaped-or-nested ambiguity as AVTransport: try the raw bytes
	// first, then exactly one unescape pass.
	var doc struct {
		Instance struct {
			Volume []channelVal `xml:"Volume"`
			Mute   []channelVal `xml:"Mute"`
		} `xml:"InstanceID"`
	}
	if err := xml.Unmarshal([]byte(lastChange), &doc); err != nil {
		if err := xml.Unmarshal([]byte(html.UnescapeString(lastChange)), &doc); err != nil {
			return RenderingControlEvent{}, fmt.Errorf("parsing LastChange: %w", err)
		}
	}

	var event RenderingControlEvent
	if v, ok := master(doc.Instance.Volume); ok {
		event.Volume, event.HasVolume = atoiOrZero(v), true
	}
	if v, ok := master(doc.Instance.Mute); ok {
		event.Muted, event.HasMute = v == "1", true
	}
	return event, nil
}

// master returns the Master channel's value, ignoring per-driver channels.
func master(vals []channelVal) (string, bool) {
	for _, v := range vals {
		if v.Channel == masterChannel {
			return v.Val, true
		}
	}
	return "", false
}

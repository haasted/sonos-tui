package sonos

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// DeviceDescription is the subset of a speaker's device_description.xml
// (served at Speaker.Location) that's useful today.
//
// Despite the name, <friendlyName> is not actually friendly — Sonos
// populates it with "<ip> - <model> - <RINCON id>" (e.g.
// "192.168.1.50 - Sonos Amp - RINCON_542A1BE7BCE101400"), so it's omitted
// here. RoomName/DisplayName are the fields that hold what a user actually
// named the speaker in the Sonos app (e.g. "Spisestue" / "Amp").
type DeviceDescription struct {
	RoomName    string
	DisplayName string
	ModelName   string
	ModelNumber string
}

var _ slog.LogValuer = DeviceDescription{}

func (dd DeviceDescription) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("room", dd.RoomName),
		slog.String("name", dd.DisplayName),
		slog.String("model", dd.ModelName),
		slog.String("modelNo", dd.ModelNumber),
	)
}

// descriptionHTTPClient is used for the background device_description.xml
// fetches Discover starts for each speaker. A shared http.Client is safe
// for concurrent use across goroutines; its Timeout bounds each fetch, so
// callers don't need to additionally scope the context they pass in.
var descriptionHTTPClient = &http.Client{Timeout: 5 * time.Second}

// FetchDeviceDescription fetches and parses a speaker's device_description.xml.
func FetchDeviceDescription(ctx context.Context, client *http.Client, location string) (DeviceDescription, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return DeviceDescription{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return DeviceDescription{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return DeviceDescription{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return DeviceDescription{}, err
	}

	// Root element is <root>, matched implicitly by xml.Unmarshal — the
	// "device" tag below only needs to name root's child, not "root" again
	// (same lesson as the earlier InstanceID/Event path bugs).
	var doc struct {
		Device struct {
			RoomName    string `xml:"roomName"`
			DisplayName string `xml:"displayName"`
			ModelName   string `xml:"modelName"`
			ModelNumber string `xml:"modelNumber"`
		} `xml:"device"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return DeviceDescription{}, fmt.Errorf("parsing device description: %w", err)
	}

	return DeviceDescription{
		RoomName:    doc.Device.RoomName,
		DisplayName: doc.Device.DisplayName,
		ModelName:   doc.Device.ModelName,
		ModelNumber: doc.Device.ModelNumber,
	}, nil
}

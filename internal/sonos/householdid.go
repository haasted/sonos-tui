package sonos

import (
	"context"
	"encoding/xml"
	"fmt"
)

// DeviceProperties holds what a speaker says about itself, its household
// among it.
const (
	devicePropertiesService = "urn:schemas-upnp-org:service:DeviceProperties:1"
	devicePropertiesPath    = "/DeviceProperties/Control"
)

// GetHouseholdID reads the id of the household the speaker belongs to, such
// as "Sonos_" and 26 characters. The Sonos cloud's id for the same household
// is this one, a dot and more characters.
func (c *SonosClient) GetHouseholdID(ctx context.Context) (string, error) {
	data, err := c.Do(ctx, devicePropertiesPath, devicePropertiesService, "GetHouseholdID",
		householdIDAction{
			XMLName:   xml.Name{Local: "u:GetHouseholdID"},
			Namespace: devicePropertiesService,
		})
	if err != nil {
		return "", fmt.Errorf("reading the household id: %w", err)
	}

	var envelope struct {
		Body struct {
			Response struct {
				ID string `xml:"CurrentHouseholdID"`
			} `xml:"GetHouseholdIDResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return "", fmt.Errorf("parsing the household id: %w", err)
	}
	if envelope.Body.Response.ID == "" {
		return "", fmt.Errorf("the response carried no household id")
	}
	return envelope.Body.Response.ID, nil
}

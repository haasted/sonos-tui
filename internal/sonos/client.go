package sonos

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// SonosClient sends SOAP commands to a single Sonos speaker over the local network.
type SonosClient struct {
	BaseURL    string // e.g. "http://192.168.1.50:1400"
	HTTPClient *http.Client
}

// NewClient creates a Client targeting the given speaker IP.
func NewClient(ip string) *SonosClient {
	return &SonosClient{
		BaseURL: fmt.Sprintf("http://%s:1400", ip),
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Do sends a SOAP request built from payload and returns the response body.
// The payload is a tagged struct rather than a string, so the marshaller
// escapes its values — see soap.go.
func (c *SonosClient) Do(path, serviceURN, action string, payload any) ([]byte, error) {
	body, err := marshalRequest(payload)
	if err != nil {
		return nil, err
	}
	soapAction := fmt.Sprintf(`"%s#%s"`, serviceURN, action)

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", soapAction)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if fault := parseSOAPFault(data); fault != nil {
			return nil, fault
		}
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, data)
	}

	return data, nil
}

type soapFault struct {
	Code   string `xml:"Body>Fault>faultcode"`
	String string `xml:"Body>Fault>faultstring"`
	Detail struct {
		ErrorCode string `xml:"UPnPError>errorCode"`
		ErrorDesc string `xml:"UPnPError>errorDescription"`
	} `xml:"Body>Fault>detail"`
}

// UPnPTransitionNotAvailable is the code a speaker answers with when an action
// does not apply to what is loaded, such as a previous track on a station.
const UPnPTransitionNotAvailable = 701

// UPnPError is the decoded contents of a SOAP fault. Sonos reports refused
// actions as HTTP 500 with the reason in the body, so callers wanting to
// react to a specific failure should match on Code with errors.As.
type UPnPError struct {
	Code        int
	Description string
	Fault       string
}

func (e *UPnPError) Error() string {
	if e.Description == "" {
		return fmt.Sprintf("UPnP error %d", e.Code)
	}
	return fmt.Sprintf("UPnP error %d: %s", e.Code, e.Description)
}

func parseSOAPFault(data []byte) *UPnPError {
	var fault soapFault
	if err := xml.Unmarshal(data, &fault); err != nil {
		return nil
	}

	code, err := strconv.Atoi(strings.TrimSpace(fault.Detail.ErrorCode))
	if err != nil {
		return nil
	}

	// Speakers routinely send errorCode with no errorDescription, so fall
	// back to the standard text for the code rather than reporting a bare
	// number.
	desc := strings.TrimSpace(fault.Detail.ErrorDesc)
	if desc == "" {
		desc = upnpErrorDescriptions[code]
	}

	return &UPnPError{Code: code, Description: desc, Fault: fault.String}
}

// upnpErrorDescriptions covers the generic UPnP codes plus the AVTransport
// ones, per the UPnP AVTransport:1 service specification.
var upnpErrorDescriptions = map[int]string{
	401: "invalid action",
	402: "invalid args",
	501: "action failed",
	600: "argument value invalid",
	701: "transition not available",
	702: "no contents",
	703: "read error",
	704: "format not supported for playback",
	705: "transport is locked",
	706: "write error",
	707: "media is protected or not writable",
	708: "format not supported for recording",
	709: "media is full",
	710: "seek mode not supported",
	711: "illegal seek target",
	712: "play mode not supported",
	713: "record quality not supported",
	714: "illegal MIME-type",
	715: "content busy",
	716: "resource not found",
	717: "play speed not supported",
	718: "invalid InstanceID",
}

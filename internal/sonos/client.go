package sonos

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
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

// soapEnvelope wraps an action body in a SOAP envelope.
const soapEnvelope = `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"
            s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>%s</s:Body>
</s:Envelope>`

// Do sends a raw SOAP request and returns the response body.
func (c *SonosClient) Do(path, serviceURN, action, body string) ([]byte, error) {
	envelope := fmt.Sprintf(soapEnvelope, body)
	soapAction := fmt.Sprintf(`"%s#%s"`, serviceURN, action)

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, strings.NewReader(envelope))
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
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, data)
	}

	return data, nil
}

// soapFault is used to decode SOAP error responses.
type soapFault struct {
	XMLName xml.Name `xml:"Envelope>Body>Fault"`
	Code    string   `xml:"faultcode"`
	String  string   `xml:"faultstring"`
	Detail  struct {
		ErrorCode string `xml:"UPnPError>errorCode"`
		ErrorDesc string `xml:"UPnPError>errorDescription"`
	} `xml:"detail"`
}

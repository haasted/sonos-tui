package sonos_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	c := sonos.NewClient("192.168.1.50")
	assert.Equal(t, "http://192.168.1.50:1400", c.BaseURL)
	assert.NotNil(t, c.HTTPClient)
}

func TestClient_Do_Success(t *testing.T) {
	// Spin up a local test server that mimics a Sonos speaker response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, `"urn:schemas-upnp-org:service:AVTransport:1#Pause"`, r.Header.Get("SOAPAction"))

		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body><u:PauseResponse xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"/></s:Body>
</s:Envelope>`))
	}))
	defer srv.Close()

	// Point the client at the test server (strip "http://" and ":port" to get the host).
	c := &sonos.SonosClient{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	}

	body := `<u:Pause xmlns:u="urn:schemas-upnp-org:service:AVTransport:1"><InstanceID>0</InstanceID></u:Pause>`
	data, err := c.Do(
		"/MediaRenderer/AVTransport/Control",
		"urn:schemas-upnp-org:service:AVTransport:1",
		"Pause",
		body,
	)

	require.NoError(t, err)
	assert.Contains(t, string(data), "PauseResponse")
}

func faultServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

const upnpFault = `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <s:Fault>
      <faultcode>s:Client</faultcode>
      <faultstring>UPnPError</faultstring>
      <detail>
        <UPnPError xmlns="urn:schemas-upnp-org:control-1-0">
          <errorCode>%s</errorCode>
        </UPnPError>
      </detail>
    </s:Fault>
  </s:Body>
</s:Envelope>`

func TestClient_Do_DecodesUPnPFault(t *testing.T) {
	srv := faultServer(t, fmt.Sprintf(upnpFault, "701"))

	c := &sonos.SonosClient{BaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := c.Do("/MediaRenderer/AVTransport/Control", "urn:fake", "Previous", "<body/>")

	require.Error(t, err)

	var upnpErr *sonos.UPnPError
	require.ErrorAs(t, err, &upnpErr, "a SOAP fault should surface as a typed UPnPError, not a raw status dump")
	assert.Equal(t, 701, upnpErr.Code)
	assert.Equal(t, "transition not available", upnpErr.Description,
		"the standard description should fill in when the speaker sends only a code")
	assert.NotContains(t, err.Error(), "<s:Envelope", "the raw XML body should not leak into the message")
}

func TestClient_Do_UsesSpeakerSuppliedDescription(t *testing.T) {
	body := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body><s:Fault>
    <faultcode>s:Client</faultcode>
    <faultstring>UPnPError</faultstring>
    <detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0">
      <errorCode>402</errorCode>
      <errorDescription>Something specific went wrong</errorDescription>
    </UPnPError></detail>
  </s:Fault></s:Body>
</s:Envelope>`
	srv := faultServer(t, body)

	c := &sonos.SonosClient{BaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := c.Do("/path", "urn:fake", "Play", "<body/>")

	var upnpErr *sonos.UPnPError
	require.ErrorAs(t, err, &upnpErr)
	assert.Equal(t, 402, upnpErr.Code)
	assert.Equal(t, "Something specific went wrong", upnpErr.Description)
}

func TestClient_Do_UnknownErrorCodeStillReported(t *testing.T) {
	srv := faultServer(t, fmt.Sprintf(upnpFault, "9999"))

	c := &sonos.SonosClient{BaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := c.Do("/path", "urn:fake", "Play", "<body/>")

	var upnpErr *sonos.UPnPError
	require.ErrorAs(t, err, &upnpErr)
	assert.Equal(t, 9999, upnpErr.Code)
	assert.Contains(t, err.Error(), "9999", "an unmapped code should still name itself")
}

func TestClient_Do_NonFaultErrorKeepsRawBody(t *testing.T) {
	srv := faultServer(t, "plain gateway failure")

	c := &sonos.SonosClient{BaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := c.Do("/path", "urn:fake", "Play", "<body/>")

	require.Error(t, err)
	var upnpErr *sonos.UPnPError
	assert.NotErrorAs(t, err, &upnpErr)
	assert.Contains(t, err.Error(), "500")
}

func TestClient_Do_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := &sonos.SonosClient{BaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := c.Do("/some/path", "urn:fake:service:1", "FakeAction", "<body/>")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

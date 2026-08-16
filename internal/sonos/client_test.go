package sonos_test

import (
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
	c := &sonos.Client{
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

func TestClient_Do_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := &sonos.Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := c.Do("/some/path", "urn:fake:service:1", "FakeAction", "<body/>")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

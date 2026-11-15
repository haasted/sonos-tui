package sonos_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a speaker's answer, as captured from a real one. The id is read
// from DeviceProperties, not from /MediaRenderer.
func TestHouseholdID_ReadsItFromASpeaker(t *testing.T) {
	var path, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		path, body = r.URL.Path, string(raw)
		io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">`+
			`<s:Body><u:GetHouseholdIDResponse xmlns:u="urn:schemas-upnp-org:service:DeviceProperties:1">`+
			`<CurrentHouseholdID>Sonos_abcdefghijklmnopqrstuvwxyz</CurrentHouseholdID>`+
			`</u:GetHouseholdIDResponse></s:Body></s:Envelope>`)
	}))
	defer server.Close()

	id, err := clientFor(server).GetHouseholdID(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "Sonos_abcdefghijklmnopqrstuvwxyz", id)
	assert.Equal(t, "/DeviceProperties/Control", path)
	assert.Contains(t, body, "u:GetHouseholdID")
}

// Case: a speaker that answers with no id. Better an error than an empty
// id that matches no household.
func TestHouseholdID_RefusesAnEmptyAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
			`<s:Body><u:GetHouseholdIDResponse/></s:Body></s:Envelope>`)
	}))
	defer server.Close()

	_, err := clientFor(server).GetHouseholdID(context.Background())

	assert.Error(t, err)
}

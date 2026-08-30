package sonos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturedRequest records what a fake speaker saw, so tests can assert on
// the GENA headers rather than just the happy-path return value.
type capturedRequest struct {
	method   string
	path     string
	callback string
	nt       string
	sid      string
	timeout  string
}

func eventServer(t *testing.T, sid string, status int) (*httptest.Server, *capturedRequest) {
	t.Helper()

	var got capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = capturedRequest{
			method:   r.Method,
			path:     r.URL.Path,
			callback: r.Header.Get("CALLBACK"),
			nt:       r.Header.Get("NT"),
			sid:      r.Header.Get("SID"),
			timeout:  r.Header.Get("TIMEOUT"),
		}
		if sid != "" {
			w.Header().Set("SID", sid)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	return srv, &got
}

func TestSubscribe_SendsGENAHeadersAndReturnsSID(t *testing.T) {
	srv, got := eventServer(t, "uuid:sub-1", http.StatusOK)

	sid, err := subscribe(context.Background(), srv.Client(), srv.URL, avTransportEventPath, "http://192.168.1.2:9999/notify/abc")

	require.NoError(t, err)
	assert.Equal(t, "uuid:sub-1", sid)
	assert.Equal(t, "SUBSCRIBE", got.method)
	assert.Equal(t, avTransportEventPath, got.path)
	assert.Equal(t, "<http://192.168.1.2:9999/notify/abc>", got.callback, "CALLBACK must be angle-bracketed per UPnP")
	assert.Equal(t, "upnp:event", got.nt)
	assert.Equal(t, "Second-300", got.timeout)
}

func TestSubscribe_MissingSIDIsAnError(t *testing.T) {
	srv, _ := eventServer(t, "", http.StatusOK)

	_, err := subscribe(context.Background(), srv.Client(), srv.URL, avTransportEventPath, "http://callback/notify/abc")

	require.Error(t, err, "a 200 with no SID leaves nothing to renew or unsubscribe with")
}

func TestSubscribe_NonOKStatusIsAnError(t *testing.T) {
	srv, _ := eventServer(t, "uuid:sub-1", http.StatusServiceUnavailable)

	_, err := subscribe(context.Background(), srv.Client(), srv.URL, avTransportEventPath, "http://callback/notify/abc")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

func TestRenew_IdentifiesSubscriptionBySID(t *testing.T) {
	srv, got := eventServer(t, "", http.StatusOK)

	err := renew(context.Background(), srv.Client(), srv.URL, avTransportEventPath, "uuid:sub-1")

	require.NoError(t, err)
	assert.Equal(t, "SUBSCRIBE", got.method, "renewal is a SUBSCRIBE carrying an SID, not a distinct verb")
	assert.Equal(t, "uuid:sub-1", got.sid)
	assert.Equal(t, "Second-300", got.timeout)
	assert.Empty(t, got.callback, "renewal must not resend CALLBACK — that would start a second subscription")
}

func TestUnsubscribe_SendsSIDAndNoTimeout(t *testing.T) {
	srv, got := eventServer(t, "", http.StatusOK)

	err := unsubscribe(context.Background(), srv.Client(), srv.URL, avTransportEventPath, "uuid:sub-1")

	require.NoError(t, err)
	assert.Equal(t, "UNSUBSCRIBE", got.method)
	assert.Equal(t, "uuid:sub-1", got.sid)
	assert.Empty(t, got.timeout)
}

func TestUnsubscribe_NonOKStatusIsAnError(t *testing.T) {
	srv, _ := eventServer(t, "", http.StatusPreconditionFailed)

	err := unsubscribe(context.Background(), srv.Client(), srv.URL, avTransportEventPath, "uuid:gone")

	require.Error(t, err)
}

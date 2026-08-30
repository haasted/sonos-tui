package sonos

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSpeaker struct {
	server *httptest.Server

	mu           sync.Mutex
	callbackURL  string
	unsubscribed bool
}

func newFakeSpeaker(t *testing.T) *fakeSpeaker {
	t.Helper()

	fs := &fakeSpeaker{}
	fs.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		defer fs.mu.Unlock()

		switch r.Method {
		case "SUBSCRIBE":
			if cb := r.Header.Get("CALLBACK"); cb != "" {
				fs.callbackURL = strings.Trim(cb, "<>")
			}
			w.Header().Set("SID", "uuid:fake-sub")
		case "UNSUBSCRIBE":
			fs.unsubscribed = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(fs.server.Close)

	return fs
}

func (fs *fakeSpeaker) callback() string {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.callbackURL
}

func (fs *fakeSpeaker) sentUnsubscribe() bool {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.unsubscribed
}

func testDevice(fs *fakeSpeaker) *Device {
	speaker := Speaker{IP: "test-speaker", USN: "uuid:RINCON_TEST::urn:test"}
	return NewDeviceWithClient(speaker, &SonosClient{BaseURL: fs.server.URL, HTTPClient: fs.server.Client()})
}

func TestEventManager_DeliversNotifyToSubscribedDevice(t *testing.T) {
	fs := newFakeSpeaker(t)
	device := testDevice(fs)

	em, err := NewEventManager()
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		em.Close(ctx)
	})

	require.NoError(t, em.Subscribe(context.Background(), device))

	callback := fs.callback()
	require.NotEmpty(t, callback, "the speaker should have been handed a callback URL to post events to")

	body := `<?xml version="1.0"?>
<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0">
  <e:property><LastChange>&lt;Event xmlns="urn:schemas-upnp-org:metadata-1-0/AVT/"&gt;&lt;InstanceID val="0"&gt;&lt;TransportState val="PLAYING"/&gt;&lt;RelativeTimePosition val="0:00:42"/&gt;&lt;/InstanceID&gt;&lt;/Event&gt;</LastChange></e:property>
</e:propertyset>`

	req, err := http.NewRequest("NOTIFY", callback, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("SID", "uuid:fake-sub")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	select {
	case event := <-device.Events():
		assert.Equal(t, "PLAYING", event.Transport.State)
		assert.Equal(t, "uuid:RINCON_TEST::urn:test", event.Speaker.USN, "the event should identify which speaker it came from")
	case <-time.After(3 * time.Second):
		t.Fatal("no event delivered from the NOTIFY")
	}

	assert.Equal(t, "0:00:42", device.State().RelTime)
}

func TestEventManager_NotifyToUnknownPathIsAcknowledged(t *testing.T) {
	em, err := NewEventManager()
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		em.Close(ctx)
	})

	req, err := http.NewRequest("NOTIFY", em.listener.baseURL+notifyPrefix+"nosuchdevice", strings.NewReader("<propertyset/>"))
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestEventManager_CloseUnsubscribes(t *testing.T) {
	fs := newFakeSpeaker(t)
	device := testDevice(fs)

	em, err := NewEventManager()
	require.NoError(t, err)
	require.NoError(t, em.Subscribe(context.Background(), device))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, em.Close(ctx))

	assert.True(t, fs.sentUnsubscribe(), "Close must unsubscribe so the speaker stops posting to a dead callback URL")
}

func TestEventManager_SubscribeFailurePropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	em, err := NewEventManager()
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		em.Close(ctx)
	})

	device := NewDeviceWithClient(
		Speaker{IP: "unreachable", USN: "uuid:unreachable"},
		&SonosClient{BaseURL: srv.URL, HTTPClient: srv.Client()},
	)

	require.Error(t, em.Subscribe(context.Background(), device))

	em.listener.mu.RLock()
	routes := len(em.listener.routes)
	em.listener.mu.RUnlock()
	assert.Zero(t, routes)
}

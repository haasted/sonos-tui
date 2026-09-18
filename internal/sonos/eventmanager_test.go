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

	mu            sync.Mutex
	callbacks     map[string]string // event path -> callback URL handed to us
	unsubscribed  bool
	unsubscribeds int
}

func newFakeSpeaker(t *testing.T) *fakeSpeaker {
	t.Helper()

	fs := &fakeSpeaker{callbacks: map[string]string{}}
	fs.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		defer fs.mu.Unlock()

		switch r.Method {
		case "SUBSCRIBE":
			if cb := r.Header.Get("CALLBACK"); cb != "" {
				fs.callbacks[r.URL.Path] = strings.Trim(cb, "<>")
			}
			w.Header().Set("SID", "uuid:fake-sub")
		case "UNSUBSCRIBE":
			fs.unsubscribed = true
			fs.unsubscribeds++
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(fs.server.Close)

	return fs
}

// callback returns the URL handed to the speaker for one event service.
func (fs *fakeSpeaker) callback(eventPath string) string {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.callbacks[eventPath]
}

// unsubscribeCount reports how many UNSUBSCRIBEs arrived, one per service.
func (fs *fakeSpeaker) unsubscribeCount() int {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.unsubscribeds
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

	callback := fs.callback(avTransportEventPath)
	require.NotEmpty(t, callback, "the speaker should have been handed a callback URL to post events to")

	require.NotEqual(t, callback, fs.callback(renderingControlEventPath),
		"each service needs its own callback path, or NOTIFYs cannot be told apart")

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

// Case: a volume change arriving from another control point, which must land
// in Device.State() without any command from this program.
func TestEventManager_DeliversRenderingControlToSubscribedDevice(t *testing.T) {
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

	callback := fs.callback(renderingControlEventPath)
	require.NotEmpty(t, callback)

	body := `<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0"><e:property><LastChange>` +
		`&lt;Event xmlns=&quot;urn:schemas-upnp-org:metadata-1-0/RCS/&quot;&gt;&lt;InstanceID val=&quot;0&quot;&gt;` +
		`&lt;Volume channel=&quot;Master&quot; val=&quot;27&quot;/&gt;` +
		`&lt;Volume channel=&quot;LF&quot; val=&quot;100&quot;/&gt;` +
		`&lt;Mute channel=&quot;Master&quot; val=&quot;1&quot;/&gt;` +
		`&lt;/InstanceID&gt;&lt;/Event&gt;</LastChange></e:property></e:propertyset>`

	req, err := http.NewRequest("NOTIFY", callback, strings.NewReader(body))
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	select {
	case <-device.Events():
	case <-time.After(2 * time.Second):
		t.Fatal("no event published for a RenderingControl NOTIFY")
	}

	assert.Equal(t, 27, device.State().Volume, "must take Master, not the LF channel's 100")
	assert.True(t, device.State().Muted)
}

// Case: shutdown with two subscriptions per device, which must both be
// released rather than leaving one dangling on the speaker.
func TestEventManager_CloseUnsubscribesEveryService(t *testing.T) {
	fs := newFakeSpeaker(t)
	device := testDevice(fs)

	em, err := NewEventManager()
	require.NoError(t, err)
	require.NoError(t, em.Subscribe(context.Background(), device))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, em.Close(ctx))

	assert.Equal(t, len(subscribedServices), fs.unsubscribeCount(),
		"every subscribed service should be unsubscribed on shutdown")
}

// Case: a ContentDirectory NOTIFY, which is the first subscription outside
// /MediaRenderer. Its document has no LastChange, so a body routed to either
// of the other two parsers reads as an event that never happened.
func TestEventManager_DeliversContentDirectoryToSubscribedDevice(t *testing.T) {
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

	callback := fs.callback(contentDirectoryEventPath)
	require.NotEmpty(t, callback, "the device must be subscribed to ContentDirectory")

	body := `<e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0">` +
		`<e:property><SystemUpdateID>118</SystemUpdateID></e:property>` +
		`<e:property><ContainerUpdateIDs>Q:0,42</ContainerUpdateIDs></e:property>` +
		`</e:propertyset>`

	req, err := http.NewRequest("NOTIFY", callback, strings.NewReader(body))
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	select {
	case event := <-device.Events():
		assert.Equal(t, EventContentDirectory, event.Kind, "the panel must be able to tell this from a track change")
		updateID, named := event.Directory.QueueUpdate()
		assert.True(t, named)
		assert.Equal(t, 42, updateID)
	case <-time.After(2 * time.Second):
		t.Fatal("no event published for a ContentDirectory NOTIFY")
	}

	assert.Equal(t, 42, device.State().QueueUpdateID)
}

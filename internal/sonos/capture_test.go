package sonos_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a body this package cannot parse, which must still be captured. Those
// are the valuable ones — they are exactly the schema we do not handle yet, so
// capturing after parsing would lose precisely what a capture is for.
func TestCapture_UnparseableBodyIsStillCaptured(t *testing.T) {
	captured := newCaptureRecorder()
	manager, err := sonos.NewEventManager(sonos.WithCapture(captured.record))
	require.NoError(t, err)
	t.Cleanup(func() { manager.Close(context.Background()) })

	postNotify(t, manager, "/notify/deadbeef", "this is not xml at all")

	bodies := captured.bodies()
	require.Len(t, bodies, 1)
	assert.Equal(t, "this is not xml at all", bodies[0])
}

// Case: a NOTIFY for a path nothing subscribed to. Being sent something we did
// not ask for is itself worth the evidence, so it must be captured rather than
// dropped with a debug line.
func TestCapture_UnroutedNotifyIsCaptured(t *testing.T) {
	captured := newCaptureRecorder()
	manager, err := sonos.NewEventManager(sonos.WithCapture(captured.record))
	require.NoError(t, err)
	t.Cleanup(func() { manager.Close(context.Background()) })

	postNotify(t, manager, "/notify/nothing-here", "<propertyset/>")

	events := captured.events()
	require.Len(t, events, 1)
	assert.Empty(t, events[0].Speaker.IP, "an unrouted event has no speaker")
	assert.Empty(t, events[0].Service)
	assert.Equal(t, "/notify/nothing-here", events[0].Path)
}

// Case: no capture configured, which must leave event handling untouched.
func TestCapture_AbsentHookChangesNothing(t *testing.T) {
	manager, err := sonos.NewEventManager()
	require.NoError(t, err)
	t.Cleanup(func() { manager.Close(context.Background()) })

	postNotify(t, manager, "/notify/whatever", "<propertyset/>")
}

// captureRecorder collects what the hook is handed. The listener serves on its
// own goroutine, so access has to be guarded.
type captureRecorder struct {
	mu   sync.Mutex
	seen []sonos.RawEvent
}

func newCaptureRecorder() *captureRecorder { return &captureRecorder{} }

func (c *captureRecorder) record(raw sonos.RawEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen = append(c.seen, raw)
}

func (c *captureRecorder) events() []sonos.RawEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]sonos.RawEvent(nil), c.seen...)
}

func (c *captureRecorder) bodies() []string {
	var out []string
	for _, raw := range c.events() {
		out = append(out, string(raw.Body))
	}
	return out
}

func postNotify(t *testing.T, manager *sonos.EventManager, path, body string) {
	t.Helper()

	request, err := http.NewRequest("NOTIFY", manager.CallbackBaseURL()+path, strings.NewReader(body))
	require.NoError(t, err)

	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
}

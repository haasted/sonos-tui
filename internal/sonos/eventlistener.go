package sonos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
)

// notifyPrefix is the path prefix every per-device callback URL sits
// under. Registered on the mux as a subtree so each device can have its
// own path below it.
const notifyPrefix = "/notify/"

// eventListener is the single HTTP server the whole process shares for
// inbound GENA NOTIFYs. Speakers call back into it, so this is the one
// place the program acts as a server rather than a client.
//
// NOTIFYs carry an SID header identifying the subscription, but routing on
// a per-device path instead means a renewal that changes the SID, or a
// stray NOTIFY from a subscription we already dropped, can't be
// misattributed to the wrong speaker.
type eventListener struct {
	server  *http.Server
	baseURL string

	mu     sync.RWMutex
	routes map[string]*Device
}

func newEventListener() (*eventListener, error) {
	localIP, err := GetLocalIP()
	if err != nil {
		return nil, fmt.Errorf("detecting local IP: %w", err)
	}

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, fmt.Errorf("opening callback listener: %w", err)
	}

	el := &eventListener{
		baseURL: fmt.Sprintf("http://%s:%d", localIP, ln.Addr().(*net.TCPAddr).Port),
		routes:  map[string]*Device{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc(notifyPrefix, el.handleNotify)
	el.server = &http.Server{Handler: mux}

	go func() {
		if err := el.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("callback listener stopped", "err", err)
		}
	}()
	slog.Info("callback listener ready", "url", el.baseURL)

	return el, nil
}

// register routes NOTIFYs for device to it, returning the callback URL the
// speaker should be told to post to. Re-registering the same device
// reuses its path rather than leaking a second route.
func (el *eventListener) register(device *Device) string {
	path := notifyPath(device)

	el.mu.Lock()
	el.routes[path] = device
	el.mu.Unlock()

	return el.baseURL + path
}

func (el *eventListener) unregister(device *Device) {
	el.mu.Lock()
	delete(el.routes, notifyPath(device))
	el.mu.Unlock()
}

func (el *eventListener) handleNotify(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading body", http.StatusBadRequest)
		return
	}

	el.mu.RLock()
	device := el.routes[r.URL.Path]
	el.mu.RUnlock()

	// A speaker whose subscription we already dropped can still have a
	// NOTIFY in flight. Acknowledge it rather than erroring, so the
	// speaker doesn't treat us as broken.
	if device == nil {
		slog.Debug("NOTIFY for unknown subscription", "path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := device.HandleEvent(body); err != nil {
		slog.Warn("handling event", "speaker", device.Speaker, "err", err)
	}

	w.WriteHeader(http.StatusOK)
}

func (el *eventListener) close(ctx context.Context) error {
	return el.server.Shutdown(ctx)
}

// notifyPath derives a device's callback path from its USN, hashed to keep
// the URL short and path-safe (a raw USN is a long "uuid:RINCON_...::urn:..."
// string). Deterministic, so re-registering a device is idempotent.
func notifyPath(device *Device) string {
	key := device.USN
	if key == "" {
		key = device.IP
	}
	sum := sha256.Sum256([]byte(key))
	return notifyPrefix + hex.EncodeToString(sum[:])[:12]
}

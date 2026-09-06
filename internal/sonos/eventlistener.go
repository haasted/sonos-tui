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

const notifyPrefix = "/notify/"

type eventListener struct {
	server  *http.Server
	baseURL string

	mu     sync.RWMutex
	routes map[string]route
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
		routes:  map[string]route{},
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

// route is one subscription's destination: which device, and which of its
// services the NOTIFY came from.
type route struct {
	device  *Device
	service string
}

func (el *eventListener) register(device *Device, service string) string {
	path := notifyPath(device, service)

	el.mu.Lock()
	el.routes[path] = route{device: device, service: service}
	el.mu.Unlock()

	return el.baseURL + path
}

func (el *eventListener) unregister(device *Device, service string) {
	el.mu.Lock()
	delete(el.routes, notifyPath(device, service))
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
	found := el.routes[r.URL.Path]
	el.mu.RUnlock()

	if found.device == nil {
		slog.Debug("NOTIFY for unknown subscription", "path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := found.handle(body); err != nil {
		slog.Warn("handling event", "speaker", found.device.Speaker, "service", found.service, "err", err)
	}

	w.WriteHeader(http.StatusOK)
}

func (el *eventListener) close(ctx context.Context) error {
	return el.server.Shutdown(ctx)
}

// handle dispatches the body to the parser for this route's service.
func (r route) handle(body []byte) error {
	if r.service == renderingControlEventPath {
		return r.device.HandleRenderingControlEvent(body)
	}
	return r.device.HandleAVTransportEvent(body)
}

func notifyPath(device *Device, service string) string {
	key := device.USN
	if key == "" {
		key = device.IP
	}
	sum := sha256.Sum256([]byte(key + "|" + service))
	return notifyPrefix + hex.EncodeToString(sum[:])[:12]
}

// Command listening is a prototype for ADR-0002: discover speakers,
// subscribe to their AVTransport events via GENA, and print every event
// received — for manually driving a speaker and watching what arrives.
// Deliberately simple: no interactive input, one file, raw NOTIFY bodies
// logged as-is rather than fully parsed (see the ADR's open questions).
package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/lmittmann/tint"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

const (
	avTransportEventPath = "/MediaRenderer/AVTransport/Event"
	notifyPath           = "/notify"

	discoverTimeout     = 4 * time.Second
	subscriptionTimeout = 5 * time.Minute
	renewInterval       = 4 * time.Minute // comfortably inside subscriptionTimeout
)

// subscription is one active GENA subscription, for logging and for
// unsubscribing cleanly on shutdown.
type subscription struct {
	speaker sonos.Speaker
	sid     string
}

// subscriptions is the set of currently active subscriptions, keyed by SID
// (which is how an incoming NOTIFY identifies which subscription it's
// for — no per-device callback path needed). Safe for concurrent use by
// the discovery goroutine, the renewal loop, and the NOTIFY handler.
type subscriptions struct {
	mu   sync.Mutex
	byID map[string]subscription
}

func (s *subscriptions) add(sub subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[sub.sid] = sub
}

func (s *subscriptions) all() []subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]subscription, 0, len(s.byID))
	for _, sub := range s.byID {
		out = append(out, sub)
	}
	return out
}

func (s *subscriptions) lookup(sid string) (subscription, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.byID[sid]
	return sub, ok
}

// handleNotify logs every NOTIFY body it receives, unescaped for readability
// (LastChange is HTML-entity-escaped XML, same as the DIDL-Lite metadata
// GetPositionInfo already unescapes). No structural parsing yet — that's
// this prototype's whole point: see what real events actually look like
// before designing a parser for them.
func (s *subscriptions) handleNotify(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	sid := r.Header.Get("SID")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "reading body", http.StatusBadRequest)
		return
	}

	speaker := "unknown"
	if sub, ok := s.lookup(sid); ok {
		speaker = sub.speaker.IP
	}
	slog.Info("event received", "speaker", speaker, "sid", sid, "body", html.UnescapeString(string(body)))

	w.WriteHeader(http.StatusOK)
}

func (s *subscriptions) unsubscribeAll(ctx context.Context, client *http.Client) {
	for _, sub := range s.all() {
		if err := unsubscribe(ctx, client, sub.speaker, sub.sid); err != nil {
			slog.Warn("unsubscribe failed", "speaker", sub.speaker.IP, "err", err)
			continue
		}
		slog.Info("unsubscribed", "speaker", sub.speaker.IP)
	}
}

func main() {
	slog.SetDefault(slog.New(tint.NewTextHandler(os.Stdout, &tint.Options{TimeFormat: time.TimeOnly})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		slog.Error("opening callback listener", "err", err)
		os.Exit(1)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	localIP, err := sonos.GetLocalIP()
	if err != nil {
		slog.Error("detecting local IP", "err", err)
		os.Exit(1)
	}
	callbackURL := fmt.Sprintf("http://%s:%d%s", localIP, port, notifyPath)

	subs := &subscriptions{byID: map[string]subscription{}}

	mux := http.NewServeMux()
	mux.HandleFunc(notifyPath, subs.handleNotify)
	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("callback listener stopped", "err", err)
		}
	}()
	slog.Info("callback listener ready", "url", callbackURL)

	client := &http.Client{Timeout: 5 * time.Second}

	go discoverAndSubscribe(ctx, client, callbackURL, subs)
	go renewLoop(ctx, client, subs)

	<-ctx.Done()
	slog.Info("shutting down — unsubscribing from every speaker")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	subs.unsubscribeAll(shutdownCtx, client)
	srv.Shutdown(shutdownCtx)
}

// discoverAndSubscribe finds speakers and subscribes to each as it's found,
// rather than waiting for the full discovery window to finish.
func discoverAndSubscribe(ctx context.Context, client *http.Client, callbackURL string, subs *subscriptions) {
	discoverCtx, cancel := context.WithTimeout(ctx, discoverTimeout)
	defer cancel()

	found, err := sonos.Discover(discoverCtx)
	if err != nil {
		slog.Error("discovery failed", "err", err)
		return
	}

	for speaker := range found {
		slog.Info("speaker found", "ip", speaker.IP, "usn", speaker.USN)

		sid, err := subscribe(ctx, client, speaker, callbackURL)
		if err != nil {
			slog.Warn("subscribe failed", "speaker", speaker.IP, "err", err)
			continue
		}
		subs.add(subscription{speaker: speaker, sid: sid})
		slog.Info("subscribed", "speaker", speaker.IP, "sid", sid)
	}
}

// renewLoop periodically re-subscribes every active subscription so events
// keep flowing past the initial TIMEOUT.
func renewLoop(ctx context.Context, client *http.Client, subs *subscriptions) {
	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, sub := range subs.all() {
				if err := renew(ctx, client, sub.speaker, sub.sid); err != nil {
					slog.Warn("renew failed", "speaker", sub.speaker.IP, "err", err)
					continue
				}
				slog.Debug("renewed", "speaker", sub.speaker.IP, "sid", sub.sid)
			}
		}
	}
}

// subscribe sends a GENA SUBSCRIBE for a speaker's AVTransport events and
// returns the SID the speaker assigns.
func subscribe(ctx context.Context, client *http.Client, speaker sonos.Speaker, callbackURL string) (string, error) {
	req, err := eventRequest(ctx, speaker, "SUBSCRIBE")
	if err != nil {
		return "", err
	}
	req.Header.Set("CALLBACK", "<"+callbackURL+">")
	req.Header.Set("NT", "upnp:event")
	req.Header.Set("TIMEOUT", timeoutHeader())

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return resp.Header.Get("SID"), nil
}

// renew extends an existing subscription before its TIMEOUT expires,
// reusing the same SID rather than starting a new subscription.
func renew(ctx context.Context, client *http.Client, speaker sonos.Speaker, sid string) error {
	req, err := eventRequest(ctx, speaker, "SUBSCRIBE")
	if err != nil {
		return err
	}
	req.Header.Set("SID", sid)
	req.Header.Set("TIMEOUT", timeoutHeader())

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// unsubscribe tells a speaker to stop sending events for sid.
func unsubscribe(ctx context.Context, client *http.Client, speaker sonos.Speaker, sid string) error {
	req, err := eventRequest(ctx, speaker, "UNSUBSCRIBE")
	if err != nil {
		return err
	}
	req.Header.Set("SID", sid)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func eventRequest(ctx context.Context, speaker sonos.Speaker, method string) (*http.Request, error) {
	base := sonos.NewClient(speaker.IP).BaseURL
	return http.NewRequestWithContext(ctx, method, base+avTransportEventPath, nil)
}

func timeoutHeader() string {
	return "Second-" + strconv.Itoa(int(subscriptionTimeout.Seconds()))
}

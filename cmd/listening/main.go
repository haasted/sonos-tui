package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lmittmann/tint"

	"github.com/haasted/sonos-sandbox/internal/log"
	"github.com/haasted/sonos-sandbox/internal/sonos"
)

const (
	avTransportEventPath = "/MediaRenderer/AVTransport/Event"
	notifyPath           = "/notify"
	captureDir           = "captured-data"

	discoverTimeout     = 5 * time.Second
	subscriptionTimeout = 300
	renewInterval       = 4 * time.Minute
)

var captureFlag = flag.Bool("capture", false, "write every raw NOTIFY body to its own file under "+captureDir+"/")

var cacheFileFlag = flag.String("cache-file", "",
	"path to a speaker cache JSON file; if it exists, load speakers from it instead of "+
		"running discovery, and (re)write it once startup completes")

type subscription struct {
	speaker sonos.Speaker
	sid     string
}

type subscriptions struct {
	mu      sync.Mutex
	byID    map[string]subscription
	order   []string
	capture bool

	revealed bool

	firstEventOnce sync.Once
}

func (s *subscriptions) add(sub subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[sub.sid] = sub
	s.order = append(s.order, sub.sid)
}

func (s *subscriptions) addAndAnnounce(sub subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[sub.sid] = sub
	s.order = append(s.order, sub.sid)
	if s.revealed {
		fmt.Printf("  %d  %s\n", len(s.order), sub.speaker.DisplayName())
	}
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

func (s *subscriptions) find(selector string) (subscription, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if n, err := strconv.Atoi(selector); err == nil {
		if n < 1 || n > len(s.order) {
			return subscription{}, false
		}
		return s.byID[s.order[n-1]], true
	}

	for _, sid := range s.order {
		sub := s.byID[sid]
		desc, _ := sub.speaker.Description()
		if strings.EqualFold(desc.RoomName, selector) || strings.EqualFold(desc.DisplayName, selector) {
			return sub, true
		}
	}
	return subscription{}, false
}

func (s *subscriptions) speakers() []sonos.Speaker {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]sonos.Speaker, len(s.order))
	for i, sid := range s.order {
		out[i] = s.byID[sid].speaker
	}
	return out
}

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

	s.firstEventOnce.Do(func() {
		slog.Info("first event received", "speaker", speaker)
	})

	var captured string
	if s.capture {
		path, err := captureEvent(speaker, body)
		if err != nil {
			slog.Warn("failed to capture event", "speaker", speaker, "err", err)
		}
		captured = path
	}

	event, err := sonos.ParseAVTransportEvent(body)
	if err != nil {
		slog.Warn("failed to parse event — logging raw body",
			"speaker", speaker, "sid", sid, "err", err, "body", html.UnescapeString(string(body)), "captured", captured)
		w.WriteHeader(http.StatusOK)
		return
	}

	slog.Info("event received", "speaker", speaker, "event", event, "captured", captured)

	w.WriteHeader(http.StatusOK)
}

func captureEvent(speaker string, body []byte) (string, error) {
	name := time.Now().Format("20060102-150405.000000000") + "-" + speaker + ".xml"
	path := filepath.Join(captureDir, name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (s *subscriptions) unsubscribeAll(ctx context.Context, client *http.Client) {
	for _, sub := range s.all() {
		if err := unsubscribe(ctx, client, sub.speaker, sub.sid); err != nil {
			slog.Warn("unsubscribe failed", "speaker", sub.speaker, "err", err)
			continue
		}

		slog.Info("unsubscribed", "speaker", sub.speaker)
	}
}

func main() {
	start := time.Now()

	slog.SetDefault(slog.New(log.NewElapsedHandler(
		tint.NewTextHandler(os.Stdout, &tint.Options{TimeFormat: time.TimeOnly}),
		start,
	)))
	flag.Parse()

	if *captureFlag {
		if err := os.MkdirAll(captureDir, 0o755); err != nil {
			slog.Error("creating capture directory", "dir", captureDir, "err", err)
			os.Exit(1)
		}
		slog.Info("capturing every event", "dir", captureDir)
	}

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

	subs := &subscriptions{byID: map[string]subscription{}, capture: *captureFlag}

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

	go startupSpeakers(ctx, client, callbackURL, subs, *cacheFileFlag)
	go renewLoop(ctx, client, subs)

	go func() {
		select {
		case <-time.After(menuRevealDelay):
			subs.revealMenu()
		case <-ctx.Done():
		}
	}()

	runPrompt(ctx, subs)

	slog.Info("shutting down — unsubscribing from every speaker")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	subs.unsubscribeAll(shutdownCtx, client)
	srv.Shutdown(shutdownCtx)
}

func startupSpeakers(ctx context.Context, client *http.Client, callbackURL string, subs *subscriptions, cachePath string) {
	if cachePath != "" {
		cache, ok, err := loadSpeakerCache(cachePath)
		switch {
		case err != nil:
			slog.Warn("reading speaker cache — falling back to discovery", "path", cachePath, "err", err)
		case ok:
			slog.Info("using speaker cache", "path", cachePath, "speakers", len(cache.Speakers), "savedAt", cache.SavedAt)
			if subscribeFromCache(ctx, client, callbackURL, subs, cache.Speakers) > 0 {
				writeCacheAsync(cachePath, subs)
				return
			}
			slog.Warn("no cached speakers reachable — falling back to discovery")
		}
	}
	discoverAndSubscribe(ctx, client, callbackURL, subs)
	if cachePath != "" {
		writeCacheAsync(cachePath, subs)
	}
}

func writeCacheAsync(path string, subs *subscriptions) {
	go func() {
		if err := writeSpeakerCache(path, subs.speakers()); err != nil {
			slog.Warn("writing speaker cache", "path", path, "err", err)
			return
		}
		slog.Info("speaker cache written", "path", path)
	}()
}

func subscribeFromCache(ctx context.Context, client *http.Client, callbackURL string, subs *subscriptions, cached []cachedSpeaker) int {
	var (
		wg    sync.WaitGroup
		count atomic.Int32
	)
	for _, c := range cached {
		wg.Add(1)
		go func(c cachedSpeaker) {
			defer wg.Done()

			speaker := sonos.NewSpeaker(c.IP, c.Location, c.USN, c.Description)
			sid, err := subscribe(ctx, client, speaker, callbackURL)
			if err != nil {
				slog.Warn("cached speaker unreachable — skipping", "speaker", speaker, "err", err)
				return
			}
			subs.addAndAnnounce(subscription{speaker: speaker, sid: sid})
			count.Add(1)
			slog.Info("subscribed (from cache)", "speaker", speaker, "sid", sid)
		}(c)
	}
	wg.Wait()

	n := int(count.Load())
	slog.Info("cache subscribe complete", "speakers", n, "of", len(cached))
	return n
}

func discoverAndSubscribe(ctx context.Context, client *http.Client, callbackURL string, subs *subscriptions) {
	discoverCtx, cancel := context.WithTimeout(ctx, discoverTimeout)
	defer cancel()

	found, err := sonos.Discover(discoverCtx)
	if err != nil {
		slog.Error("discovery failed", "err", err)
		return
	}

	speakerCount := 0
	for speaker := range found {
		slog.Info("speaker found", "speaker", speaker)
		speakerCount++

		sid, err := subscribe(ctx, client, speaker, callbackURL)
		if err != nil {
			slog.Warn("subscribe failed", "speaker", speaker, "err", err)
			continue
		}
		subs.addAndAnnounce(subscription{speaker: speaker, sid: sid})
		slog.Info("subscribed", "speaker", speaker, "sid", sid)
	}

	if speakerCount > 0 {
		slog.Info("Searching complete", "speakers", speakerCount)
	} else {
		slog.Warn("Searching complete. No speakers found.")
	}
}

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
					slog.Warn("renew failed", "speaker", sub.speaker, "err", err)
					continue
				}
				slog.Debug("renewed", "speaker", sub.speaker, "sid", sub.sid)
			}
		}
	}
}

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
	return fmt.Sprintf("Second-%d", subscriptionTimeout)
}

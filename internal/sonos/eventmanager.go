package sonos

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// renewInterval sits well inside subscriptionSeconds so a subscription is
// extended before the speaker expires it.
const renewInterval = 4 * time.Minute

// eventHTTPTimeout bounds each SUBSCRIBE/renew/UNSUBSCRIBE call.
const eventHTTPTimeout = 5 * time.Second

// EventManager keeps GENA subscriptions alive for a set of Devices and
// feeds inbound events back into them. One per process: it owns the single
// HTTP listener every speaker calls back into.
type EventManager struct {
	listener *eventListener
	client   *http.Client

	mu   sync.Mutex
	subs map[string]*subscription
}

type subscription struct {
	device    *Device
	eventPath string
	sid       string
	stop      context.CancelFunc
}

// NewEventManager starts the shared callback listener. Nothing is
// subscribed until Subscribe is called.
func NewEventManager() (*EventManager, error) {
	listener, err := newEventListener()
	if err != nil {
		return nil, err
	}

	return &EventManager{
		listener: listener,
		client:   &http.Client{Timeout: eventHTTPTimeout},
		subs:     map[string]*subscription{},
	}, nil
}

// Subscribe asks device's speaker to report AVTransport changes, and keeps
// the subscription renewed until Close. Inbound events update device's
// state and are published on its Events channel.
func (em *EventManager) Subscribe(ctx context.Context, device *Device) error {
	callbackURL := em.listener.register(device)

	sid, err := subscribe(ctx, em.client, device.client.BaseURL, avTransportEventPath, callbackURL)
	if err != nil {
		em.listener.unregister(device)
		return err
	}

	// Renewal outlives the caller's ctx (which may be scoped to one UI
	// action), so it gets its own cancellable context, stopped by Close.
	renewCtx, stop := context.WithCancel(context.Background())
	sub := &subscription{
		device:    device,
		eventPath: avTransportEventPath,
		sid:       sid,
		stop:      stop,
	}

	em.mu.Lock()
	if existing, ok := em.subs[device.USN]; ok {
		existing.stop()
	}
	em.subs[device.USN] = sub
	em.mu.Unlock()

	go em.renewLoop(renewCtx, sub, callbackURL)

	slog.Info("subscribed", "speaker", device.Speaker, "sid", sid)
	return nil
}

// renewLoop extends sub's lease periodically. A failed renewal falls back
// to a fresh SUBSCRIBE rather than giving up, since otherwise the
// subscription lapses silently and events just stop arriving.
func (em *EventManager) renewLoop(ctx context.Context, sub *subscription, callbackURL string) {
	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			em.mu.Lock()
			sid := sub.sid
			em.mu.Unlock()

			if err := renew(ctx, em.client, sub.device.client.BaseURL, sub.eventPath, sid); err == nil {
				slog.Debug("renewed subscription", "speaker", sub.device.Speaker, "sid", sid)
				continue
			} else {
				slog.Warn("renew failed — resubscribing", "speaker", sub.device.Speaker, "err", err)
			}

			fresh, err := subscribe(ctx, em.client, sub.device.client.BaseURL, sub.eventPath, callbackURL)
			if err != nil {
				slog.Warn("resubscribe failed", "speaker", sub.device.Speaker, "err", err)
				continue
			}

			em.mu.Lock()
			sub.sid = fresh
			em.mu.Unlock()
			slog.Info("resubscribed", "speaker", sub.device.Speaker, "sid", fresh)
		}
	}
}

// Close stops renewals and unsubscribes from every speaker, so they stop
// posting to a callback URL that is about to go away, then shuts the
// listener down. ctx bounds the unsubscribe calls.
func (em *EventManager) Close(ctx context.Context) error {
	em.mu.Lock()
	subs := make([]*subscription, 0, len(em.subs))
	for _, sub := range em.subs {
		subs = append(subs, sub)
	}
	em.subs = map[string]*subscription{}
	em.mu.Unlock()

	for _, sub := range subs {
		sub.stop()
		em.listener.unregister(sub.device)

		if err := unsubscribe(ctx, em.client, sub.device.client.BaseURL, sub.eventPath, sub.sid); err != nil {
			slog.Warn("unsubscribe failed", "speaker", sub.device.Speaker, "err", err)
			continue
		}
		slog.Info("unsubscribed", "speaker", sub.device.Speaker)
	}

	return em.listener.close(ctx)
}

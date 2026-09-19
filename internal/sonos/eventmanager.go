package sonos

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

const (
	renewInterval    = 4 * time.Minute
	eventHTTPTimeout = 5 * time.Second
)

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

// Subscribe subscribes device to every service in subscribedServices. A
// failure on one service does not abandon the others: a speaker with live
// playback events but no volume events is more useful than none at all.
func (em *EventManager) Subscribe(ctx context.Context, device *Device) error {
	var errs []error
	for _, service := range subscribedServices {
		if err := em.subscribeService(ctx, device, service); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", service, err))
		}
		slog.Debug("Subscribed to service", "service", service, "device", device.Speaker)
	}
	return errors.Join(errs...)
}

func (em *EventManager) subscribeService(ctx context.Context, device *Device, service string) error {
	callbackURL := em.listener.register(device, service)

	sid, err := subscribe(ctx, em.client, device.client.BaseURL, service, callbackURL)
	if err != nil {
		em.listener.unregister(device, service)
		return err
	}

	renewCtx, stop := context.WithCancel(context.Background())
	sub := &subscription{
		device:    device,
		eventPath: service,
		sid:       sid,
		stop:      stop,
	}

	key := subKey(device, service)
	em.mu.Lock()
	if existing, ok := em.subs[key]; ok {
		existing.stop()
	}
	em.subs[key] = sub
	em.mu.Unlock()

	go em.renewLoop(renewCtx, sub, callbackURL)

	slog.Info("subscribed", "speaker", device.Speaker, "service", service, "sid", sid)
	return nil
}

// subKey identifies one subscription: a device now has one per service.
func subKey(device *Device, service string) string {
	return device.USN + "|" + service
}

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
		em.listener.unregister(sub.device, sub.eventPath)

		if err := unsubscribe(ctx, em.client, sub.device.client.BaseURL, sub.eventPath, sub.sid); err != nil {
			slog.Warn("unsubscribe failed", "speaker", sub.device.Speaker, "err", err)
			continue
		}
		slog.Info("unsubscribed", "speaker", sub.device.Speaker)
	}

	return em.listener.close(ctx)
}

package sonos

import (
	"context"
	"fmt"
	"net/http"
)

const (
	avTransportEventPath      = "/MediaRenderer/AVTransport/Event"
	renderingControlEventPath = "/MediaRenderer/RenderingControl/Event"

	// contentDirectoryEventPath is the first subscription outside
	// /MediaRenderer. The queue is served by the MediaServer half of the
	// device, which is a separate UPnP device behind the same address.
	contentDirectoryEventPath = "/MediaServer/ContentDirectory/Event"
)

// subscribedServices are the event services each device is subscribed to.
var subscribedServices = []string{
	avTransportEventPath,
	renderingControlEventPath,
	contentDirectoryEventPath,
}

const subscriptionSeconds = 300

func subscribe(ctx context.Context, client *http.Client, baseURL, eventPath, callbackURL string) (string, error) {
	req, err := eventRequest(ctx, baseURL, eventPath, "SUBSCRIBE")
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
		return "", fmt.Errorf("subscribe: unexpected status %d", resp.StatusCode)
	}

	sid := resp.Header.Get("SID")
	if sid == "" {
		return "", fmt.Errorf("subscribe: speaker returned no SID")
	}
	return sid, nil
}

func renew(ctx context.Context, client *http.Client, baseURL, eventPath, sid string) error {
	req, err := eventRequest(ctx, baseURL, eventPath, "SUBSCRIBE")
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
		return fmt.Errorf("renew: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func unsubscribe(ctx context.Context, client *http.Client, baseURL, eventPath, sid string) error {
	req, err := eventRequest(ctx, baseURL, eventPath, "UNSUBSCRIBE")
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
		return fmt.Errorf("unsubscribe: unexpected status %d", resp.StatusCode)
	}
	return nil
}

func eventRequest(ctx context.Context, baseURL, eventPath, method string) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, method, baseURL+eventPath, nil)
}

func timeoutHeader() string {
	return fmt.Sprintf("Second-%d", subscriptionSeconds)
}

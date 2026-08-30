package sonos

import (
	"context"
	"fmt"
	"net/http"
)

// avTransportEventPath is the GENA event endpoint for AVTransport, the
// service that reports transport state and track changes.
const avTransportEventPath = "/MediaRenderer/AVTransport/Event"

// subscriptionSeconds is the lease length requested from the speaker. The
// speaker may grant less, so renewal runs well inside this window.
const subscriptionSeconds = 300

// subscribe registers callbackURL to receive NOTIFYs for eventPath on the
// speaker at baseURL, returning the subscription ID the speaker assigns.
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

// renew extends an existing subscription. Unlike subscribe it identifies
// the subscription by SID rather than supplying a callback URL again.
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

// unsubscribe cancels a subscription so the speaker stops sending NOTIFYs
// to a callback URL that is about to disappear.
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

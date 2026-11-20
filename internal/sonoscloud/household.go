package sonoscloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
)

const (
	// controlProtocol is the subprotocol the websocket needs: without it the
	// server answers 200 and does not open the websocket.
	controlProtocol = "v1.api.smartspeaker.audio"

	// householdWait is the longest the household question may take, from
	// the ticket to the answer.
	householdWait = 15 * time.Second
)

// controlAPI is Sonos's Control API websocket, which the web app opens with a
// ticket from its own backend. Only tests change it.
var controlAPI = "wss://api.ws.sonos.com/websocket"

// ErrHouseholdUnknown says the account holds no household that is this one,
// or more than one with nothing to choose between them.
var ErrHouseholdUnknown = errors.New("sonoscloud: cannot tell which household is this one")

// Household is one Sonos household the account holds.
type Household struct {
	ID        string `json:"id"`        // the cloud's id: the speakers' id, a dot and more
	OwnerLUID string `json:"ownerLuid"` // the account that owns it, which Sonos's own lists use
}

// HouseholdStore keeps the household between runs. found is false when none
// is kept.
type HouseholdStore interface {
	LoadHousehold() (h Household, found bool, err error)
	SaveHousehold(Household) error
	ClearHousehold() error
}

// controlHeader is the first object of a Control API message.
type controlHeader struct {
	Command   string `json:"command,omitempty"`
	Namespace string `json:"namespace"`
	CorrID    string `json:"corrId"`
	Type      string `json:"type,omitempty"`
	Success   bool   `json:"success,omitempty"`
}

// WithHouseholdStore keeps the household in store, so that it is asked for
// only when the kept one no longer fits.
func WithHouseholdStore(store HouseholdStore) Option {
	return func(c *Client) { c.households = store }
}

// LocalID is the household's id as its speakers report it: the cloud's id up
// to the dot.
func (h Household) LocalID() string {
	id, _, _ := strings.Cut(h.ID, ".")
	return id
}

// PickHousehold chooses this household from the account's: the one whose
// LocalID the speakers report, or, with no speaker found (localID empty), the
// only one.
func PickHousehold(households []Household, localID string) (Household, error) {
	if localID == "" {
		if len(households) == 1 {
			return households[0], nil
		}
		return Household{}, ErrHouseholdUnknown
	}
	for _, h := range households {
		if h.LocalID() == localID {
			return h, nil
		}
	}
	return Household{}, ErrHouseholdUnknown
}

// Household is this household, as PickHousehold chooses it. A kept household
// that still fits is answered without a request; otherwise the account's
// households are asked for, and the choice is kept.
func (c *Client) Household(ctx context.Context, localID string) (Household, error) {
	if c.households != nil {
		kept, found, err := c.households.LoadHousehold()
		if err != nil {
			return Household{}, err
		}
		if found && (localID == "" || kept.LocalID() == localID) {
			return kept, nil
		}
	}

	households, err := c.Households(ctx)
	if err != nil {
		return Household{}, err
	}
	h, err := PickHousehold(households, localID)
	if err != nil {
		return Household{}, err
	}
	if c.households != nil {
		if err := c.households.SaveHousehold(h); err != nil {
			return Household{}, err
		}
	}
	return h, nil
}

// Households asks the Sonos cloud for the account's households, as the web
// app does: a ticket from its backend opens the Control API websocket, and
// getHouseholds lists them.
func (c *Client) Households(ctx context.Context) ([]Household, error) {
	ctx, cancel := context.WithTimeout(ctx, householdWait)
	defer cancel()

	body, err := c.get(ctx, "/api/mfe", "")
	if err != nil {
		return nil, err
	}
	var mfe struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(body, &mfe); err != nil || mfe.Ticket == "" {
		return nil, errors.New("sonoscloud: the web app gave no ticket for the Control API")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	conn, err := c.dialControlAPI(ctx, mfe.Ticket)
	if err != nil {
		return nil, err
	}
	defer conn.CloseNow()

	corrID := uuid.NewString() // matches the answer to the question
	ask := []any{
		controlHeader{Command: "getHouseholds", Namespace: "households", CorrID: corrID},
		map[string]string{"connectedOnly": "true"},
	}
	if err := wsjson.Write(ctx, conn, ask); err != nil {
		return nil, fmt.Errorf("sonoscloud: asking for the households: %w", err)
	}
	households, err := awaitHouseholds(ctx, conn, corrID)
	if err != nil {
		return nil, err
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
	return households, nil
}

// dialControlAPI opens the Control API websocket with a ticket, sending what
// the browser sends when the web app opens it. The ticket is short-lived and
// never logged.
func (c *Client) dialControlAPI(ctx context.Context, ticket string) (*websocket.Conn, error) {
	target, err := url.Parse(controlAPI)
	if err != nil {
		return nil, fmt.Errorf("sonoscloud: the Control API address: %w", err)
	}
	target.RawQuery = url.Values{"ticket": {ticket}}.Encode()

	// The websocket library refuses a client timeout: ctx bounds the dial.
	hc := *c.http
	hc.Timeout = 0
	header := http.Header{}
	header.Set("Origin", origin(c.base))
	header.Set("User-Agent", browserAgent)
	header.Set("Accept-Language", browserLocale)
	header.Set("Cache-Control", "no-cache")
	header.Set("Pragma", "no-cache")

	conn, resp, err := websocket.Dial(ctx, target.String(), &websocket.DialOptions{
		HTTPClient: &hc, HTTPHeader: header, Subprotocols: []string{controlProtocol},
	})
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("sonoscloud: opening the Control API: %w", unwrapURLError(err))
	}
	slog.Debug("sonoscloud", "method", "WS", "host", target.Host, "path", target.Path, "status", http.StatusSwitchingProtocols)
	return conn, nil
}

// awaitHouseholds reads messages until the answer to corrID arrives. Other
// messages, such as events the server sends unasked, are passed over.
func awaitHouseholds(ctx context.Context, conn *websocket.Conn, corrID string) ([]Household, error) {
	for {
		var message []json.RawMessage
		if err := wsjson.Read(ctx, conn, &message); err != nil {
			return nil, fmt.Errorf("sonoscloud: waiting for the households: %w", err)
		}
		var header controlHeader
		if len(message) == 0 || json.Unmarshal(message[0], &header) != nil || header.CorrID != corrID {
			continue
		}
		var body struct {
			Households []Household `json:"households"`
		}
		if !header.Success || len(message) < 2 || json.Unmarshal(message[1], &body) != nil {
			return nil, fmt.Errorf("sonoscloud: getHouseholds failed (%s/%s)", header.Namespace, header.Type)
		}
		return body.Households, nil
	}
}

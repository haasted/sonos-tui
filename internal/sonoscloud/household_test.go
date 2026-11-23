package sonoscloud_test

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// Two households of one account, as the Control API lists them.
var (
	home    = sonoscloud.Household{ID: "Sonos_home0000000000000000000000.cloud0000000000000000", OwnerLUID: "111"}
	cottage = sonoscloud.Household{ID: "Sonos_cottage00000000000000000000.cloud1111111111111111", OwnerLUID: "111"}
)

// Case: the account's households asked for. A ticket from the web app opens
// the websocket with the subprotocol and the web app's Origin, and an event
// sent unasked before the answer is passed over.
func TestHouseholds_AsksTheControlAPI(t *testing.T) {
	f := newFakeControl(t, home, cottage)

	got, err := f.client(t, nil).Households(t.Context())

	require.NoError(t, err)
	assert.Equal(t, []sonoscloud.Household{home, cottage}, got)
	assert.Equal(t, "ticket-1", f.ticket)
	assert.Equal(t, f.web.URL, f.origin)
}

// Case: the household to use, chosen from the account's by the id the
// speakers report, or, with no speaker found, the only one.
func TestPickHousehold(t *testing.T) {
	for name, tc := range map[string]struct {
		households []sonoscloud.Household
		localID    string
		want       sonoscloud.Household
		err        error
	}{
		"the speakers' household":             {[]sonoscloud.Household{home, cottage}, cottage.LocalID(), cottage, nil},
		"no speaker, one household":           {[]sonoscloud.Household{home}, "", home, nil},
		"no speaker, two households":          {[]sonoscloud.Household{home, cottage}, "", sonoscloud.Household{}, sonoscloud.ErrHouseholdUnknown},
		"the speakers' household is not here": {[]sonoscloud.Household{home}, "Sonos_elsewhere", sonoscloud.Household{}, sonoscloud.ErrHouseholdUnknown},
	} {
		got, err := sonoscloud.PickHousehold(tc.households, tc.localID)
		assert.ErrorIs(t, err, tc.err, name)
		assert.Equal(t, tc.want, got, name)
	}
}

// Case: the cloud's id and the speakers'. The speakers' is the cloud's up to
// the dot, as measured on a real household.
func TestHousehold_LocalIDIsTheCloudsUpToTheDot(t *testing.T) {
	assert.Equal(t, "Sonos_home0000000000000000000000", home.LocalID())
}

// Case: a household kept from before that fits the speakers. It is answered
// without a request.
func TestHousehold_AKeptHouseholdIsUsedAsItIs(t *testing.T) {
	f := newFakeControl(t, home, cottage)
	kept := &memHouseholds{household: home, found: true}

	got, err := f.client(t, kept).Household(t.Context(), home.LocalID())

	require.NoError(t, err)
	assert.Equal(t, home, got)
	assert.Zero(t, f.tickets, "a request went out")
}

// Case: the kept household is not the speakers' any more, as after a move to
// another household. The households are asked for, and the new one is kept.
func TestHousehold_ANewHouseholdIsAskedForAndKept(t *testing.T) {
	f := newFakeControl(t, home, cottage)
	kept := &memHouseholds{household: home, found: true}

	got, err := f.client(t, kept).Household(t.Context(), cottage.LocalID())

	require.NoError(t, err)
	assert.Equal(t, cottage, got)
	assert.Equal(t, cottage, kept.household)
}

// Case: no store given. The household is kept in memory, so a second call
// is answered without a request.
func TestHousehold_KeptInMemoryWithoutAStore(t *testing.T) {
	f := newFakeControl(t, home, cottage)
	c := f.client(t, nil)

	for range 2 {
		got, err := c.Household(t.Context(), home.LocalID())
		require.NoError(t, err)
		assert.Equal(t, home, got)
	}

	assert.Equal(t, 1, f.tickets)
}

// Case: no saved session. The household cannot be asked for, and nothing is
// sent to either server.
func TestHouseholds_SignedOut(t *testing.T) {
	f := newFakeControl(t, home)

	_, err := f.client(t, nil, withoutSession).Households(t.Context())

	assert.ErrorIs(t, err, sonoscloud.ErrSignedOut)
	assert.Zero(t, f.tickets)
}

// Case: the Control API answers that getHouseholds failed. That is an error,
// not an empty account.
func TestHouseholds_AFailedAnswer(t *testing.T) {
	f := newFakeControl(t)
	f.fail = true

	_, err := f.client(t, nil).Households(t.Context())

	assert.ErrorContains(t, err, "getHouseholds failed")
}

// fakeControl is the web app, which hands out tickets, and the Control API,
// which lists households over a websocket.
type fakeControl struct {
	web, control *httptest.Server
	households   []sonoscloud.Household
	fail         bool // answer getHouseholds with success false

	mu      sync.Mutex
	tickets int    // tickets handed out
	ticket  string // the ticket the websocket was opened with
	origin  string // the Origin it was opened with
}

// clientOption changes the session a fake client starts with.
type clientOption func(*sonoscloud.Session)

func withoutSession(s *sonoscloud.Session) { *s = sonoscloud.Session{} }

func newFakeControl(t *testing.T, households ...sonoscloud.Household) *fakeControl {
	t.Helper()
	f := &fakeControl{households: households}
	f.web = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("fake web app got %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/api/mfe" {
			http.NotFound(w, r)
			return
		}
		f.mu.Lock()
		f.tickets++
		n := f.tickets
		f.mu.Unlock()
		fmt.Fprintf(w, `{"ticket":"ticket-%d","fingerprint":"f"}`, n)
	}))
	f.control = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("fake Control API got %s %s", r.Method, r.URL.Path)
		f.serveControl(t, w, r)
	}))
	t.Cleanup(f.web.Close)
	t.Cleanup(f.control.Close)
	return f
}

// client is a client of the fake servers, trusting both, with a valid saved
// session unless an option changes it.
func (f *fakeControl) client(t *testing.T, kept sonoscloud.HouseholdStore, opts ...clientOption) *sonoscloud.Client {
	t.Helper()
	session := validSession(sessionName)
	for _, opt := range opts {
		opt(&session)
	}
	roots := x509.NewCertPool()
	roots.AddCert(f.web.Certificate())
	roots.AddCert(f.control.Certificate())
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}}
	sonoscloud.SetControlAPI(t, "wss://"+strings.TrimPrefix(f.control.URL, "https://")+"/websocket")
	options := []sonoscloud.Option{sonoscloud.WithBaseURL(f.web.URL), sonoscloud.WithHTTPClient(hc)}
	if kept != nil {
		options = append(options, sonoscloud.WithHouseholdStore(kept))
	}
	c, err := sonoscloud.New(append(options, sonoscloud.WithSessionStore(&memStore{session: session}))...)
	require.NoError(t, err)
	return c
}

// serveControl accepts the websocket only with the subprotocol, sends an
// event first, then answers getHouseholds.
func (f *fakeControl) serveControl(t *testing.T, w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.ticket, f.origin = r.URL.Query().Get("ticket"), r.Header.Get("Origin")
	f.mu.Unlock()
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{"v1.api.smartspeaker.audio"}, InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	if conn.Subprotocol() != "v1.api.smartspeaker.audio" {
		conn.Close(websocket.StatusPolicyViolation, "no subprotocol")
		return
	}

	var ask []json.RawMessage
	if err := wsjson.Read(r.Context(), conn, &ask); err != nil || len(ask) < 2 {
		return
	}
	var header struct{ Command, Namespace, CorrID string }
	if json.Unmarshal(ask[0], &header) != nil || header.Command != "getHouseholds" {
		return
	}

	event := []any{map[string]any{"namespace": "groups", "type": "groupsChanged", "corrId": "other"}, map[string]any{}}
	_ = wsjson.Write(r.Context(), conn, event)
	answer := []any{
		map[string]any{"namespace": "households", "type": "households", "corrId": header.CorrID, "success": !f.fail},
		map[string]any{"households": f.households},
	}
	_ = wsjson.Write(r.Context(), conn, answer)
	_, _, _ = conn.Read(r.Context()) // until the client closes
}

// memHouseholds is a HouseholdStore in memory.
type memHouseholds struct {
	household sonoscloud.Household
	found     bool
}

var _ sonoscloud.HouseholdStore = &memHouseholds{}

func (m *memHouseholds) LoadHousehold() (sonoscloud.Household, bool, error) {
	return m.household, m.found, nil
}

func (m *memHouseholds) SaveHousehold(h sonoscloud.Household) error {
	m.household, m.found = h, true
	return nil
}

func (m *memHouseholds) ClearHousehold() error {
	m.household, m.found = sonoscloud.Household{}, false
	return nil
}

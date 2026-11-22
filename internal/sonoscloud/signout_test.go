package sonoscloud_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// idToken stands in for the id_token_hint the web app's sign-out puts in
// its redirect. No output may show it.
const idToken = "id-token-hint-value"

// Case: a sign-out with a valid session. The web app's sign-out gets the
// session as the router's fetch, the OAuth server's sign-out gets the
// token, and the session and household are deleted.
func TestSignOut_TellsTheWebAppAndTheOAuthServer(t *testing.T) {
	f := newFakeSignOut(t)
	store := &memStore{session: validSession(sessionName)}
	households := &memHouseholds{household: sonoscloud.Household{ID: "Sonos_HOUSEHOLD.PLACEHOLDER"}, found: true}
	c := newClient(t, f.srv, store, sonoscloud.WithHouseholdStore(households))

	require.NoError(t, c.SignOut(t.Context()))

	logout := f.request("/api/auth/logout")
	require.NotNil(t, logout, "the web app was not told")
	cookie, err := logout.Cookie(sessionName)
	require.NoError(t, err)
	assert.Equal(t, secret, cookie.Value)
	assert.Equal(t, "1", logout.Header.Get("RSC"))
	assert.NotEmpty(t, logout.URL.Query().Get("_rsc"))
	assert.Equal(t, "cors", logout.Header.Get("Sec-Fetch-Mode"))
	assert.Equal(t, f.srv.URL+"/en-us/web-app", logout.Header.Get("Referer"))

	oauth := f.request("/oauth/v4/meridian/logout")
	require.NotNil(t, oauth, "the OAuth server was not told")
	assert.Equal(t, idToken, oauth.URL.Query().Get("id_token_hint"))
	assert.Equal(t, "navigate", oauth.Header.Get("Sec-Fetch-Mode"))
	assert.Equal(t, f.srv.URL+"/", oauth.Header.Get("Referer"))
	assert.NotNil(t, f.request("/logout"), "the redirect back to the web app was not followed")

	assert.True(t, store.cleared, "the saved session stayed")
	_, found, _ := households.LoadHousehold()
	assert.False(t, found, "the household stayed")
	assert.False(t, c.SignedIn())
}

// Case: Sonos cannot be told, by a refusal, a dead server or a redirect to
// a host not Sonos's. The local data goes all the same, the redirect is not
// followed, and the error says Sonos was not told.
func TestSignOut_LocalDataGoesWhenSonosIsNotTold(t *testing.T) {
	var foreignAsked int
	foreign := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { foreignAsked++ }))
	t.Cleanup(foreign.Close)

	for name, logout := range map[string]http.HandlerFunc{
		"refused": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"foreign": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, foreign.URL+"/logout?id_token_hint="+idToken, http.StatusTemporaryRedirect)
		},
	} {
		srv := fakeWebApp(t, logout)
		store := &memStore{session: validSession(sessionName)}
		c := newClient(t, srv, store)

		err := c.SignOut(t.Context())

		assert.ErrorIs(t, err, sonoscloud.ErrServerNotTold, name)
		assert.True(t, store.cleared, "%s: the saved session stayed", name)
		assert.False(t, c.SignedIn(), name)
	}
	assert.Zero(t, foreignAsked, "a redirect to a host not Sonos's was followed")

	dead := fakeWebApp(t, func(http.ResponseWriter, *http.Request) {})
	store := &memStore{session: validSession(sessionName)}
	c := newClient(t, dead, store)
	dead.Close()
	assert.ErrorIs(t, c.SignOut(t.Context()), sonoscloud.ErrServerNotTold, "dead")
	assert.True(t, store.cleared, "dead: the saved session stayed")
}

// Case: a sign-out with no valid session. Sonos is asked nothing, as there
// is nothing to end, and what is kept is deleted all the same.
func TestSignOut_SignedOutAsksNothing(t *testing.T) {
	var requests int
	srv := fakeWebApp(t, func(http.ResponseWriter, *http.Request) { requests++ })
	store := &memStore{}

	require.NoError(t, newClient(t, srv, store).SignOut(t.Context()))

	assert.Zero(t, requests)
	assert.True(t, store.cleared)
}

// Case: a sign-out that works, and one the OAuth server refuses. The debug
// log names each step's headers and cookies, but neither the log nor the
// error shows the token or the session.
func TestSignOut_TheTokenNeverReachesOutput(t *testing.T) {
	var log bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	f := newFakeSignOut(t)
	require.NoError(t, newClient(t, f.srv, &memStore{session: validSession(sessionName)}).SignOut(t.Context()))
	assert.Contains(t, log.String(), "id_token_hint=%E2%80%A6", "the log does not name the token's parameter")
	assert.Contains(t, log.String(), sessionName+" (max-age -1", "the log does not name the cookie cleared")

	f.oauthStatus = http.StatusBadRequest
	err := newClient(t, f.srv, &memStore{session: validSession(sessionName)}).SignOut(t.Context())

	require.ErrorIs(t, err, sonoscloud.ErrServerNotTold)
	for what, text := range map[string]string{"the error": err.Error(), "the log": log.String()} {
		assert.NotContains(t, text, idToken, what)
		assert.NotContains(t, text, secret, what)
	}
}

// fakeSignOut is the web app and the OAuth server in one TLS server, as
// both answer a sign-out, recording each request by path.
type fakeSignOut struct {
	srv         *httptest.Server
	oauthStatus int                      // what the OAuth server answers; a redirect back when 0
	requests    map[string]*http.Request // the last request for each path

	mu sync.Mutex // guards requests
}

func newFakeSignOut(t *testing.T) *fakeSignOut {
	f := &fakeSignOut{requests: map[string]*http.Request{}}
	f.srv = fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests[r.URL.Path] = r.Clone(r.Context())
		f.mu.Unlock()
		switch r.URL.Path {
		case "/api/auth/logout":
			http.SetCookie(w, &http.Cookie{Name: sessionName, Value: secret, Path: "/", MaxAge: -1, Secure: true})
			back := url.Values{"id_token_hint": {idToken}, "post_logout_redirect_uri": {f.srv.URL + "/logout"}}
			http.Redirect(w, r, f.srv.URL+"/oauth/v4/meridian/logout?"+back.Encode(), http.StatusTemporaryRedirect)
		case "/oauth/v4/meridian/logout":
			if f.oauthStatus != 0 {
				w.WriteHeader(f.oauthStatus)
				return
			}
			http.Redirect(w, r, r.URL.Query().Get("post_logout_redirect_uri"), http.StatusFound)
		}
	})
	return f
}

// request is the request the server got for path, or nil.
func (f *fakeSignOut) request(path string) *http.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[path]
}

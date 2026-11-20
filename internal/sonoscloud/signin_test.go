package sonoscloud_test

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// The account the fake Okta knows, and an account it has locked.
const (
	goodUser     = "reader@example.com"
	goodPassword = "correct horse"
	lockedUser   = "locked@example.com"
)

// Case: a good username and password. The sign-in goes through the web app
// and Okta, saves the new session in place of the old one, and the client
// is signed in with it at once.
func TestSignIn_SavesTheNewSession(t *testing.T) {
	f := newFakeSignIn(t)
	store := &memStore{session: validSession(sessionName)} // an old session, to be replaced
	c := f.client(t, store)

	require.NoError(t, c.SignIn(t.Context(), goodUser, goodPassword))

	saved := store.saved()
	require.Len(t, saved.Cookies, 1)
	assert.Equal(t, "new-session", saved.Cookies[0].Value)
	_, err := c.Session(t.Context())
	assert.NoError(t, err, "the client is not signed in with the new session")
}

// Case: the requests of a sign-in. Each looks as the browser's would: page
// loads as navigations, the page's fetches as fetches, and the password sent
// to Okta from Okta's own page.
func TestSignIn_LooksLikeTheBrowser(t *testing.T) {
	f := newFakeSignIn(t)

	require.NoError(t, f.client(t, &memStore{}).SignIn(t.Context(), goodUser, goodPassword))

	landing := f.request("GET /")
	assert.Equal(t, "navigate", landing.Get("Sec-Fetch-Mode"))
	assert.Equal(t, "none", landing.Get("Sec-Fetch-Site"))
	assert.Equal(t, "cors", f.request("GET /api/auth/csrf").Get("Sec-Fetch-Mode"))
	signin := f.request("POST /api/auth/signin/okta")
	assert.Equal(t, f.web.URL, signin.Get("Origin"))
	authorize := f.request("GET /oauth2/default/v1/authorize")
	assert.Equal(t, "cross-site", authorize.Get("Sec-Fetch-Site"))
	authn := f.request("POST /api/v1/authn")
	assert.Equal(t, f.okta.URL, authn.Get("Origin"))
	assert.Contains(t, authn.Get("Referer"), "/oauth2/default/v1/authorize")
	assert.Equal(t, "same-origin", authn.Get("Sec-Fetch-Site"))
}

// Case: a wrong password. The sign-in says so, and the session saved
// before is kept as it was.
func TestSignIn_AWrongPassword(t *testing.T) {
	f := newFakeSignIn(t)
	old := validSession(sessionName)
	store := &memStore{session: old}

	err := f.client(t, store).SignIn(t.Context(), goodUser, "wrong")

	assert.ErrorIs(t, err, sonoscloud.ErrBadCredentials)
	assert.Equal(t, old, store.saved())
}

// Case: Okta answers with a status other than success, here a locked
// account. The sign-in stops and names the status.
func TestSignIn_AnUnfinishedSignInStopsIt(t *testing.T) {
	f := newFakeSignIn(t)

	err := f.client(t, &memStore{}).SignIn(t.Context(), lockedUser, goodPassword)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "LOCKED_OUT")
	assert.NotErrorIs(t, err, sonoscloud.ErrBadCredentials)
}

// Case: Okta's page without the state token, as when its sign-in changes.
// The sign-in says the flow has changed and sends no password.
func TestSignIn_AChangedOktaPage(t *testing.T) {
	f := newFakeSignIn(t)
	f.oktaPage = `<html>a new sign-in</html>`

	err := f.client(t, &memStore{}).SignIn(t.Context(), goodUser, goodPassword)

	assert.ErrorIs(t, err, sonoscloud.ErrFlowChanged)
	assert.Nil(t, f.request("POST /api/v1/authn"), "the password was sent anyway")
}

// Case: every sign-in that fails. Neither the username nor the password
// shows in the error or in the debug log.
func TestSignIn_TheCredentialsNeverReachOutput(t *testing.T) {
	var log bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	f := newFakeSignIn(t)

	for _, user := range []string{goodUser, lockedUser} {
		err := f.client(t, &memStore{}).SignIn(t.Context(), user, "wrong-"+goodPassword)
		require.Error(t, err)
		for _, secret := range []string{user, "wrong-" + goodPassword} {
			assert.NotContains(t, err.Error(), secret)
			assert.NotContains(t, log.String(), secret)
		}
	}
}

// fakeSignIn is the web app and Okta, each a TLS server, answering the
// sign-in as SONOS-CLOUD-NOTES.md describes it.
type fakeSignIn struct {
	t         *testing.T
	web, okta *httptest.Server
	oktaPage  string // the sign-in page Okta serves

	mu   sync.Mutex
	seen map[string]http.Header // the headers of the first request, by method and path
}

func newFakeSignIn(t *testing.T) *fakeSignIn {
	t.Helper()
	f := &fakeSignIn{t: t, seen: map[string]http.Header{},
		oktaPage: `<script>var oktaData = {"signIn":{"stateToken":"00\x2Dstate-token"}};</script>`}
	f.okta = httptest.NewTLSServer(http.HandlerFunc(f.serveOkta))
	f.web = httptest.NewTLSServer(http.HandlerFunc(f.serveWeb))
	t.Cleanup(f.okta.Close)
	t.Cleanup(f.web.Close)
	return f
}

// client is a client of the fake web app that trusts both fake servers.
func (f *fakeSignIn) client(t *testing.T, store sonoscloud.SessionStore) *sonoscloud.Client {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(f.web.Certificate())
	roots.AddCert(f.okta.Certificate())
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}}
	c, err := sonoscloud.New(sonoscloud.WithSessionStore(store), sonoscloud.WithBaseURL(f.web.URL), sonoscloud.WithHTTPClient(hc))
	require.NoError(t, err)
	return c
}

// request is the headers of the first request seen with this method and
// path, or nil when none came.
func (f *fakeSignIn) request(key string) http.Header {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seen[key]
}

// record keeps the request's headers and logs that the fake server got it.
func (f *fakeSignIn) record(server string, r *http.Request) {
	f.t.Logf("fake %s got %s %s", server, r.Method, r.URL.Path)
	f.mu.Lock()
	defer f.mu.Unlock()
	if key := r.Method + " " + r.URL.Path; f.seen[key] == nil {
		f.seen[key] = r.Header.Clone()
	}
}

func (f *fakeSignIn) serveWeb(w http.ResponseWriter, r *http.Request) {
	f.record("web app", r)
	switch r.Method + " " + r.URL.Path {
	case "GET /":
		if _, err := r.Cookie(sessionName); err != nil {
			http.Redirect(w, r, "/en-us/login", http.StatusTemporaryRedirect)
		}
	case "GET /en-us/login":
		fmt.Fprint(w, "<html>the sign-in starts in a script</html>")
	case "GET /api/auth/csrf":
		http.SetCookie(w, &http.Cookie{Name: "__Host-next-auth.csrf-token", Value: "csrf", Path: "/", Secure: true})
		fmt.Fprint(w, `{"csrfToken":"csrf"}`)
	case "POST /api/auth/signin/okta":
		if r.FormValue("csrfToken") != "csrf" || r.FormValue("json") != "true" {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, `{"url":%q}`, f.okta.URL+"/oauth2/default/v1/authorize?state=s")
	case "GET /api/auth/callback/okta":
		if r.URL.Query().Get("code") != "good" {
			http.Error(w, "bad code", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: sessionName, Value: "new-session", Path: "/", MaxAge: 864000, Secure: true})
		http.Redirect(w, r, "/", http.StatusFound)
	case "GET /api/auth/session":
		if c, err := r.Cookie(sessionName); err == nil && c.Value == "new-session" {
			fmt.Fprint(w, `{"expires":"2026-11-25T12:00:00.000Z"}`)
			return
		}
		fmt.Fprint(w, `{}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeSignIn) serveOkta(w http.ResponseWriter, r *http.Request) {
	f.record("Okta", r)
	switch r.Method + " " + r.URL.Path {
	case "GET /oauth2/default/v1/authorize":
		fmt.Fprint(w, f.oktaPage)
	case "POST /api/v1/authn":
		var got struct{ Username, Password, StateToken string }
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil || got.StateToken != "00-state-token" {
			http.Error(w, `{"errorCode":"E0000011"}`, http.StatusForbidden)
			return
		}
		switch {
		case got.Username == lockedUser:
			fmt.Fprint(w, `{"status":"LOCKED_OUT"}`)
		case got.Username != goodUser || got.Password != goodPassword:
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"errorCode":"E0000004","errorSummary":"Authentication failed"}`)
		default:
			fmt.Fprintf(w, `{"status":"SUCCESS","_links":{"next":{"href":%q}}}`, f.okta.URL+"/login/step-up/redirect?stateToken=00-state-token")
		}
	case "GET /login/step-up/redirect":
		http.Redirect(w, r, f.web.URL+"/api/auth/callback/okta?code=good", http.StatusFound)
	default:
		http.NotFound(w, r)
	}
}

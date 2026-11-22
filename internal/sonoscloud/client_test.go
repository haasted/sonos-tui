package sonoscloud_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// secret is a session value no output may ever show.
const secret = "s3cret-session-value"

const sessionName = "__Secure-next-auth.session-token"

// Case: a request with a saved session. It carries every part of the
// session cookie, and the headers the browser sends with the web app's fetch.
func TestClient_SendsTheSessionAndTheBrowsersHeaders(t *testing.T) {
	var got *http.Request
	srv := fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(r.Context())
		fmt.Fprint(w, `{"expires":"2026-11-24T12:00:00.000Z"}`)
	})
	store := &memStore{session: validSession(sessionName+".0", sessionName+".1")}

	_, err := newClient(t, srv, store).Session(t.Context())
	require.NoError(t, err)

	for _, part := range []string{sessionName + ".0", sessionName + ".1"} {
		cookie, err := got.Cookie(part)
		require.NoError(t, err, "the request did not carry %s", part)
		assert.Equal(t, secret, cookie.Value)
	}
	assert.Contains(t, got.Header.Get("User-Agent"), "Chrome/")
	assert.Equal(t, "application/json", got.Header.Get("Accept"))
	assert.Equal(t, srv.URL+"/en-us/web-app", got.Header.Get("Referer"))
	assert.Equal(t, "same-origin", got.Header.Get("Sec-Fetch-Site"))
	assert.Equal(t, "cors", got.Header.Get("Sec-Fetch-Mode"))
	assert.Equal(t, `"macOS"`, got.Header.Get("Sec-Ch-Ua-Platform"))
}

// Case: the web app's answer about the session. Its expiry is read; an
// answer with no session, and a 401, both say signed out.
func TestClient_SessionAnswers(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   error
	}{
		"signed in":  {http.StatusOK, `{"user":{},"expires":"2026-11-24T12:00:00.000Z"}`, nil},
		"no session": {http.StatusOK, `{}`, sonoscloud.ErrSignedOut},
		"refused":    {http.StatusUnauthorized, `{"error":"missing JWT"}`, sonoscloud.ErrSignedOut},
	} {
		srv := fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			fmt.Fprint(w, tc.body)
		})

		info, err := newClient(t, srv, &memStore{session: validSession(sessionName)}).Session(t.Context())

		if tc.want != nil {
			assert.ErrorIs(t, err, tc.want, name)
			continue
		}
		require.NoError(t, err, name)
		assert.Equal(t, time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC), info.Expires.UTC(), name)
	}
}

// Case: no saved session, and one that has run out. Both say signed out
// without asking the server anything.
func TestClient_NoValidSessionSendsNothing(t *testing.T) {
	expired := validSession(sessionName)
	expired.Cookies[0].Expires = time.Now().Add(-time.Minute)

	for name, session := range map[string]sonoscloud.Session{"none": {}, "expired": expired} {
		var requests int
		srv := fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) { requests++ })

		_, err := newClient(t, srv, &memStore{session: session}).Session(t.Context())

		assert.ErrorIs(t, err, sonoscloud.ErrSignedOut, name)
		assert.Zero(t, requests, "%s: a request went out", name)
	}
}

// Case: a valid session, none, and one that has run out. SignedIn says which
// without a request.
func TestClient_SignedInAsksNothing(t *testing.T) {
	expired := validSession(sessionName)
	expired.Cookies[0].Expires = time.Now().Add(-time.Minute)

	for name, tc := range map[string]struct {
		session sonoscloud.Session
		want    bool
	}{
		"valid":   {validSession(sessionName), true},
		"none":    {sonoscloud.Session{}, false},
		"expired": {expired, false},
	} {
		var requests int
		srv := fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) { requests++ })

		assert.Equal(t, tc.want, newClient(t, srv, &memStore{session: tc.session}).SignedIn(), name)
		assert.Zero(t, requests, "%s: a request went out", name)
	}
}

// Case: a saved session, and none. SessionEnds gives the cookie's expiry, or
// the zero time, without a request.
func TestClient_SessionEndsAsItsCookieSays(t *testing.T) {
	var requests int
	srv := fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) { requests++ })
	session := validSession(sessionName)

	ends := newClient(t, srv, &memStore{session: session}).SessionEnds()
	none := newClient(t, srv, &memStore{}).SessionEnds()

	assert.True(t, ends.Equal(session.Expires()), "got %v, want %v", ends, session.Expires())
	assert.True(t, none.IsZero())
	assert.Zero(t, requests)
}

// Case: the server renews the session, as it does on every use. The new
// value and its new expiry are saved at once.
func TestClient_SavesARenewedSession(t *testing.T) {
	srv := fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: sessionName, Value: "renewed", Path: "/", MaxAge: 864000, Secure: true})
		fmt.Fprint(w, `{"expires":"2026-11-24T12:00:00.000Z"}`)
	})
	store := &memStore{session: validSession(sessionName)}

	_, err := newClient(t, srv, store).Session(t.Context())
	require.NoError(t, err)

	saved := store.saved()
	require.Len(t, saved.Cookies, 1)
	assert.Equal(t, "renewed", saved.Cookies[0].Value)
	assert.WithinDuration(t, time.Now().Add(240*time.Hour), saved.Expires(), time.Minute)
}

// Case: the server deletes the session cookie, as it does on signing out.
// The saved session is cleared, and the next request says signed out.
func TestClient_ClearsADeletedSession(t *testing.T) {
	srv := fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: sessionName, Value: "", Path: "/", MaxAge: -1, Secure: true})
		fmt.Fprint(w, `{}`)
	})
	store := &memStore{session: validSession(sessionName)}
	c := newClient(t, srv, store)

	_, _ = c.Session(t.Context())
	_, err := c.Session(t.Context())

	assert.True(t, store.cleared, "the deleted session was not cleared")
	assert.ErrorIs(t, err, sonoscloud.ErrSignedOut)
}

// Case: every way the session can reach output: an error from a failed
// request, the session printed in each form, and the debug log. The value
// shows in none of them.
func TestClient_TheSessionNeverReachesOutput(t *testing.T) {
	var log bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	srv := fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: sessionName, Value: secret, Path: "/", MaxAge: 60, Secure: true})
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"echo":"`+secret+`"}`)
	})
	session := validSession(sessionName)

	_, err := newClient(t, srv, &memStore{session: session}).Session(t.Context())
	require.Error(t, err)
	var status *sonoscloud.StatusError
	assert.True(t, errors.As(err, &status), "a failed request is not a StatusError")
	slog.Info("session", "session", session, "cookie", session.Cookies[0])

	for what, text := range map[string]string{
		"the error": err.Error(),
		"%v":        fmt.Sprintf("%v", session),
		"%+v":       fmt.Sprintf("%+v", session),
		"%#v":       fmt.Sprintf("%#v", session),
		"the log":   log.String(),
	} {
		assert.NotContains(t, text, secret, what)
	}
}

// fakeWebApp is a TLS server, so the session's Secure cookies are sent. It
// logs each request it gets.
func fakeWebApp(t *testing.T, handle http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("fake web app got %s %s", r.Method, r.URL.Path)
		handle(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newClient(t *testing.T, srv *httptest.Server, store sonoscloud.SessionStore, opts ...sonoscloud.Option) *sonoscloud.Client {
	t.Helper()
	opts = append([]sonoscloud.Option{sonoscloud.WithSessionStore(store), sonoscloud.WithBaseURL(srv.URL), sonoscloud.WithHTTPClient(srv.Client())}, opts...)
	c, err := sonoscloud.New(opts...)
	require.NoError(t, err)
	return c
}

// validSession is a session of the named parts, each holding secret, that
// runs out in a day.
func validSession(names ...string) sonoscloud.Session {
	var s sonoscloud.Session
	for _, name := range names {
		s.Cookies = append(s.Cookies, sonoscloud.Cookie{Name: name, Value: secret, Expires: time.Now().Add(24 * time.Hour)})
	}
	return s
}

// memStore is a SessionStore in memory, recording what was saved.
type memStore struct {
	mu      sync.Mutex
	session sonoscloud.Session
	cleared bool
}

func (m *memStore) Load() (sonoscloud.Session, error) { return m.saved(), nil }

func (m *memStore) Save(s sonoscloud.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session = s
	return nil
}

func (m *memStore) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session, m.cleared = sonoscloud.Session{}, true
	return nil
}

func (m *memStore) saved() sonoscloud.Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.session
}

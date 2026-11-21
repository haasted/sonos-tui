package sonoscloud_test

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// Case: a client made with no store. It touches no file and says signed out
// without a request.
func TestNew_WithoutAStoreStartsSignedOut(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var requests int
	srv := fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Write([]byte(`{"expires":"2026-11-25T12:00:00.000Z"}`))
	})

	c, err := sonoscloud.New(sonoscloud.WithBaseURL(srv.URL), sonoscloud.WithHTTPClient(srv.Client()))
	require.NoError(t, err)
	_, err = c.Session(t.Context())

	assert.ErrorIs(t, err, sonoscloud.ErrSignedOut)
	assert.Zero(t, requests)
	entries, err := os.ReadDir(home)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

// Case: a session whose parts run out at different times. It is valid until
// the first part runs out.
func TestSession_ValidUntilTheFirstPartRunsOut(t *testing.T) {
	now := time.Now()
	s := sonoscloud.Session{Cookies: []sonoscloud.Cookie{
		{Name: sessionName + ".0", Expires: now.Add(2 * time.Hour)},
		{Name: sessionName + ".1", Expires: now.Add(time.Hour)},
	}}

	assert.Equal(t, now.Add(time.Hour), s.Expires())
	assert.True(t, s.Valid(now))
	assert.False(t, s.Valid(now.Add(time.Hour)))
	assert.False(t, sonoscloud.Session{}.Valid(now))
}

// Case: a session saved with no expiry, as the experiment saved it. It is
// left to the server to judge, not taken as run out.
func TestSession_NoExpiryIsLeftToTheServer(t *testing.T) {
	s := sonoscloud.Session{Cookies: []sonoscloud.Cookie{{Name: sessionName, Value: secret}}}

	assert.True(t, s.Valid(time.Now()))
	assert.True(t, s.Expires().IsZero())
}

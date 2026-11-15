package sonoscloud_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// Case: a session saved and loaded again. It comes back whole, in a file
// that only the reader can read, in a folder made for it.
func TestFileStore_SavesAndLoadsTheSession(t *testing.T) {
	store := sonoscloud.FileStore{Path: filepath.Join(t.TempDir(), "sonos-tui", "web-session")}
	session := validSession(sessionName+".0", sessionName+".1")
	for i := range session.Cookies {
		session.Cookies[i].Expires = session.Cookies[i].Expires.Truncate(time.Second).UTC()
	}

	require.NoError(t, store.Save(session))
	loaded, err := store.Load()
	require.NoError(t, err)

	assert.Equal(t, session, loaded)
	info, err := os.Stat(store.Path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// Case: no file, and a file cleared. Both load as no session, and clearing
// a file that is not there is no error.
func TestFileStore_NoFileIsNoSession(t *testing.T) {
	store := sonoscloud.FileStore{Path: filepath.Join(t.TempDir(), "web-session")}

	loaded, err := store.Load()
	require.NoError(t, err)
	assert.Empty(t, loaded.Cookies)

	require.NoError(t, store.Save(validSession(sessionName)))
	require.NoError(t, store.Clear())
	require.NoError(t, store.Clear())
	loaded, err = store.Load()
	require.NoError(t, err)
	assert.Empty(t, loaded.Cookies)
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

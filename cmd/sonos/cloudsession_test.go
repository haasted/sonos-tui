package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
	"github.com/haasted/sonos-tui/internal/store"
)

// madeUpCookie is a cookie value for tests only; no real session is in them.
const madeUpCookie = "made-up-cookie-value"

// Case: a session saved and loaded again. It comes back whole, and the
// setting holds the cookies as JSON, the format the sign-in writes.
func TestCloudSession_SavesAndLoadsTheSession(t *testing.T) {
	db := openTestStore(t)
	sessions := cloudSession{ctx: t.Context(), settings: db.Settings()}
	expires := time.Now().Add(24 * time.Hour).Truncate(time.Second).UTC()
	session := sonoscloud.Session{Cookies: []sonoscloud.Cookie{
		{Name: "part.0", Value: madeUpCookie, Expires: expires},
		{Name: "part.1", Value: madeUpCookie, Expires: expires},
	}}

	require.NoError(t, sessions.Save(session))
	loaded, err := sessions.Load()
	require.NoError(t, err)
	assert.Equal(t, session, loaded)

	stored, err := db.Settings().Get(t.Context(), store.SettingCloudSession, "")
	require.NoError(t, err)
	want, err := json.Marshal(session.Cookies)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), stored)
}

// Case: no session saved, and a session cleared. Both load as no session,
// and clearing when there is none is no error.
func TestCloudSession_NoSettingIsNoSession(t *testing.T) {
	sessions := cloudSession{ctx: t.Context(), settings: openTestStore(t).Settings()}

	loaded, err := sessions.Load()
	require.NoError(t, err)
	assert.Empty(t, loaded.Cookies)

	require.NoError(t, sessions.Save(sonoscloud.Session{Cookies: []sonoscloud.Cookie{{Name: "part", Value: madeUpCookie}}}))
	require.NoError(t, sessions.Clear())
	require.NoError(t, sessions.Clear())
	loaded, err = sessions.Load()
	require.NoError(t, err)
	assert.Empty(t, loaded.Cookies)
}

// Case: a saved value that is not a session. Loading it fails, and the error
// holds no part of the value.
func TestCloudSession_ABadValueGivesAnErrorWithoutIt(t *testing.T) {
	db := openTestStore(t)
	require.NoError(t, db.Settings().Set(t.Context(), store.SettingCloudSession, `{"value":"`+madeUpCookie))

	_, err := cloudSession{ctx: t.Context(), settings: db.Settings()}.Load()

	require.Error(t, err)
	assert.NotContains(t, err.Error(), madeUpCookie)
}

// Case: the client made as the app makes it. It starts with the session the
// database holds, so it is signed in.
func TestOpenCloud_UsesTheSessionInTheDatabase(t *testing.T) {
	db := openTestStore(t)
	session := sonoscloud.Session{Cookies: []sonoscloud.Cookie{{Name: "part", Value: madeUpCookie}}}
	require.NoError(t, cloudSession{ctx: t.Context(), settings: db.Settings()}.Save(session))

	client := openCloud(t.Context(), db)

	require.NotNil(t, client)
	assert.True(t, client.SignedIn())
}

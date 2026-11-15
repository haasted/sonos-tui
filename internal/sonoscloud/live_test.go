package sonoscloud_test

import (
	"errors"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// live gates tests that reach the real Sonos cloud with the reader's saved
// session. Run with: go test ./internal/sonoscloud/... -live -v
var live = flag.Bool("live", false, "run tests against the real Sonos cloud")

// Case: the saved session, against the real web app. The server takes it,
// and it runs out in the future. The renewal it gets is saved back.
func TestLive_TheSavedSessionIsValid(t *testing.T) {
	c := liveClient(t)

	info, err := c.Session(t.Context())

	require.NoError(t, err)
	require.True(t, info.Expires.After(time.Now()))
	t.Logf("the session runs out at %s", info.Expires.Format(time.RFC3339))
}

// Case: the account's households, from the real Control API. There is at
// least one, and each id is the speakers' form, a dot and more. The ids are
// not logged.
func TestLive_TheHouseholdsAreListed(t *testing.T) {
	c := liveClient(t)

	households, err := c.Households(t.Context())

	require.NoError(t, err)
	require.NotEmpty(t, households)
	for _, h := range households {
		require.True(t, strings.HasPrefix(h.ID, "Sonos_"), "an id not of the Sonos form")
		require.NotEqual(t, h.ID, h.LocalID(), "an id with no dot")
		require.NotEmpty(t, h.OwnerLUID)
	}
	t.Logf("the account holds %d household(s)", len(households))
}

// liveClient is a client with the reader's saved session. It skips the test
// without -live, and when no valid session is saved or the server no longer
// takes it: a live test never signs in, as that needs the password.
func liveClient(t *testing.T) *sonoscloud.Client {
	t.Helper()
	if !*live {
		t.Skip("needs -live")
	}
	logRequestsTo(t)
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	store := sonoscloud.FileStore{Path: filepath.Join(home, ".config", "sonos-tui", "web-session")}
	saved, err := store.Load()
	require.NoError(t, err)
	if !saved.Valid(time.Now()) {
		t.Skip("no valid session saved; sign in first")
	}
	c, err := sonoscloud.New(store)
	require.NoError(t, err)
	if _, err := c.Session(t.Context()); errors.Is(err, sonoscloud.ErrSignedOut) {
		t.Skip("the server no longer takes the saved session; sign in first")
	}
	return c
}

// logRequestsTo shows the package's debug log in the test's output for the
// test's length, so a live run shows each real request and its answer.
func logRequestsTo(t *testing.T) {
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(testWriter{t}, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
}

// testWriter writes each log line to the test's own output.
type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}

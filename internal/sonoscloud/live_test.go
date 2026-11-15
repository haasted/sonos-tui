package sonoscloud_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// live gates tests that reach the real Sonos cloud with the reader's saved
// session. Run with: go test ./internal/sonoscloud/... -live -v
var live = flag.Bool("live", false, "run tests against the real Sonos cloud")

// Case: the session the experiment saved, against the real web app. It is
// valid and runs out in the future. The renewal it gets is saved back.
func TestLive_TheSavedSessionIsValid(t *testing.T) {
	if !*live {
		t.Skip("needs -live and a saved session")
	}
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	store := sonoscloud.FileStore{Path: filepath.Join(home, ".config", "sonos-tui", "web-session")}
	c, err := sonoscloud.New(store)
	require.NoError(t, err)

	info, err := c.Session(t.Context())

	require.NoError(t, err, "sign in with make sonos-web first")
	require.True(t, info.Expires.After(time.Now()))
	t.Logf("the session runs out at %s", info.Expires.Format(time.RFC3339))
}

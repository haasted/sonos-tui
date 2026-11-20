package sonoscloud_test

import (
	"errors"
	"flag"
	"log/slog"
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

// Case: a search of Apple Music (service 52231) in the real household. It
// finds David Bowie's albums, and an album's summary gives its year.
func TestLive_SearchFindsAlbums(t *testing.T) {
	c := liveClient(t)
	households, err := c.Households(t.Context())
	require.NoError(t, err)
	household, err := sonoscloud.PickHousehold(households, "")
	if errors.Is(err, sonoscloud.ErrHouseholdUnknown) {
		t.Skip("the account holds more than one household")
	}

	results, err := c.Search(t.Context(), household.ID, 52231, "david bowie")

	require.NoError(t, err)
	var albumYear bool
	for _, g := range results.Groups {
		t.Logf("%s: %d", g.Name, len(g.Results))
		for _, r := range g.Results {
			if _, _, ok := r.GenreYear(); ok {
				albumYear = true
			}
		}
	}
	require.True(t, albumYear, "no album with a genre and a year")
}

// Case: an album and an artist opened in the real household. The album has
// its tracks; the artist's albums go past its page's own, up to 200.
func TestLive_OpenAlbumAndArtist(t *testing.T) {
	c := liveClient(t)
	households, err := c.Households(t.Context())
	require.NoError(t, err)
	household, err := sonoscloud.PickHousehold(households, "")
	if errors.Is(err, sonoscloud.ErrHouseholdUnknown) {
		t.Skip("the account holds more than one household")
	}

	album, err := c.OpenAlbum(t.Context(), household.ID, 52231, "album:1676286487")
	require.NoError(t, err)
	require.NotEmpty(t, album.Tracks)
	_, year, _ := album.GenreYear()
	t.Logf("%s by %s (%s): %d of %d tracks", album.Name, album.Artist, year, len(album.Tracks), album.Total)

	artist, err := c.OpenArtist(t.Context(), household.ID, 52231, "artist:551695")
	require.NoError(t, err)
	require.NotNil(t, artist.Station)
	require.Greater(t, len(artist.Albums), 100, "the rest of the list was not fetched")
	require.LessOrEqual(t, len(artist.Albums), 200)
	t.Logf("%s: %d of %d albums, station %q", artist.Name, len(artist.Albums), artist.Total, artist.Station.Name)
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
	path, err := sonoscloud.DefaultSessionPath()
	require.NoError(t, err)
	store := sonoscloud.FileStore{Path: path}
	saved, err := store.Load()
	require.NoError(t, err)
	if !saved.Valid(time.Now()) {
		t.Skip("no valid session saved; sign in first")
	}
	c, err := sonoscloud.New(sonoscloud.WithSessionStore(store))
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

package sonoscloud_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// Case: a search of one service. The request names the household, the
// service and the account, and asks for 50 results. The answer, a real one
// cut down, comes back grouped by type in the web app's order.
func TestSearch_AsksTheServiceAndGroupsTheResults(t *testing.T) {
	var got *http.Request
	srv := fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(r.Context())
		w.Write(searchFixture(t))
	})

	results, err := newClient(t, srv, &memStore{session: validSession(sessionName)}).
		Search(t.Context(), "Sonos_home.cloud", 52231, 2, "david bowie")

	require.NoError(t, err)
	assert.Equal(t, "/api/content/v1/households/Sonos_home.cloud/services/52231/accounts/2/search", got.URL.Path)
	assert.Equal(t, "david bowie", got.URL.Query().Get("query"))
	assert.Equal(t, "50", got.URL.Query().Get("count"))
	var names []string
	for _, g := range results.Groups {
		names = append(names, g.Name)
	}
	assert.Equal(t, []string{"ARTISTS", "TRACKS", "ALBUMS", "PLAYLISTS", "PROGRAMS"}, names)
}

// Case: the fields of each type of result, read from the real answer.
func TestSearch_ReadsEachTypeOfResult(t *testing.T) {
	results := searchOf(t, searchFixture(t))

	artist := results.Groups[0].Results[0]
	assert.Equal(t, sonoscloud.TypeArtist, artist.Type)
	assert.Equal(t, "artist:551695", artist.ID.ObjectID)
	assert.Equal(t, "2", artist.ID.AccountID)
	assert.False(t, artist.Playable)
	assert.NotEmpty(t, artist.ImageURL)

	track := results.Groups[1].Results[0]
	assert.Equal(t, "Starman (2012 Remaster)", track.Name)
	assert.Equal(t, []sonoscloud.Ref{{ID: artist.ID, Name: "David Bowie"}}, track.Artists)
	require.NotNil(t, track.Album)
	assert.Equal(t, "album:1039796877", track.Album.ID.ObjectID)
	assert.Equal(t, 4, track.Ordinal)
	assert.Equal(t, 254*time.Second, track.Duration)

	album := results.Groups[2].Results[0]
	assert.Equal(t, "Blackstar", album.Name)
	assert.True(t, album.Explicit)
	assert.Equal(t, "Rock - 2016", album.Summary)

	playlist := results.Groups[3].Results[0]
	assert.Equal(t, "Playlist by Apple Music Rock", playlist.Owner)

	station := results.Groups[4].Results[0]
	assert.Equal(t, sonoscloud.TypeProgram, station.Type)
	assert.Equal(t, "radio:ra.551695", station.ID.ObjectID)
}

// Case: an album's summary, and summaries that are not an album's genre and
// year. Only an album's "Genre - Year" gives both.
func TestResult_GenreYear(t *testing.T) {
	for name, tc := range map[string]struct {
		result sonoscloud.Result
		genre  string
		year   int
		ok     bool
	}{
		"an album":                  {sonoscloud.Result{Type: sonoscloud.TypeAlbum, Summary: "Rock - 2016"}, "Rock", 2016, true},
		"a genre with a dash in it": {sonoscloud.Result{Type: sonoscloud.TypeAlbum, Summary: "Singer - Songwriter - 1971"}, "Singer - Songwriter", 1971, true},
		"an album with no year":     {sonoscloud.Result{Type: sonoscloud.TypeAlbum, Summary: "Rock"}, "", 0, false},
		"a playlist":                {sonoscloud.Result{Type: sonoscloud.TypePlaylist, Summary: "Hits - 2024"}, "", 0, false},
	} {
		genre, year, ok := tc.result.GenreYear()
		assert.Equal(t, tc.ok, ok, name)
		assert.Equal(t, tc.genre, genre, name)
		assert.Equal(t, tc.year, year, name)
	}
}

// Case: a group of a type not seen before. Its results come back with their
// own type, not dropped.
func TestSearch_KeepsATypeNotSeenBefore(t *testing.T) {
	answer := `{"resourceOrder":["PODCASTS"],"PODCASTS":{"resources":[
		{"id":{"objectId":"podcast:1"},"name":"A podcast","type":"PODCAST"}]}}`

	results := searchOf(t, []byte(answer))

	require.Len(t, results.Groups, 1)
	assert.Equal(t, sonoscloud.ResultType("PODCAST"), results.Groups[0].Results[0].Type)
}

// Case: a household the cloud does not know, which it answers 410 Gone. The
// search says the household is wrong, so the caller can ask again.
func TestSearch_AWrongHousehold(t *testing.T) {
	srv := fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	})

	_, err := newClient(t, srv, &memStore{session: validSession(sessionName)}).
		Search(t.Context(), "Sonos_elsewhere.cloud", 52231, 2, "david bowie")

	assert.ErrorIs(t, err, sonoscloud.ErrWrongHousehold)
}

// Case: the API's version header on an answer. It goes into the request's
// debug line, so a change shows on the request where it happens.
func TestSearch_LogsTheAPIVersion(t *testing.T) {
	var log bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	srv := fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Sonos-Api-Version", "0.91.3")
		w.Write(searchFixture(t))
	})

	_, err := newClient(t, srv, &memStore{session: validSession(sessionName)}).
		Search(t.Context(), "Sonos_home.cloud", 52231, 2, "david bowie")

	require.NoError(t, err)
	assert.Contains(t, log.String(), "status=200 apiVersion=0.91.3")
}

// searchOf runs one search against a fake web app that answers body.
func searchOf(t *testing.T, body []byte) sonoscloud.SearchResults {
	t.Helper()
	srv := fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) { w.Write(body) })
	results, err := newClient(t, srv, &memStore{session: validSession(sessionName)}).
		Search(t.Context(), "Sonos_home.cloud", 52231, 2, "david bowie")
	require.NoError(t, err)
	return results
}

// searchFixture is a real search for "david bowie" on Apple Music, cut down
// to two results of each type.
func searchFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/search_david_bowie.json")
	require.NoError(t, err)
	return data
}

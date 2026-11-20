package sonoscloud_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// Case: an album opened. The request names the id escaped twice, as the web
// app does, and the real page comes back as the album and its tracks, with
// what only the defaults hold (the summary, the audio format) filled in.
func TestOpenAlbum_ReadsTheAlbumAndItsTracks(t *testing.T) {
	f := newFakeBrowse(t)

	album, err := f.client(t).OpenAlbum(t.Context(), "Sonos_home.cloud", 52231, "album:1676286487")

	require.NoError(t, err)
	assert.Equal(t, "/api/content/v2/households/Sonos_home.cloud/services/52231/accounts/2/albums/album%253A1676286487/browse", f.paths[0])
	assert.Equal(t, "count=50&muse2=true", f.queries[0])
	assert.Equal(t, "album:1676286487", album.ID.ObjectID)
	assert.Equal(t, "Blackstar", album.Name)
	assert.Equal(t, "David Bowie", album.Artist)
	genre, year, ok := album.GenreYear()
	assert.True(t, ok)
	assert.Equal(t, "Rock", genre)
	assert.Equal(t, "2016", year)
	assert.Equal(t, 7, album.Total)
	require.Len(t, album.Tracks, 3)

	track := album.Tracks[0]
	assert.Equal(t, "song:1676286495", track.ID.ObjectID)
	assert.Equal(t, "★", track.Name)
	assert.Equal(t, 1, track.Ordinal)
	assert.Equal(t, 597*time.Second, track.Duration)
	assert.True(t, track.Playable)
	assert.Equal(t, "audio/aac", track.MimeType)
	assert.Equal(t, []sonoscloud.Ref{{
		ID: sonoscloud.ResultID{ObjectID: "artist:551695", ServiceID: "52231", AccountID: "2"}, Name: "David Bowie",
	}}, track.Artists)
}

// Case: a track the account cannot play, which has no ACTION_PLAY, as a
// store-only release's tracks have not.
func TestOpenAlbum_ATrackThatCannotBePlayed(t *testing.T) {
	f := newFakeBrowse(t)
	f.album = []byte(`{"type":"TEMPLATE_ALBUM_PAGE","title":"Triplicate","resource":{"id":{"objectId":"album%3A1"}},
		"tracks":{"total":1,"items":[{"title":"I Guess I'll Have to Change My Plan","actions":["ACTION_FAVORITES"],
		"resource":{"id":{"objectId":"song%3A2"}},"ordinal":1,"duration":"200"}]}}`)

	album, err := f.client(t).OpenAlbum(t.Context(), "Sonos_home.cloud", 52231, "album:1")

	require.NoError(t, err)
	assert.False(t, album.Tracks[0].Playable)
}

// Case: an artist opened, from the real page: its station, and its albums
// with their official artist line, and their year from the defaults.
func TestOpenArtist_ReadsTheArtistAndItsAlbums(t *testing.T) {
	f := newFakeBrowse(t)
	f.containerPages = 0 // the list stops after the page's own entries

	artist, err := f.client(t).OpenArtist(t.Context(), "Sonos_home.cloud", 52231, "artist:551695")

	require.NoError(t, err)
	assert.Equal(t, "/api/content/v2/households/Sonos_home.cloud/services/52231/accounts/2/artists/artist%253A551695/browse", f.paths[0])
	assert.Equal(t, "muse2=true", f.queries[0])
	assert.Equal(t, "David Bowie", artist.Name)
	require.NotNil(t, artist.Station)
	assert.Equal(t, "radio:ra.551695", artist.Station.ID.ObjectID)
	assert.Equal(t, "David Bowie Radio", artist.Station.Name)
	assert.Equal(t, 187, artist.Total)

	first := artist.Albums[0]
	assert.Equal(t, "album:1440880284", first.ID.ObjectID)
	assert.Equal(t, "David Bowie & Trevor Jones", first.Artist)
	assert.Equal(t, sonoscloud.KindAlbum, first.Kind)
	assert.True(t, first.Playable)
	_, year, ok := first.GenreYear()
	assert.True(t, ok)
	assert.Equal(t, "1986", year)
}

// Case: an artist with more albums than its page holds. The rest come from
// its container 100 at a time, from offset 100, and stop at 200 albums.
func TestOpenArtist_FetchesTheRestUpTo200(t *testing.T) {
	f := newFakeBrowse(t)
	f.containerPages = 5 // far more than the cap

	artist, err := f.client(t).OpenArtist(t.Context(), "Sonos_home.cloud", 52231, "artist:551695")

	require.NoError(t, err)
	assert.Len(t, artist.Albums, 200)
	assert.Equal(t, []string{"offset=100", "offset=200"}, f.offsets())
	assert.Contains(t, f.queries[1], "count=100")
	assert.Contains(t, f.queries[1], "filterExplicit=false")
	assert.Equal(t, "/api/content/v2/households/Sonos_home.cloud/services/52231/accounts/2/containers/artist%253A551695/resources", f.paths[1])
}

// Case: the container's last page has no nextUri. The list stops there,
// short of the cap.
func TestOpenArtist_StopsAtTheLastPage(t *testing.T) {
	f := newFakeBrowse(t)
	f.containerPages = 1

	artist, err := f.client(t).OpenArtist(t.Context(), "Sonos_home.cloud", 52231, "artist:551695")

	require.NoError(t, err)
	assert.Len(t, artist.Albums, 3+100)
	assert.Equal(t, []string{"offset=100"}, f.offsets())
}

// Case: the kinds Apple Music marks at the end of a title. Anything else is
// an album.
func TestOpenArtist_AlbumKinds(t *testing.T) {
	f := newFakeBrowse(t)
	f.artist = artistPage("Changes - Single", "Live Santa Monica '72", "The Man Who Sold the World - EP")

	artist, err := f.client(t).OpenArtist(t.Context(), "Sonos_home.cloud", 52231, "artist:551695")

	require.NoError(t, err)
	var kinds []sonoscloud.AlbumKind
	for _, a := range artist.Albums {
		kinds = append(kinds, a.Kind)
	}
	assert.Equal(t, []sonoscloud.AlbumKind{sonoscloud.KindSingle, sonoscloud.KindAlbum, sonoscloud.KindEP}, kinds)
}

// Case: a household the cloud does not know. Opening says so, as a search
// does.
func TestOpenAlbum_AWrongHousehold(t *testing.T) {
	f := newFakeBrowse(t)
	f.gone = true

	_, err := f.client(t).OpenAlbum(t.Context(), "Sonos_elsewhere.cloud", 52231, "album:1676286487")

	assert.ErrorIs(t, err, sonoscloud.ErrWrongHousehold)
}

// fakeBrowse is the web app answering album, artist and container requests.
type fakeBrowse struct {
	srv            *httptest.Server
	album, artist  []byte
	containerPages int  // how many container pages have entries, each with a nextUri but the last
	gone           bool // answer every request 410

	mu      sync.Mutex
	paths   []string
	queries []string
}

func newFakeBrowse(t *testing.T) *fakeBrowse {
	t.Helper()
	f := &fakeBrowse{album: readFixture(t, "album_blackstar.json"), artist: readFixture(t, "artist_bowie.json")}
	f.srv = fakeWebApp(t, func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.EscapedPath())
		f.queries = append(f.queries, r.URL.RawQuery)
		f.mu.Unlock()
		switch {
		case f.gone:
			w.WriteHeader(http.StatusGone)
		case strings.Contains(r.URL.EscapedPath(), "/albums/"):
			w.Write(f.album)
		case strings.Contains(r.URL.EscapedPath(), "/artists/"):
			w.Write(f.artist)
		case strings.Contains(r.URL.EscapedPath(), "/containers/"):
			w.Write(f.containerPage(r.URL.Query().Get("offset")))
		default:
			http.NotFound(w, r)
		}
	})
	return f
}

// client is a client of the fake with a valid saved session, knowing the
// household's Apple Music account.
func (f *fakeBrowse) client(t *testing.T) *sonoscloud.Client {
	t.Helper()
	return newClient(t, f.srv, &memStore{session: validSession(sessionName)}, sonoscloud.WithAccount(sonoscloud.AppleMusic, 2))
}

// containerPage is 100 made-up albums from offset, with a nextUri unless it
// is the last page that has entries; past those pages it is empty.
func (f *fakeBrowse) containerPage(offset string) []byte {
	var n int
	fmt.Sscan(offset, &n)
	page := n / 100
	if page > f.containerPages {
		return []byte(`{"items":[],"total":187}`)
	}
	var items []string
	for i := range 100 {
		items = append(items, fmt.Sprintf(`{"title":"Album %d","actions":["ACTION_PLAY"],"resource":{"type":"ALBUM","id":{"objectId":"album%%3A%d"}}}`, n+i, n+i))
	}
	next := ""
	if page < f.containerPages {
		next = `,"nextUri":"next"`
	}
	return []byte(`{"items":[` + strings.Join(items, ",") + `],"total":187` + next + `}`)
}

// offsets are the offsets the container was asked for, in order.
func (f *fakeBrowse) offsets() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var got []string
	for _, q := range f.queries {
		for _, part := range strings.Split(q, "&") {
			if strings.HasPrefix(part, "offset=") {
				got = append(got, part)
			}
		}
	}
	return got
}

// artistPage is an artist page whose one section holds albums of the titles,
// and nothing more.
func artistPage(titles ...string) []byte {
	type entry struct {
		Title    string   `json:"title"`
		Actions  []string `json:"actions"`
		Resource struct {
			Type string `json:"type"`
		} `json:"resource"`
	}
	var entries []entry
	for _, title := range titles {
		e := entry{Title: title, Actions: []string{"ACTION_PLAY"}}
		e.Resource.Type = "ALBUM"
		entries = append(entries, e)
	}
	page := map[string]any{"type": "TEMPLATE_PERSON_PAGE", "title": "David Bowie",
		"sections": map[string]any{"items": []any{map[string]any{"items": entries, "total": len(entries)}}}}
	data, _ := json.Marshal(page)
	return data
}

// readFixture is a real answer, cut down and with its private ids replaced.
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return data
}

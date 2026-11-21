package sonoscloud_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
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
	longAlbum      int  // the tracks a made-up long album holds; 0 serves the Blackstar fixture

	mu      sync.Mutex
	paths   []string
	queries []string
}

// Case: an album of 294 tracks opened. Its first 150 are read: the page
// asks for 50 as the web app does, then the track list from offset 0, where
// offsets start on a multiple of 100.
func TestOpenAlbum_ALongAlbumStopsAt150(t *testing.T) {
	f := newFakeBrowse(t)
	f.longAlbum = 294

	album, err := f.client(t).OpenAlbum(t.Context(), "Sonos_home.cloud", 52231, "album:900")

	require.NoError(t, err)
	assert.Equal(t, 294, album.Total)
	require.Len(t, album.Tracks, 150)
	assert.Equal(t, 1, album.Tracks[0].Ordinal)
	assert.Equal(t, 150, album.Tracks[149].Ordinal)
	assert.Equal(t, []string{"0+100", "100+50"}, f.tracksAsked())
}

// Case: the rest of that album read. Every track comes once and in order,
// though offset 150 is not a multiple of 100, and nothing is asked at or past
// the end. Opening it again gives the whole album without a request.
func TestRestOfAlbum_ReadsEveryTrackOnce(t *testing.T) {
	f := newFakeBrowse(t)
	f.longAlbum = 294
	c := f.client(t)
	album, err := c.OpenAlbum(t.Context(), "Sonos_home.cloud", 52231, "album:900")
	require.NoError(t, err)

	whole, err := c.RestOfAlbum(t.Context(), "Sonos_home.cloud", 52231, album)

	require.NoError(t, err)
	require.Len(t, whole.Tracks, 294)
	for i, track := range whole.Tracks {
		require.Equal(t, i+1, track.Ordinal, "track %d", i)
	}
	assert.Equal(t, []string{"0+100", "100+50", "100+100", "200+94"}, f.tracksAsked())

	asked := len(f.paths)
	again, err := c.OpenAlbum(t.Context(), "Sonos_home.cloud", 52231, "album:900")
	require.NoError(t, err)
	assert.Len(t, again.Tracks, 294)
	assert.Len(t, f.paths, asked, "the whole album was not kept")
}

// Case: an album longer than maxAlbumTracks. The rest stops at 1000 tracks.
func TestRestOfAlbum_StopsAt1000(t *testing.T) {
	f := newFakeBrowse(t)
	f.longAlbum = 1500
	c := f.client(t)
	album, err := c.OpenAlbum(t.Context(), "Sonos_home.cloud", 52231, "album:900")
	require.NoError(t, err)

	whole, err := c.RestOfAlbum(t.Context(), "Sonos_home.cloud", 52231, album)

	require.NoError(t, err)
	assert.Len(t, whole.Tracks, 1000)
	assert.Equal(t, 1500, whole.Total)
}

// Case: an artist opened, again at once, and again once pageLife has passed.
// The second open asks nothing and gives the same artist; the third asks.
func TestOpenArtist_IsKeptForAWhile(t *testing.T) {
	f := newFakeBrowse(t)
	f.containerPages = 0
	now := time.Now()
	c := f.clientAt(t, func() time.Time { return now })

	first, err := c.OpenArtist(t.Context(), "Sonos_home.cloud", 52231, "artist:551695")
	require.NoError(t, err)
	second, err := c.OpenArtist(t.Context(), "Sonos_home.cloud", 52231, "artist:551695")
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, 1, f.pagesAsked(), "the second open asked again")

	now = now.Add(31 * time.Minute)
	_, err = c.OpenArtist(t.Context(), "Sonos_home.cloud", 52231, "artist:551695")
	require.NoError(t, err)
	assert.Equal(t, 2, f.pagesAsked(), "an artist older than pageLife was given again")
}

// Case: an album opened twice. The second open asks nothing.
func TestOpenAlbum_IsKeptForAWhile(t *testing.T) {
	f := newFakeBrowse(t)
	c := f.client(t)

	_, err := c.OpenAlbum(t.Context(), "Sonos_home.cloud", 52231, "album:1676286487")
	require.NoError(t, err)
	_, err = c.OpenAlbum(t.Context(), "Sonos_home.cloud", 52231, "album:1676286487")
	require.NoError(t, err)

	assert.Equal(t, 1, f.pagesAsked())
}

// Case: an open that fails, then succeeds. The failure is not kept, so the
// second open asks again.
func TestOpenArtist_AFailureIsNotKept(t *testing.T) {
	f := newFakeBrowse(t)
	f.containerPages = 0
	c := f.client(t)

	f.gone = true
	_, err := c.OpenArtist(t.Context(), "Sonos_home.cloud", 52231, "artist:551695")
	require.Error(t, err)
	f.gone = false
	_, err = c.OpenArtist(t.Context(), "Sonos_home.cloud", 52231, "artist:551695")

	require.NoError(t, err)
	assert.Equal(t, 2, f.pagesAsked())
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
		case strings.HasSuffix(r.URL.EscapedPath(), "/tracks"):
			f.writeTracks(w, r.URL.Query())
		case strings.Contains(r.URL.EscapedPath(), "/albums/") && f.longAlbum > 0:
			w.Write(f.longAlbumPage())
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

// pagesAsked counts the requests for an artist's or an album's page, leaving
// out the requests for the rest of its list.
func (f *fakeBrowse) pagesAsked() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, path := range f.paths {
		if strings.HasSuffix(path, "/browse") {
			n++
		}
	}
	return n
}

// clientAt is client, reading the time from now.
func (f *fakeBrowse) clientAt(t *testing.T, now func() time.Time) *sonoscloud.Client {
	t.Helper()
	return newClient(t, f.srv, &memStore{session: validSession(sessionName)},
		sonoscloud.WithAccount(sonoscloud.AppleMusic, 2), sonoscloud.WithClock(now))
}

// writeTracks answers an album's track list as the API does: from offset
// rounded up to a multiple of 100, at most count and 100 tracks, and a 500
// at or past the end. The fixture's album gives its own tracks.
func (f *fakeBrowse) writeTracks(w http.ResponseWriter, query url.Values) {
	offset, _ := strconv.Atoi(query.Get("offset"))
	count, _ := strconv.Atoi(query.Get("count"))
	if f.longAlbum == 0 {
		var page struct {
			Tracks json.RawMessage `json:"tracks"`
		}
		json.Unmarshal(f.album, &page)
		w.Write(page.Tracks)
		return
	}
	from := (offset + 99) / 100 * 100
	if from >= f.longAlbum {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(madeUpTracks(from, min(from+min(count, 100), f.longAlbum), f.longAlbum))
}

// longAlbumPage is the page of the made-up long album: its first 50 tracks.
func (f *fakeBrowse) longAlbumPage() []byte {
	return []byte(`{"title":"Complete Edition","resource":{"type":"ALBUM","id":{"objectId":"album%3A900","serviceId":"52231","accountId":"2"}},` +
		`"tracks":` + string(madeUpTracks(0, 50, f.longAlbum)) + `}`)
}

// madeUpTracks is a track list of tracks from up to to, of an album of total.
func madeUpTracks(from, to, total int) []byte {
	var items []string
	for n := from; n < to; n++ {
		items = append(items, fmt.Sprintf(`{"title":"Track %d","ordinal":%d,"actions":["ACTION_PLAY"],`+
			`"resource":{"type":"TRACK","id":{"objectId":"song%%3A%d","serviceId":"52231","accountId":"2"}}}`, n+1, n+1, n+1))
	}
	return []byte(fmt.Sprintf(`{"items":[%s],"total":%d}`, strings.Join(items, ","), total))
}

// tracksAsked is the offset and count of each request for the track list.
func (f *fakeBrowse) tracksAsked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var asked []string
	for i, path := range f.paths {
		if strings.HasSuffix(path, "/tracks") {
			q, _ := url.ParseQuery(f.queries[i])
			asked = append(asked, q.Get("offset")+"+"+q.Get("count"))
		}
	}
	return asked
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

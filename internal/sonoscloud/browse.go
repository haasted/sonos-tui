package sonoscloud

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	// maxEntries is the most entries a list is fetched to, so a huge artist
	// does not turn into a flood of requests.
	maxEntries = 200

	// pageSize is the most entries the API sends for one request, and the
	// step its offsets go in: it rounds an offset up to a multiple of it.
	pageSize = 100

	// albumCount is how many tracks an album page asks for, as the web app does.
	albumCount = "50"
)

// The kinds of album that Apple Music marks in a title.
const (
	KindAlbum  AlbumKind = "album"
	KindSingle AlbumKind = "single"
	KindEP     AlbumKind = "EP"
)

// AlbumKind is what kind of release an album is.
type AlbumKind string

// Album is an album opened: the album and its tracks.
type Album struct {
	ID       ResultID
	Name     string
	Artist   string // the official artist line
	Summary  string // "Genre - Year"
	ImageURL string
	Tracks   []Track
	Total    int // tracks the album holds; more than len(Tracks) when it was cut off
}

// Track is one track of an opened album.
type Track struct {
	ID       ResultID
	Name     string
	Artists  []Ref
	Ordinal  int
	Duration time.Duration
	Playable bool   // the account can play it; a store-only release cannot
	MimeType string // its audio format, such as "audio/aac"
	ImageURL string
}

// Artist is an artist opened: the artist, its station, and its albums in the
// API's order, albums by others that it appears on among them.
type Artist struct {
	ID       ResultID
	Name     string
	ImageURL string
	Station  *Ref // the artist's station, when the service has one
	Albums   []AlbumEntry
	Total    int // albums the API counts, an upper bound: some cannot be played
}

// AlbumEntry is one album in an artist's list.
type AlbumEntry struct {
	ID       ResultID
	Name     string
	Artist   string // the official artist line, such as "David Bowie & Trevor Jones"
	Kind     AlbumKind
	Summary  string // "Genre - Year"
	ImageURL string
	Playable bool
}

// wirePage is an opened page as the content API writes it.
type wirePage struct {
	Type          string        `json:"type"`
	Title         string        `json:"title"`
	Subtitle      string        `json:"subtitle"`
	Resource      wireResource  `json:"resource"`
	Images        wireImages    `json:"images"`
	CustomActions []wireAction  `json:"customActions"`
	Tracks        wireEntryList `json:"tracks"`
	Sections      struct {
		Items []wireEntryList `json:"items"`
	} `json:"sections"`
}

// wireEntryList is a list of entries: an album's tracks, or one section of
// an artist's page, or one page of a container.
type wireEntryList struct {
	Items         []wireEntry `json:"items"`
	Total         int         `json:"total"`
	NextURI       string      `json:"nextUri"`
	FastScrollURI string      `json:"fastScrollUri"`
}

// wireEntry is one entry of a page. Its ids are escaped once.
type wireEntry struct {
	Title    string       `json:"title"`
	Subtitle string       `json:"subtitle"`
	Images   wireImages   `json:"images"`
	Actions  []string     `json:"actions"`
	Resource wireResource `json:"resource"`
	Artists  []struct {
		ID   string `json:"id"` // srn:content:audio:artist:artist%3A551695#52231:2
		Name string `json:"name"`
	} `json:"artists"`
	Ordinal  int    `json:"ordinal"`
	Duration string `json:"duration"` // seconds, as text
}

type wireResource struct {
	Type     string   `json:"type"`
	ID       ResultID `json:"id"`
	Defaults string   `json:"defaults"`
}

type wireImages struct {
	Tile string `json:"tile1x1"`
}

type wireAction struct {
	Action   string       `json:"action"`
	Label    string       `json:"label"`
	Resource wireResource `json:"resource"`
}

// OpenAlbum opens an album of a service, such as "album:1676286487", with
// its tracks.
func (c *Client) OpenAlbum(ctx context.Context, household string, service int, objectID string) (Album, error) {
	var page wirePage
	if err := c.getPage(ctx, household, service, "albums/"+pathID(objectID)+"/browse",
		url.Values{"muse2": {"true"}, "count": {albumCount}}, &page); err != nil {
		return Album{}, err
	}

	c.learnAccount(service, page.Resource.ID.AccountID)
	d := decodeDetails(page.Resource.Defaults)
	album := Album{
		ID: unescapedID(page.Resource.ID), Name: cmp.Or(page.Title, d.Name), Artist: cmp.Or(page.Subtitle, d.Artist),
		Summary: d.Summary, ImageURL: cmp.Or(page.Images.Tile, d.ImageURL), Total: page.Tracks.Total,
	}
	checkSummary(album.ID, album.Summary)
	for _, e := range page.Tracks.Items {
		album.Tracks = append(album.Tracks, e.track())
	}
	return album, nil
}

// OpenArtist opens an artist of a service, such as "artist:551695", with its
// station and up to maxEntries of its albums.
func (c *Client) OpenArtist(ctx context.Context, household string, service int, objectID string) (Artist, error) {
	var page wirePage
	if err := c.getPage(ctx, household, service, "artists/"+pathID(objectID)+"/browse",
		url.Values{"muse2": {"true"}}, &page); err != nil {
		return Artist{}, err
	}

	c.learnAccount(service, page.Resource.ID.AccountID)
	artist := Artist{ID: unescapedID(page.Resource.ID), Name: page.Title, ImageURL: page.Images.Tile}
	for _, a := range page.CustomActions {
		if a.Action == "ACTION_PLAY_STATION" {
			artist.Station = &Ref{ID: unescapedID(a.Resource.ID), Name: cmp.Or(decodeDetails(a.Resource.Defaults).Name, a.Label)}
		}
	}
	if len(page.Sections.Items) == 0 {
		return artist, nil
	}

	// The page holds the list's first page; the rest comes from its container.
	section := page.Sections.Items[0]
	artist.Total = section.Total
	entries := section.Items
	if section.NextURI != "" || (section.FastScrollURI != "" && section.Total > len(entries)) {
		rest, err := c.restOfList(ctx, household, service, objectID, len(entries))
		if err != nil {
			return Artist{}, err
		}
		entries = append(entries, rest...)
	}
	for _, e := range entries[:min(len(entries), maxEntries)] {
		artist.Albums = append(artist.Albums, e.albumEntry())
	}
	return artist, nil
}

// restOfList fetches a container's entries from offset pageSize on, a page
// at a time, until a page is empty or has nowhere to go, or maxEntries are
// had with the had already in hand.
func (c *Client) restOfList(ctx context.Context, household string, service int, objectID string, had int) ([]wireEntry, error) {
	var rest []wireEntry
	for offset := pageSize; had+len(rest) < maxEntries; offset += pageSize {
		var list wireEntryList
		if err := c.getPage(ctx, household, service, "containers/"+pathID(objectID)+"/resources", url.Values{
			"count": {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(offset)}, "filterExplicit": {"false"}, "muse2": {"true"},
		}, &list); err != nil {
			return nil, err
		}
		rest = append(rest, list.Items...)
		if len(list.Items) == 0 || list.NextURI == "" {
			break
		}
	}
	return rest, nil
}

// getPage gets a page of the household's service and reads it into into. A
// 410 Gone is ErrWrongHousehold, as for a search.
func (c *Client) getPage(ctx context.Context, household string, service int, rest string, query url.Values, into any) error {
	path := fmt.Sprintf("/api/content/v2/households/%s/services/%d/accounts/%d/%s",
		url.PathEscape(household), service, c.account(service), rest)
	body, err := c.get(ctx, path, query)
	var status *StatusError
	if errors.As(err, &status) && status.Status == http.StatusGone {
		return c.wrongHousehold()
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("sonoscloud: reading %s: %w", rest, err)
	}
	return nil
}

// GenreYear reads the album's summary, "Genre - Year".
func (a Album) GenreYear() (genre, year string, ok bool) { return genreYear(a.Summary) }

// GenreYear reads the album's summary, "Genre - Year".
func (e AlbumEntry) GenreYear() (genre, year string, ok bool) { return genreYear(e.Summary) }

func (e wireEntry) track() Track {
	d := decodeDetails(e.Resource.Defaults)
	seconds, _ := strconv.Atoi(e.Duration)
	t := Track{
		ID: unescapedID(e.Resource.ID), Name: cmp.Or(e.Title, d.Name), Ordinal: e.Ordinal,
		Duration: time.Duration(seconds) * time.Second, Playable: slices.Contains(e.Actions, "ACTION_PLAY"),
		MimeType: d.MimeType, ImageURL: cmp.Or(e.Images.Tile, d.ImageURL),
	}
	for _, a := range e.Artists {
		t.Artists = append(t.Artists, Ref{ID: srnID(a.ID), Name: a.Name})
	}
	return t
}

func (e wireEntry) albumEntry() AlbumEntry {
	d := decodeDetails(e.Resource.Defaults)
	name := cmp.Or(e.Title, d.Name)
	entry := AlbumEntry{
		ID: unescapedID(e.Resource.ID), Name: name, Artist: cmp.Or(e.Subtitle, d.Artist), Kind: albumKind(name),
		Summary: d.Summary, ImageURL: cmp.Or(e.Images.Tile, d.ImageURL), Playable: slices.Contains(e.Actions, "ACTION_PLAY"),
	}
	checkSummary(entry.ID, entry.Summary)
	return entry
}

// albumKind reads the kind Apple Music marks at the end of a title.
func albumKind(title string) AlbumKind {
	switch {
	case strings.HasSuffix(title, " - Single"):
		return KindSingle
	case strings.HasSuffix(title, " - EP"):
		return KindEP
	}
	return KindAlbum
}

// pathID is an object id as the path takes it: escaped twice, so that
// "album:1" becomes "album%253A1".
func pathID(objectID string) string { return url.PathEscape(url.QueryEscape(objectID)) }

// unescapedID is a page's id, escaped once there, as a search gives it.
func unescapedID(id ResultID) ResultID {
	if objectID, err := url.QueryUnescape(id.ObjectID); err == nil {
		id.ObjectID = objectID
	}
	return id
}

// srnID reads an id written as srn:content:audio:{type}:{objectId} and then
// #{service}:{account}, with the object id escaped once.
func srnID(srn string) ResultID {
	rest, owner, _ := strings.Cut(srn, "#")
	at := strings.LastIndex(rest, ":")
	objectID, err := url.QueryUnescape(rest[at+1:])
	if err != nil {
		objectID = rest[at+1:]
	}
	service, account, _ := strings.Cut(owner, ":")
	return ResultID{ObjectID: objectID, ServiceID: service, AccountID: account}
}

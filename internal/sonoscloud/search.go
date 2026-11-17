package sonoscloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// searchCount is how many results a search asks for, as the web app does.
// It is the whole answer's count, shared among the types.
const searchCount = "50"

// The result types seen so far. A type not listed here is still answered,
// with its own name.
const (
	TypeArtist   ResultType = "ARTIST"
	TypeAlbum    ResultType = "ALBUM"
	TypeTrack    ResultType = "TRACK"
	TypePlaylist ResultType = "PLAYLIST"
	TypeProgram  ResultType = "PROGRAM" // a station, such as "David Bowie & Similar Artists"
)

// ErrWrongHousehold says the Sonos cloud does not know the household asked
// about; it answers 410 Gone. Ask for the household again.
var ErrWrongHousehold = errors.New("sonoscloud: the Sonos cloud does not know this household")

// ResultType is what a result is: an artist, an album and so on.
type ResultType string

// SearchResults is one search's answer: its results grouped by type, in the
// order the web app shows the groups.
type SearchResults struct {
	Groups []ResultGroup
}

// ResultGroup is the results of one type, such as "ALBUMS".
type ResultGroup struct {
	Name    string
	Results []Result
}

// Result is one thing a search found.
type Result struct {
	ID       ResultID
	Type     ResultType
	Name     string
	Artists  []Ref  // an album's or a track's artists
	Album    *Ref   // a track's album
	Owner    string // a playlist's owner
	Summary  string // an album's "Genre - Year", a playlist's description, a track's artist
	ImageURL string
	Playable bool
	Explicit bool
	Ordinal  int           // a track's number on its album
	Duration time.Duration // a track's length
}

// ResultID names a result in its service. ObjectID is the service's own id,
// such as "album:1676286487"; AccountID is the household's account the
// service answered for.
type ResultID struct {
	ObjectID  string `json:"objectId"`
	ServiceID string `json:"serviceId"`
	AccountID string `json:"accountId"`
}

// Ref is another result a result points to, such as a track's album.
type Ref struct {
	ID   ResultID
	Name string
}

// wireResult is a result as the content API writes it.
type wireResult struct {
	ID         ResultID   `json:"id"`
	Name       string     `json:"name"`
	Type       ResultType `json:"type"`
	Playable   bool       `json:"playable"`
	Explicit   bool       `json:"explicit"`
	Ordinal    int        `json:"ordinal"`
	DurationMs int64      `json:"durationMs"`
	Artists    []wireRef  `json:"artists"`
	Container  *wireRef   `json:"container"`
	Owner      *wireRef   `json:"owner"`
	Summary    struct {
		Content string `json:"content"`
	} `json:"summary"`
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}

type wireRef struct {
	ID   ResultID `json:"id"`
	Name string   `json:"name"`
}

// Search searches one service of the household, such as Apple Music (52231),
// with the account the household has on it. The account in the request is
// not checked; each result's ID says the account the service answered for.
func (c *Client) Search(ctx context.Context, household string, service, account int, query string) (SearchResults, error) {
	path := fmt.Sprintf("/api/content/v1/households/%s/services/%d/accounts/%d/search",
		url.PathEscape(household), service, account)
	body, err := c.get(ctx, path, url.Values{"query": {query}, "count": {searchCount}})
	var status *StatusError
	if errors.As(err, &status) && status.Status == http.StatusGone {
		return SearchResults{}, ErrWrongHousehold
	}
	if err != nil {
		return SearchResults{}, err
	}
	return parseSearch(body)
}

// GenreYear reads an album's summary, "Genre - Year", the only place the
// content API gives its year. It reports false for any other summary.
func (r Result) GenreYear() (genre string, year int, ok bool) {
	at := strings.LastIndex(r.Summary, " - ")
	if r.Type != TypeAlbum || at < 0 {
		return "", 0, false
	}
	year, err := strconv.Atoi(r.Summary[at+3:])
	if err != nil {
		return "", 0, false
	}
	return r.Summary[:at], year, true
}

// parseSearch reads a search's answer: the group names in resourceOrder, and
// each group under its own name.
func parseSearch(body []byte) (SearchResults, error) {
	var answer map[string]json.RawMessage
	if err := json.Unmarshal(body, &answer); err != nil {
		return SearchResults{}, fmt.Errorf("sonoscloud: reading the search: %w", err)
	}
	var order []string
	if err := json.Unmarshal(answer["resourceOrder"], &order); err != nil {
		return SearchResults{}, fmt.Errorf("sonoscloud: reading the search's order: %w", err)
	}

	var results SearchResults
	for _, name := range order {
		var group struct {
			Resources []wireResult `json:"resources"`
		}
		if err := json.Unmarshal(answer[name], &group); err != nil {
			return SearchResults{}, fmt.Errorf("sonoscloud: reading the search's %s: %w", name, err)
		}
		g := ResultGroup{Name: name}
		for _, w := range group.Resources {
			g.Results = append(g.Results, w.result())
		}
		results.Groups = append(results.Groups, g)
	}
	return results, nil
}

func (w wireResult) result() Result {
	r := Result{
		ID: w.ID, Type: w.Type, Name: w.Name, Summary: w.Summary.Content,
		Playable: w.Playable, Explicit: w.Explicit, Ordinal: w.Ordinal,
		Duration: time.Duration(w.DurationMs) * time.Millisecond,
	}
	for _, a := range w.Artists {
		r.Artists = append(r.Artists, Ref(a))
	}
	if w.Container != nil {
		album := Ref(*w.Container)
		r.Album = &album
	}
	if w.Owner != nil {
		r.Owner = w.Owner.Name
	}
	if len(w.Images) > 0 {
		r.ImageURL = w.Images[0].URL
	}
	return r
}

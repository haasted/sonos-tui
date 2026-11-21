package sonoscloud

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// AppleMusic is Apple Music's service type, as the content API's paths and a
// speaker's SA_RINCON52231_ desc name it. A speaker's URI calls it sid=204.
const AppleMusic = 52231

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
// about; it answers 410 Gone. The kept household is forgotten, so the next
// Household asks the cloud again.
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
	Ordinal  int           // a track's number on its album
	Duration time.Duration // a track's length
	MimeType string        // a track's audio format, such as "audio/aac"
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
	Defaults string `json:"defaults"`
}

type wireRef struct {
	ID   ResultID `json:"id"`
	Name string   `json:"name"`
}

// Search searches one service of the household, such as AppleMusic. The
// request names the household's account on it as last seen; the content API
// does not check it, and each result's ID says the real one.
func (c *Client) Search(ctx context.Context, household string, service int, query string) (SearchResults, error) {
	path := fmt.Sprintf("/api/content/v1/households/%s/services/%d/accounts/%d/search",
		url.PathEscape(household), service, c.account(service))
	body, err := c.get(ctx, path, url.Values{"query": {query}, "count": {searchCount}})
	var status *StatusError
	if errors.As(err, &status) && status.Status == http.StatusGone {
		return SearchResults{}, c.wrongHousehold()
	}
	if err != nil {
		return SearchResults{}, err
	}
	results, err := parseSearch(body)
	for _, g := range results.Groups {
		for _, r := range g.Results {
			c.learnAccount(service, r.ID.AccountID)
		}
	}
	return results, err
}

// GenreYear reads an album's summary, "Genre - Year", the only place the
// content API gives its year. It reports false for any other summary.
func (r Result) GenreYear() (genre, year string, ok bool) {
	if r.Type != TypeAlbum {
		return "", "", false
	}
	return genreYear(r.Summary)
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

// result is the result a wire result describes. Its own fields win, and its
// defaults fill in what they leave out.
func (w wireResult) result() Result {
	d := decodeDetails(w.Defaults)
	r := Result{
		ID: w.ID, Type: w.Type, Name: cmp.Or(w.Name, d.Name), Summary: cmp.Or(w.Summary.Content, d.Summary),
		Playable: w.Playable, Ordinal: w.Ordinal, MimeType: d.MimeType,
		Duration: time.Duration(max(w.DurationMs, d.DurationMs)) * time.Millisecond,
	}
	for _, a := range w.Artists {
		r.Artists = append(r.Artists, Ref(a))
	}
	if w.Container != nil {
		album := Ref(*w.Container)
		r.Album = &album
	} else if d.ContainerType == string(TypeAlbum) && d.ContainerID != "" {
		r.Album = &Ref{ID: ResultID{ObjectID: d.ContainerID, ServiceID: w.ID.ServiceID, AccountID: w.ID.AccountID}, Name: d.ContainerName}
	}
	if w.Owner != nil {
		r.Owner = w.Owner.Name
	}
	if len(w.Images) > 0 {
		r.ImageURL = w.Images[0].URL
	}
	r.ImageURL = cmp.Or(r.ImageURL, d.ImageURL)
	if r.Type == TypeAlbum {
		checkSummary(r.ID, r.Summary)
	}
	return r
}

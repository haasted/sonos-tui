package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// dim and plain wrap an id so it reads as a detail beside a result's name.
const (
	dim   = "\x1b[2m"
	plain = "\x1b[0m"
)

// resource is one search result, with the fields the console shows. The
// answer has more; the saved file holds all of it.
type resource struct {
	ID struct {
		ObjectID string `json:"objectId"`
	} `json:"id"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Artists  []named         `json:"artists"`
	Summary  json.RawMessage `json:"summary"` // an object with content, or a string
	Owner    named           `json:"owner"`
	Album    named           `json:"container"` // a track's album
	Millis   int             `json:"durationMs"`
	Defaults string          `json:"defaults"` // base64 JSON, sent along when the result is opened
}

type named struct {
	Name string `json:"name"`
}

// page is what opening a result answers: a template the web app draws, with
// the result's own items, such as an album's tracks.
type page struct {
	Type     string `json:"type"` // the template, such as TEMPLATE_ALBUM_PAGE
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Tracks   struct {
		Items []pageTrack `json:"items"`
		Total int         `json:"total"`
	} `json:"tracks"`
	Sections struct {
		Items []pageSection `json:"items"`
	} `json:"sections"`
}

// pageSection is one group on a page, such as an artist's albums. It holds a
// first page of items; FastScrollURI says the whole list can be had.
type pageSection struct {
	Title         string     `json:"title"`
	Total         int        `json:"total"`
	Items         []pageItem `json:"items"`
	FastScrollURI string     `json:"fastScrollUri"`
}

// maxItems is the most entries one list is fetched to, so a huge artist does
// not turn into a flood of requests or one enormous answer.
const maxItems = 1000

// pageSize is the most entries the API sends for one request, and the step
// its offsets go in.
const pageSize = 100

// pageItem is one entry in a section, which can be opened in its turn.
type pageItem struct {
	Title    string       `json:"title"`
	Subtitle string       `json:"subtitle"`
	Resource pageResource `json:"resource"`
}

// pageResource names what a page entry is, with its id escaped once.
type pageResource struct {
	Type     string `json:"type"`
	Defaults string `json:"defaults"`
	ID       struct {
		ObjectID string `json:"objectId"`
	} `json:"id"`
}

// asResult makes a page entry openable like a search result.
func (p pageResource) asResult(name string) resource {
	var r resource
	r.ID.ObjectID, _ = url.PathUnescape(p.ID.ObjectID)
	r.Name, r.Type, r.Defaults = name, p.Type, p.Defaults
	return r
}

// pageTrack is one track on a page. Its id is escaped once, song%3A1, where a
// search result's is not.
type pageTrack struct {
	Ordinal  int          `json:"ordinal"`
	Title    string       `json:"title"`
	Artists  []named      `json:"artists"`
	Duration string       `json:"duration"` // seconds, as text
	Resource pageResource `json:"resource"`
}

// section is one type's results, as the search of one service groups them.
type section struct {
	Info struct {
		Count int `json:"count"`
	} `json:"info"`
	Resources []resource `json:"resources"`
}

// console asks for one search after another and lists the results, each
// with a number across the whole list. It ends on q or at the end of input.
func console(opts searchOptions) error {
	if opts.account == 0 {
		return errors.New("-interactive searches one service alone, so it needs -account")
	}
	opts, err := opts.resolved()
	if err != nil {
		return err
	}

	var results []resource // the last search's results, in the order they are numbered
	in := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\nsearch, open <n>, or q> ")
		if !in.Scan() {
			fmt.Println()
			return in.Err()
		}
		query := strings.TrimSpace(in.Text())
		switch query {
		case "":
			continue
		case "q", "quit":
			return nil
		}
		if n, ok := openNumber(query); ok {
			if n < 1 || n > len(results) {
				fmt.Printf("   no result %d\n", n)
				continue
			}
			opened, err := b.open(opts, results[n-1])
			if err != nil {
				fmt.Println("  ", err)
			} else if len(opened) > 0 {
				results = opened
			}
			continue
		}
		opts.query = query
		body, err := b.fetchSearch(opts)
		if err != nil {
			fmt.Println("  ", err)
			continue
		}
		if results, err = printResults(body); err != nil {
			fmt.Println("  ", err)
		}
	}
}

// openNumber reads "open 12" or "o 12". A bare number is a search, since a
// query can be a number, such as 1999.
func openNumber(input string) (int, bool) {
	fields := strings.Fields(input)
	if len(fields) != 2 || (fields[0] != "open" && fields[0] != "o") {
		return 0, false
	}
	n, err := strconv.Atoi(fields[1])
	return n, err == nil
}

// browsePaths is the path segment the web app opens each type under. Only
// the types whose request has been seen in the browser are here.
var browsePaths = map[string]string{
	"ALBUM":  "albums",
	"ARTIST": "artists",
}

// pageTemplates are the templates printPage knows how to list.
var pageTemplates = map[string]bool{"TEMPLATE_ALBUM_PAGE": true, "TEMPLATE_PERSON_PAGE": true}

// open asks for what a result holds, as the web app does when the result is
// clicked, lists it, and returns its entries so they can be opened in turn.
func (b *browser) open(opts searchOptions, r resource) ([]resource, error) {
	segment, ok := browsePaths[r.Type]
	if !ok {
		return nil, fmt.Errorf("opening the type %s is not known yet; find its request in the web app", strings.ToLower(r.Type))
	}
	// The web app escapes the id twice: album:1 becomes album%253A1.
	id := url.PathEscape(url.PathEscape(r.ID.ObjectID))
	q := url.Values{"muse2": {"true"}, "count": {strconv.Itoa(opts.count)}}
	if r.Defaults != "" {
		q.Set("defaults", r.Defaults)
	}
	target := fmt.Sprintf("%s/api/content/v2/households/%s/services/%s/accounts/%d/%s/%s/browse?%s",
		webApp, url.PathEscape(opts.household), opts.services, opts.account, segment, id, q.Encode())

	fmt.Printf("\nopening %s\n", r.Name)
	resp, err := b.send(http.MethodGet, target, nil, http.Header{"Accept": {"application/json"}})
	if err != nil {
		return nil, err
	}
	out := filepath.Join(os.TempDir(), "sonosweb-browse.json")
	if err := os.WriteFile(out, resp.Body, 0o600); err != nil {
		return nil, err
	}
	fmt.Printf("   status %d, %d bytes, saved to %s\n", resp.StatusCode, len(resp.Body), out)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("opening answered %d: %.300s", resp.StatusCode, resp.Body)
	}
	var p page
	if err := json.Unmarshal(resp.Body, &p); err != nil {
		return nil, fmt.Errorf("reading the page: %w", err)
	}
	if err := b.completeSections(opts, r, &p); err != nil {
		fmt.Println("  ", err)
	}
	return printPage(p, resp.Body)
}

// completeSections fetches the whole list of a section that came with only a
// first page, such as an artist with more albums than were sent, up to
// maxItems. The API rounds an offset up to a multiple of pageSize and sends no
// more than pageSize at a time, so it is asked a whole page at a time.
func (b *browser) completeSections(opts searchOptions, r resource, p *page) error {
	for i, section := range p.Sections.Items {
		if section.Total <= len(section.Items) || section.FastScrollURI == "" {
			continue
		}
		fmt.Printf("   fetching all %d entries (at most %d)\n", section.Total, maxItems)
		var all []pageItem
		for offset := 0; offset < maxItems; offset += pageSize {
			items, more, err := b.fetchPage(opts, r, offset)
			if err != nil {
				return err
			}
			all = append(all, items...)
			if len(items) == 0 || !more {
				break
			}
		}
		if len(all) > len(section.Items) {
			p.Sections.Items[i].Items = all
		}
	}
	return nil
}

// fetchPage fetches the page of a container's list that starts at offset, and
// reports whether a page follows it. A page can hold fewer than pageSize
// entries and still not be the last.
func (b *browser) fetchPage(opts searchOptions, r resource, offset int) ([]pageItem, bool, error) {
	q := url.Values{"count": {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(offset)}, "filterExplicit": {"false"}, "muse2": {"true"}}
	target := fmt.Sprintf("%s/api/content/v2/households/%s/services/%s/accounts/%d/containers/%s/resources?%s",
		webApp, url.PathEscape(opts.household), opts.services, opts.account,
		url.PathEscape(url.PathEscape(r.ID.ObjectID)), q.Encode())
	resp, err := b.send(http.MethodGet, target, nil, http.Header{"Accept": {"application/json"}})
	if err != nil {
		return nil, false, err
	}
	if resp.StatusCode >= 400 {
		return nil, false, fmt.Errorf("fetching entries from %d answered %d", offset, resp.StatusCode)
	}
	var answer struct {
		Items   []pageItem `json:"items"`
		NextURI string     `json:"nextUri"`
	}
	if err := json.Unmarshal(resp.Body, &answer); err != nil {
		return nil, false, fmt.Errorf("reading entries from %d: %w", offset, err)
	}
	fmt.Printf("   %d entries from %d\n", len(answer.Items), offset)
	return answer.Items, answer.NextURI != "", nil
}

// printPage lists a page: an album's tracks, or the entries of each section,
// such as an artist's albums, numbered so they can be opened. A template not
// known yet is printed as its shape, from body, to learn it from.
func printPage(p page, body []byte) ([]resource, error) {
	if !pageTemplates[p.Type] {
		var answer any
		_ = json.Unmarshal(body, &answer)
		fmt.Println("   a", p.Type, "is not known yet; its shape:")
		printShape(answer, "   ", 0)
		return nil, nil
	}

	fmt.Printf("\n%s\n", strings.Join(nonEmpty(p.Title, p.Subtitle), " · "))
	var entries []resource
	for _, t := range p.Tracks.Items {
		var artists []string
		for _, a := range t.Artists {
			artists = append(artists, a.Name)
		}
		seconds, _ := strconv.Atoi(t.Duration)
		entry := t.Resource.asResult(t.Title)
		entries = append(entries, entry)
		fmt.Printf("%4d  %s  %s%s%s\n", len(entries),
			strings.Join(nonEmpty(t.Title, strings.Join(artists, ", "), duration(seconds*1000)), " · "),
			dim, entry.ID.ObjectID, plain)
	}
	if p.Tracks.Total > len(p.Tracks.Items) {
		fmt.Printf("   %d of %d tracks\n", len(p.Tracks.Items), p.Tracks.Total)
	}
	for _, section := range p.Sections.Items {
		if len(section.Items) == 0 {
			continue
		}
		fmt.Printf("\n%s (%d)\n", strings.Join(nonEmpty(section.Title, strings.ToUpper(section.Items[0].Resource.Type)+"S"), " "), section.Total)
		items := slices.Clone(section.Items)
		// Newest first; the API's own order is neither by date nor by name.
		slices.SortStableFunc(items, func(a, b pageItem) int { return strings.Compare(b.year(), a.year()) })
		for _, item := range items {
			entry := item.Resource.asResult(item.Title)
			entries = append(entries, entry)
			kind, title := item.kind()
			fmt.Printf("%4d  %-4s  %-6s  %s  %s%s%s\n", len(entries), item.year(), kind,
				strings.Join(nonEmpty(title, item.Subtitle), " · "), dim, entry.ID.ObjectID, plain)
		}
		if section.Total > len(section.Items) {
			fmt.Printf("   %d of %d\n", len(section.Items), section.Total)
		}
	}
	return entries, nil
}

// year is the release year from the summary in an entry's defaults, such as
// "Alternative - 2017", or empty when there is none.
func (i pageItem) year() string {
	var d struct {
		Summary string `json:"summary"`
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(i.Resource.Defaults, "="))
	if err != nil || json.Unmarshal(raw, &d) != nil {
		return ""
	}
	_, year, found := strings.Cut(d.Summary, " - ")
	if !found || len(year) != 4 {
		return ""
	}
	return year
}

// kind says what an album is from its title, and returns the title without
// the suffix that said so. Apple names singles and EPs with a fixed suffix;
// a live album is only known by the word in its title.
func (i pageItem) kind() (string, string) {
	if i.Resource.Type != "ALBUM" {
		return "", i.Title
	}
	switch {
	case strings.HasSuffix(i.Title, " - Single"):
		return "single", strings.TrimSuffix(i.Title, " - Single")
	case strings.HasSuffix(i.Title, " - EP"):
		return "EP", strings.TrimSuffix(i.Title, " - EP")
	case strings.Contains(strings.ToLower(i.Title), "live"):
		return "live", i.Title
	}
	return "album", i.Title
}

// nonEmpty keeps the parts that have text.
func nonEmpty(parts ...string) []string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return kept
}

// printResults lists a search's results by type, in the order the web app
// shows them, each numbered across the whole list, and returns them in that
// order.
func printResults(body []byte) ([]resource, error) {
	var answer map[string]json.RawMessage
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil, fmt.Errorf("reading the results: %w", err)
	}
	// An empty search still has resourceOrder, as an empty list.
	var order []string
	if err := json.Unmarshal(answer["resourceOrder"], &order); err != nil {
		return nil, errors.New("the answer has no resourceOrder; is it the search of one service?")
	}

	var results []resource
	for _, key := range order {
		var s section
		if err := json.Unmarshal(answer[key], &s); err != nil || len(s.Resources) == 0 {
			continue
		}
		fmt.Printf("\n%s (%d)\n", key, s.Info.Count)
		for _, r := range s.Resources {
			results = append(results, r)
			fmt.Printf("%4d  %s  %s%s%s\n", len(results), describe(r), dim, r.ID.ObjectID, plain)
		}
	}
	if len(results) == 0 {
		fmt.Println("\n   nothing found")
	}
	return results, nil
}

// describe is one result on one line: its name, and what tells it apart by
// type, such as an album's artist and year or a track's album and length.
func describe(r resource) string {
	parts := []string{r.Name}
	var artists []string
	for _, a := range r.Artists {
		artists = append(artists, a.Name)
	}
	switch r.Type {
	case "ALBUM":
		parts = append(parts, strings.Join(artists, ", "), summary(r.Summary))
	case "TRACK":
		parts = append(parts, strings.Join(artists, ", "), r.Album.Name, duration(r.Millis))
	case "PLAYLIST":
		parts = append(parts, r.Owner.Name)
	}
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, " · ")
}

// summary reads a result's summary, which is an object with content for some
// types and a string for others.
func summary(raw json.RawMessage) string {
	var object struct {
		Content string `json:"content"`
	}
	if json.Unmarshal(raw, &object) == nil && object.Content != "" {
		return object.Content
	}
	var text string
	_ = json.Unmarshal(raw, &text)
	return text
}

func duration(millis int) string {
	if millis <= 0 {
		return ""
	}
	seconds := millis / 1000
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}

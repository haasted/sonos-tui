package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

// pageSection is one group on a page, such as an artist's albums.
type pageSection struct {
	Title string     `json:"title"`
	Total int        `json:"total"`
	Items []pageItem `json:"items"`
}

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
	return printPage(resp.Body)
}

// printPage lists a page: an album's tracks, or the entries of each section,
// such as an artist's albums, numbered so they can be opened. A template not
// known yet is printed as its shape, to learn it from.
func printPage(body []byte) ([]resource, error) {
	var p page
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("reading the page: %w", err)
	}
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
		for _, item := range section.Items {
			entry := item.Resource.asResult(item.Title)
			entries = append(entries, entry)
			fmt.Printf("%4d  %s  %s%s%s\n", len(entries), strings.Join(nonEmpty(item.Title, item.Subtitle), " · "),
				dim, entry.ID.ObjectID, plain)
		}
		if section.Total > len(section.Items) {
			fmt.Printf("   %d of %d\n", len(section.Items), section.Total)
		}
	}
	return entries, nil
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

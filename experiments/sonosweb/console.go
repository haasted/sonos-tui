package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Artists []named         `json:"artists"`
	Summary json.RawMessage `json:"summary"` // an object with content, or a string
	Owner   named           `json:"owner"`
	Album   named           `json:"container"` // a track's album
	Millis  int             `json:"durationMs"`
}

type named struct {
	Name string `json:"name"`
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

	in := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\nsearch (q to quit)> ")
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
		opts.query = query
		body, err := b.fetchSearch(opts)
		if err != nil {
			fmt.Println("  ", err)
			continue
		}
		if err := printResults(body); err != nil {
			fmt.Println("  ", err)
		}
	}
}

// printResults lists a search's results by type, in the order the web app
// shows them, each numbered across the whole list.
func printResults(body []byte) error {
	var answer map[string]json.RawMessage
	if err := json.Unmarshal(body, &answer); err != nil {
		return fmt.Errorf("reading the results: %w", err)
	}
	// An empty search still has resourceOrder, as an empty list.
	var order []string
	if err := json.Unmarshal(answer["resourceOrder"], &order); err != nil {
		return errors.New("the answer has no resourceOrder; is it the search of one service?")
	}

	n := 0
	for _, key := range order {
		var s section
		if err := json.Unmarshal(answer[key], &s); err != nil || len(s.Resources) == 0 {
			continue
		}
		fmt.Printf("\n%s (%d)\n", key, s.Info.Count)
		for _, r := range s.Resources {
			n++
			fmt.Printf("%4d  %s  %s%s%s\n", n, describe(r), dim, r.ID.ObjectID, plain)
		}
	}
	if n == 0 {
		fmt.Println("\n   nothing found")
	}
	return nil
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

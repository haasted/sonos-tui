package sonoscloud

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// details is what a result's or an entry's defaults holds: base64 of a small
// JSON summary, which the web app uses to draw a page before its data comes.
// Which fields are there depends on the type.
type details struct {
	Name          string    `json:"name"`
	Artist        string    `json:"artist"`
	ArtistID      string    `json:"artistId"`
	Summary       string    `json:"summary"` // an album's "Genre - Year"
	ImageURL      string    `json:"imageUrl"`
	Playable      looseBool `json:"playable"`
	MimeType      string    `json:"mimeType"` // a track's, such as "audio/aac"
	DurationMs    int64     `json:"durationMs"`
	ContainerID   string    `json:"containerId"` // a track's album
	ContainerName string    `json:"containerName"`
	ContainerType string    `json:"containerType"`
}

// looseBool is a JSON flag written as true or as "true": a station's
// defaults write it as text, an album's as a boolean.
type looseBool bool

func (b *looseBool) UnmarshalJSON(data []byte) error {
	*b = looseBool(strings.Trim(string(data), `"`) == "true")
	return nil
}

// decodeDetails reads a defaults value, with or without base64 padding. An
// empty or broken value gives empty details: they only add to what a result
// already says.
func decodeDetails(defaults string) details {
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(defaults, "="))
	if err != nil {
		return details{}
	}
	var d details
	if json.Unmarshal(raw, &d) != nil {
		return details{}
	}
	return d
}

// sonosQuery encodes parameters as key, value, key, value, in the order given:
// url.Values sorts them, and the web app's order is part of how it looks.
func sonosQuery(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, url.QueryEscape(pairs[i])+"="+url.QueryEscape(pairs[i+1]))
	}
	return strings.Join(parts, "&")
}

// genreYear reads an album's summary, "Genre - Year", the only place the
// content API gives its year.
func genreYear(summary string) (genre string, year int, ok bool) {
	at := strings.LastIndex(summary, " - ")
	if at < 0 {
		return "", 0, false
	}
	year, err := strconv.Atoi(summary[at+3:])
	if err != nil {
		return "", 0, false
	}
	return summary[:at], year, true
}

// orDefault is value, or fallback when value is empty.
func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

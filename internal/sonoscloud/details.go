package sonoscloud

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
)

// genreYearPattern is an album's summary: a genre, which may hold " - ", then its year.
var genreYearPattern = regexp.MustCompile(`^(.+) - (\d{4})$`)

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

var _ json.Unmarshaler = new(looseBool)

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

// genreYear reads an album's summary, "Genre - Year", the only place the
// content API gives its year.
func genreYear(summary string) (genre, year string, ok bool) {
	m := genreYearPattern.FindStringSubmatch(summary)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// checkSummary logs an album summary that genreYear cannot read, to find the
// formats it does not know yet.
func checkSummary(album ResultID, summary string) {
	if _, _, ok := genreYear(summary); !ok {
		slog.Debug("album summary has no genre and year", "album", album.ObjectID, "summary", summary)
	}
}

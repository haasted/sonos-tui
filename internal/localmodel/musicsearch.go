package localmodel

import (
	"context"
	"fmt"
	"slices"
)

// Makes a guess about the user's intended search target, which can be used to pre-select
// media type and hoist results to the top.

// The categories a search string can be for, as the schema names them.
const (
	CategoryTrack        Category = "track"
	CategoryArtist       Category = "artist"
	CategoryAlbum        Category = "album"
	CategoryRadioStation Category = "radio_station"
)

// The classifier's instructions and schema: one full title and its likely
// categories, ranked, so a title that is both an album and a track can be
// both. As fast as one category (LOCAL-MODEL.md). The model's confidence was
// left out: it said "high" also when it was wrong.
const (
	classifyInstructions = "Identify what this music search string refers to."
	classifySchema       = `{
  "title": "MusicClassification",
  "type": "object",
  "additionalProperties": false,
  "x-order": ["full_title", "categories"],
  "required": ["full_title", "categories"],
  "properties": {
    "full_title": {
      "type": "string",
      "description": "The full official name of the most famous track, artist, album or radio station that the search string probably refers to. The search string is often incomplete: a surname alone means the full name of the best-known artist with that name. Correct misspellings and use the official capitalization."
    },
    "categories": {
      "type": "array",
      "minItems": 1,
      "maxItems": 3,
      "description": "What the full title is, the most likely first. Give both album and track when the title is both.",
      "items": {
        "type": "string",
        "enum": ["track", "artist", "album", "radio_station"]
      }
    }
  }
}`
)

// Category is what a search string may be for.
type Category string

// MusicSearchClassifier guesses what a music search string is for. It is
// one user of the fm engine: its own schema, instructions and prompt.
type MusicSearchClassifier struct {
	fm fm
}

// Classification is the model's guess at what a search string is for.
type Classification struct {
	FullTitle  string     `json:"full_title"` // the full name it probably means, such as "The Dark Side of the Moon"
	Categories []Category `json:"categories"` // what that title is, the most likely first
}

var _ validator = Classification{}

var categories = []Category{CategoryTrack, CategoryArtist, CategoryAlbum, CategoryRadioStation}

// NewMusicSearchClassifier is a classifier that asks the default fm.
func NewMusicSearchClassifier() MusicSearchClassifier {
	return MusicSearchClassifier{fm: defaultFM}
}

// Classify guesses what a music search string is for: its full name, and
// what that name is, the most likely first.
func (c MusicSearchClassifier) Classify(ctx context.Context, query string) (Classification, error) {
	return c.fm.respond[Classification](ctx, classifyInstructions, classifySchema,
		fmt.Sprintf("Search string: %q", query))
}

// Validate checks the answer against the schema: a title, and at least one
// known category.
func (c Classification) Validate() error {
	switch {
	case c.FullTitle == "":
		return fmt.Errorf("no full title")
	case len(c.Categories) == 0:
		return fmt.Errorf("no categories")
	}
	for _, category := range c.Categories {
		if !slices.Contains(categories, category) {
			return fmt.Errorf("category %q", category)
		}
	}
	return nil
}

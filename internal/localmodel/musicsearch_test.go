package localmodel_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/localmodel"
)

// Case: well-known search strings, each with its full name and category. A
// canary: greedy answers are stable, so a failure most likely means a macOS
// update brought a new model whose answers changed, not that the code broke.
func TestMusicSearchClassifier_KnownSearches(t *testing.T) {
	requireFm(t)
	classifier := localmodel.NewMusicSearchClassifier()

	for query, want := range map[string]localmodel.Classification{
		"dark side moon": {FullTitle: "The Dark Side of the Moon", Categories: []localmodel.Category{localmodel.CategoryAlbum}},
		"bowie":          {FullTitle: "David Bowie", Categories: []localmodel.Category{localmodel.CategoryArtist}},
		"ozzy":           {FullTitle: "Ozzy Osbourne", Categories: []localmodel.Category{localmodel.CategoryArtist}},
		"kexp":           {FullTitle: "KEXP", Categories: []localmodel.Category{localmodel.CategoryRadioStation}},
	} {
		start := time.Now()
		got, err := classifier.Classify(t.Context(), query)
		t.Logf("%-16q %v", query, time.Since(start).Round(time.Millisecond))

		require.NoError(t, err, query)
		assert.Equal(t, want, got, "%q: the model's answer changed; check the model, then the code", query)
	}
}

// Case: titles that are both an album and a track. Album is among their
// categories, so the search can prefer it. A canary, as above.
func TestMusicSearchClassifier_AnAlbumThatIsAlsoATrack(t *testing.T) {
	requireFm(t)
	classifier := localmodel.NewMusicSearchClassifier()

	for query, title := range map[string]string{"thriller": "Thriller", "purple rain": "Purple Rain", "nevermind": "Nevermind"} {
		got, err := classifier.Classify(t.Context(), query)

		require.NoError(t, err, query)
		assert.Equal(t, title, got.FullTitle, "%q: the model's answer changed; check the model, then the code", query)
		assert.Contains(t, got.Categories, localmodel.CategoryAlbum, "%q: the model's answer changed; check the model, then the code", query)
	}
}

// Case: search strings that look like options or hold quotes. They reach
// the model as text, so fm neither prints its help nor fails.
func TestMusicSearchClassifier_AnySearchStringIsText(t *testing.T) {
	requireFm(t)
	classifier := localmodel.NewMusicSearchClassifier()

	for _, query := range []string{"--help", "-bowie", `"heroes"`} {
		got, err := classifier.Classify(t.Context(), query)

		require.NoError(t, err, query)
		assert.NotEmpty(t, got.FullTitle, query)
	}
}

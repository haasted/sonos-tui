// The user's search string is supplied to the local language model for categorization.
// Used for subtly improving the UI if the response is received before the search results are rendered.
package main

import (
	"context"
	"errors"
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-tui/internal/localmodel"
	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// warmQuery is the throwaway question that loads the model before the
// reader's first one.
const warmQuery = "warm"

// musicClassifier guesses what a music search is for. Tests use a fake.
type musicClassifier interface {
	Classify(ctx context.Context, query string) (localmodel.Classification, error)
}

var _ musicClassifier = localmodel.MusicSearchClassifier{}

type (
	// classifierFoundMsg is the startup check's answer: the classifier, or
	// nil when the local model cannot be used.
	classifierFoundMsg struct{ classifier musicClassifier }

	// searchGuessMsg is the model's guess for the search of run gen.
	searchGuessMsg struct {
		gen   int
		guess localmodel.Classification
		err   error
	}
)

// guessTypes is the search type for each category the model names.
var guessTypes = map[localmodel.Category]sonoscloud.ResultType{
	localmodel.CategoryArtist:       sonoscloud.TypeArtist,
	localmodel.CategoryAlbum:        sonoscloud.TypeAlbum,
	localmodel.CategoryTrack:        sonoscloud.TypeTrack,
	localmodel.CategoryRadioStation: sonoscloud.TypeProgram,
}

// findLocalModel is the music search classifier, when fm can answer.
func findLocalModel(ctx context.Context) (musicClassifier, error) {
	if err := localmodel.Available(ctx); err != nil {
		return nil, err
	}
	return localmodel.NewMusicSearchClassifier(), nil
}

// lookForClassifier runs find in the background, so the start is not slower.
func lookForClassifier(ctx context.Context, find func(context.Context) (musicClassifier, error)) tea.Cmd {
	return func() tea.Msg {
		classifier, err := find(ctx)
		if err != nil {
			slog.Info("the local language model cannot be used; the search works without it", "err", err)
			return classifierFoundMsg{}
		}
		slog.Info("the local language model guides the search")
		return classifierFoundMsg{classifier: classifier}
	}
}

func (m appModel) applyClassifierFound(msg classifierFoundMsg) (tea.Model, tea.Cmd) {
	m.classifier = msg.classifier
	return m, nil
}

// guessSearch asks the classifier about query, under the search's run and
// context, so a newer query stops it too.
func guessSearch(ctx context.Context, classifier musicClassifier, query string, gen int) tea.Cmd {
	return func() tea.Msg {
		guess, err := classifier.Classify(ctx, query)
		return searchGuessMsg{gen: gen, guess: guess, err: err}
	}
}

// warmClassifier asks the throwaway question, so the first real one does
// not wait for the model to load.
func warmClassifier(ctx context.Context, classifier musicClassifier) tea.Cmd {
	return func() tea.Msg {
		if _, err := classifier.Classify(ctx, warmQuery); err != nil {
			slog.Debug("warming the local language model", "err", err)
		}
		return nil
	}
}

// applySearchGuess keeps the model's guess for the search still running. A
// guess that comes after the results is dropped: the screen must not move.
func (m appModel) applySearchGuess(msg searchGuessMsg) (tea.Model, tea.Cmd) {
	content, ok := m.search.Content.(searchContent)
	switch {
	case !ok || msg.gen != m.searchRun:
		return m, nil
	case msg.err != nil:
		if !errors.Is(msg.err, context.Canceled) {
			slog.Debug("the local language model gave no guess", "err", msg.err)
		}
		return m, nil
	case !content.running:
		slog.Debug("the local language model's guess came after the results; not used", "guess", msg.guess.Categories)
		return m, nil
	}
	content.guess = guessTypes[msg.guess.Categories[0]]
	m.search.Content = content
	slog.Debug("the local language model's guess", "guess", msg.guess.Categories, "title", msg.guess.FullTitle)
	return m, nil
}

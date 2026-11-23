package main

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/localmodel"
	"github.com/haasted/sonos-tui/internal/sonoscloud"
)

// Case: the model guesses album before the results come. The results open
// on the albums, though the artists come first in the type row.
func TestSearchGuess_ATimelyGuessOpensItsType(t *testing.T) {
	m := askedSearch(t, &fakeClassifier{})

	m = deliver(m, searchGuessMsg{gen: m.searchRun, guess: guessOf(localmodel.CategoryAlbum)})
	m = deliver(m, searchDoneMsg{gen: m.searchRun, query: "thriller", results: guessResults()})

	assert.Equal(t, sonoscloud.TypeAlbum, searchOf(m).kind)
}

// Case: the guess comes after the results. It is not used: the results stay
// on the first type that has any, so the screen does not move.
func TestSearchGuess_ALateGuessIsNotUsed(t *testing.T) {
	m := askedSearch(t, &fakeClassifier{})

	m = deliver(m, searchDoneMsg{gen: m.searchRun, query: "thriller", results: guessResults()})
	m = deliver(m, searchGuessMsg{gen: m.searchRun, guess: guessOf(localmodel.CategoryAlbum)})

	assert.Equal(t, sonoscloud.TypeArtist, searchOf(m).kind)
}

// Case: a timely guess whose type has no results, a failed guess, and a
// guess for an earlier query. Each leaves the results as without a model.
func TestSearchGuess_AGuessThatCannotBeUsedChangesNothing(t *testing.T) {
	for name, guess := range map[string]func(m appModel) searchGuessMsg{
		"no results of its type": func(m appModel) searchGuessMsg {
			return searchGuessMsg{gen: m.searchRun, guess: guessOf(localmodel.CategoryRadioStation)}
		},
		"failed": func(m appModel) searchGuessMsg {
			return searchGuessMsg{gen: m.searchRun, err: errors.New("fm exited 1")}
		},
		"earlier query": func(m appModel) searchGuessMsg {
			return searchGuessMsg{gen: m.searchRun - 1, guess: guessOf(localmodel.CategoryAlbum)}
		},
	} {
		m := askedSearch(t, &fakeClassifier{})

		m = deliver(m, guess(m))
		m = deliver(m, searchDoneMsg{gen: m.searchRun, query: "thriller", results: guessResults()})

		assert.Equal(t, sonoscloud.TypeArtist, searchOf(m).kind, name)
	}
}

// Case: a guess kept for one query, then a new query. The new results do not
// open on the old guess.
func TestSearchGuess_ANewQueryForgetsTheOldGuess(t *testing.T) {
	m := askedSearch(t, &fakeClassifier{})
	m = deliver(m, searchGuessMsg{gen: m.searchRun, guess: guessOf(localmodel.CategoryAlbum)})

	m = deliver(m, searchAskMsg{query: "bowie"})
	m = deliver(m, searchDoneMsg{gen: m.searchRun, query: "bowie", results: guessResults()})

	assert.Equal(t, sonoscloud.TypeArtist, searchOf(m).kind)
}

// Case: a query asked with the classifier there, and one without. Only the
// first asks the model, with the query as typed.
func TestSearchGuess_TheModelIsAskedBesideTheSearch(t *testing.T) {
	classifier := &fakeClassifier{}
	m := pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s")
	m.classifier = classifier
	_, cmd := deliverCmd(m, searchAskMsg{query: "thriller"})
	flatten(cmd)

	m.classifier = nil
	_, cmd = deliverCmd(m, searchAskMsg{query: "bowie"})
	flatten(cmd)

	assert.Equal(t, []string{"thriller"}, classifier.queries())
}

// Case: the search opened with the classifier there. The model is asked a
// throwaway question, so the reader's first one does not wait for it to load.
func TestSearchGuess_OpeningTheSearchWarmsTheModel(t *testing.T) {
	classifier := &fakeClassifier{}
	m := searchModel(t, &fakeCloud{signedIn: true})
	m.classifier = classifier

	_, cmd := pressKeyCmd(m, "s")
	runBranches(cmd)

	assert.Eventually(t, func() bool { return slices.Contains(classifier.queries(), warmQuery) },
		time.Second, 10*time.Millisecond, "the model was not warmed")
}

// Case: the startup check finds no usable model. The app has no classifier
// and the search works as without one.
func TestSearchGuess_NoModelLeavesNoClassifier(t *testing.T) {
	m := sizedModel(t)
	m.classifier = &fakeClassifier{}

	m = deliver(m, classifierFoundMsg{})

	assert.Nil(t, m.classifier)
}

// fakeClassifier answers every query with guess, and records the queries.
type fakeClassifier struct {
	guess localmodel.Classification
	asked []string

	mu sync.Mutex // guards asked
}

var _ musicClassifier = &fakeClassifier{}

func (f *fakeClassifier) Classify(_ context.Context, query string) (localmodel.Classification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, query)
	return f.guess, nil
}

func (f *fakeClassifier) queries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

// askedSearch is the open search with classifier, a query asked and its
// answer not yet in.
func askedSearch(t *testing.T, classifier *fakeClassifier) appModel {
	t.Helper()
	m := pressKey(searchModel(t, &fakeCloud{signedIn: true}), "s")
	m.classifier = classifier
	m = deliver(m, searchAskMsg{query: "thriller"})
	require.True(t, searchOf(m).running)
	return m
}

// guessOf is a guess of one category.
func guessOf(category localmodel.Category) localmodel.Classification {
	return localmodel.Classification{FullTitle: "Thriller", Categories: []localmodel.Category{category}}
}

// guessResults has artists, albums and tracks, and no stations.
func guessResults() sonoscloud.SearchResults {
	return sonoscloud.SearchResults{Groups: []sonoscloud.ResultGroup{
		{Name: "ARTISTS", Results: []sonoscloud.Result{{Type: sonoscloud.TypeArtist, Name: "Michael Jackson"}}},
		{Name: "ALBUMS", Results: []sonoscloud.Result{{Type: sonoscloud.TypeAlbum, Name: "Thriller"}}},
		{Name: "TRACKS", Results: []sonoscloud.Result{{Type: sonoscloud.TypeTrack, Name: "Thriller"}}},
	}}
}

// runBranches starts each command of a batch in its own goroutine, as Bubble
// Tea does, without waiting for the timed ones.
func runBranches(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		return
	}
	for _, branch := range batch {
		if branch != nil {
			go branch()
		}
	}
}

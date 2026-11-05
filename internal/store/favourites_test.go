package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a favourite stored and read back, which must keep every field.
func TestFavourites_AFavouriteSurvivesTheRoundTrip(t *testing.T) {
	s := openWithoutFavourites(t)
	want := playlistFavourite(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))

	added, err := s.Favourites().Add(t.Context(), want)
	require.NoError(t, err)
	require.True(t, added)

	got, err := s.Favourites().List(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 1)

	want.ID = got[0].ID
	assert.Equal(t, want, got[0])
}

// Case: the same thing marked twice. One row stays, and the second mark
// reports that nothing was added.
func TestFavourites_MarkingTwiceKeepsOneRow(t *testing.T) {
	s := openWithoutFavourites(t)
	first := playlistFavourite(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
	again := first
	again.Title = "Renamed"
	again.AddedAt = first.AddedAt.Add(time.Hour)

	_, err := s.Favourites().Add(t.Context(), first)
	require.NoError(t, err)
	added, err := s.Favourites().Add(t.Context(), again)
	require.NoError(t, err)

	assert.False(t, added)
	got, err := s.Favourites().List(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Awesome singles", got[0].Title, "the row already stored must be kept")
}

// Case: several favourites, which must list newest first.
func TestFavourites_ListIsNewestFirst(t *testing.T) {
	s := openWithoutFavourites(t)
	older := playlistFavourite(time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
	newer := playlistFavourite(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
	newer.ItemID, newer.Title = "libraryplaylist:p.ZOAXddOUZE1LE", "Metallica"

	for _, fav := range []Favourite{older, newer} {
		_, err := s.Favourites().Add(t.Context(), fav)
		require.NoError(t, err)
	}

	got, err := s.Favourites().List(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "Metallica", got[0].Title)
	assert.Equal(t, "Awesome singles", got[1].Title)
}

// Case: a kind the table does not know, which the schema must refuse.
func TestFavourites_AnUnknownKindIsRefused(t *testing.T) {
	s := openWithoutFavourites(t)
	fav := playlistFavourite(time.Now())
	fav.Kind = "artist"

	_, err := s.Favourites().Add(t.Context(), fav)
	assert.Error(t, err)
}

// Case: an empty list, which is a first run rather than a fault.
func TestFavourites_AnEmptyListIsFine(t *testing.T) {
	got, err := openWithoutFavourites(t).Favourites().List(t.Context())

	require.NoError(t, err)
	assert.Empty(t, got)
}

// Case: the cache cleared with favourites stored. Favourites are not cache
// and must survive.
func TestFavourites_ClearingTheCacheKeepsThem(t *testing.T) {
	s := openWithoutFavourites(t)
	fillCache(t, s)
	_, err := s.Favourites().Add(t.Context(), playlistFavourite(time.Now()))
	require.NoError(t, err)

	require.NoError(t, s.Cache().Clear(t.Context()))

	assert.Equal(t, 1, countIn(t, s, "favourite"))
}

// Case: removing one favourite of two, then removing it again. The other
// stays, and the second removal is not a fault.
func TestFavourites_RemoveDeletesOneRow(t *testing.T) {
	s := openWithoutFavourites(t)
	at := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	other := playlistFavourite(at)
	other.ItemID, other.Title = "libraryplaylist:p.other", "Other"
	for _, fav := range []Favourite{playlistFavourite(at), other} {
		_, err := s.Favourites().Add(t.Context(), fav)
		require.NoError(t, err)
	}
	stored, err := s.Favourites().List(t.Context())
	require.NoError(t, err)
	gone := stored[0].ID

	require.NoError(t, s.Favourites().Remove(t.Context(), gone))
	require.NoError(t, s.Favourites().Remove(t.Context(), gone))

	left, err := s.Favourites().List(t.Context())
	require.NoError(t, err)
	require.Len(t, left, 1)
	assert.Equal(t, stored[1].ID, left[0].ID)
}

func playlistFavourite(addedAt time.Time) Favourite {
	return Favourite{
		Kind:     FavouritePlaylist,
		Title:    "Awesome singles",
		URI:      "x-rincon-cpcontainer:00060000libraryplaylist%3ap.V7VYpk7FYWLeW?sid=204&flags=0&sn=2",
		Metadata: "<DIDL-Lite/>",
		Service:  204,
		Account:  2,
		ItemID:   "libraryplaylist:p.V7VYpk7FYWLeW",
		AddedAt:  addedAt.UTC(),
	}
}

// openWithoutFavourites opens a temporary store with the favourite table
// emptied, so a test sees only the rows it adds, whatever a migration seeds.
func openWithoutFavourites(t *testing.T) *Store {
	t.Helper()
	s := openTemp(t)
	_, err := s.db.ExecContext(t.Context(), `DELETE FROM favourite`)
	require.NoError(t, err)
	return s
}

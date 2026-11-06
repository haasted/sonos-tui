package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a favourite stored and read back, which must keep every field.
func TestFavourites_AFavouriteSurvivesTheRoundTrip(t *testing.T) {
	s := openTemp(t)
	want := playlistFavourite(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))

	added, err := s.Favourites().Add(t.Context(), want)
	require.NoError(t, err)
	require.Equal(t, 1, added)

	got, err := s.Favourites().List(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 1)

	want.ID = got[0].ID
	assert.Equal(t, want, got[0])
}

// Case: the same thing marked twice. One row stays with what was stored
// first, the second mark reports nothing added, and the row takes the
// second mark's time, so it moves to the top.
func TestFavourites_MarkingTwiceKeepsOneRow(t *testing.T) {
	s := openTemp(t)
	first := playlistFavourite(time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
	again := first
	again.Title = "Renamed"
	again.AddedAt = first.AddedAt.Add(time.Hour)

	_, err := s.Favourites().Add(t.Context(), first)
	require.NoError(t, err)
	added, err := s.Favourites().Add(t.Context(), again)
	require.NoError(t, err)

	assert.Equal(t, 0, added)
	got, err := s.Favourites().List(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Awesome singles", got[0].Title, "the row already stored must be kept")
	assert.True(t, again.AddedAt.Equal(got[0].AddedAt), "the second mark did not move the row to the top")
}

// Case: several favourites in one call, one of them already stored. The new
// ones are counted, and the list follows the times given.
func TestFavourites_AddSeveralAtOnce(t *testing.T) {
	s := openTemp(t)
	at := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	old := playlistFavourite(at)
	_, err := s.Favourites().Add(t.Context(), old)
	require.NoError(t, err)

	second, third := playlistFavourite(at), playlistFavourite(at)
	second.ItemID, second.Title = "libraryplaylist:p.second", "Second"
	third.ItemID, third.Title = "libraryplaylist:p.third", "Third"
	old.AddedAt = at.Add(time.Hour)
	second.AddedAt = at.Add(time.Hour - time.Microsecond)
	third.AddedAt = at.Add(time.Hour - 2*time.Microsecond)

	added, err := s.Favourites().Add(t.Context(), old, second, third)
	require.NoError(t, err)
	assert.Equal(t, 2, added)

	got, err := s.Favourites().List(t.Context())
	require.NoError(t, err)
	var titles []string
	for _, fav := range got {
		titles = append(titles, fav.Title)
	}
	assert.Equal(t, []string{"Awesome singles", "Second", "Third"}, titles)
}

// Case: two times whose RFC3339Nano text would sort the wrong way round, one
// a whole second and one a microsecond after it, and one written in another
// zone. The list must still be newest first.
func TestFavourites_TimesSortAsTimesDo(t *testing.T) {
	s := openTemp(t)
	at := time.Date(2026, 10, 25, 12, 0, 0, 120000000, time.UTC)
	earlier, later := playlistFavourite(at), playlistFavourite(at.Add(time.Microsecond))
	later.ItemID, later.Title = "libraryplaylist:p.later", "Later"
	zoned := playlistFavourite(at.Add(30 * time.Minute).In(time.FixedZone("CET", 3600)))
	zoned.ItemID, zoned.Title = "libraryplaylist:p.zoned", "Zoned"

	_, err := s.Favourites().Add(t.Context(), earlier, later, zoned)
	require.NoError(t, err)

	got, err := s.Favourites().List(t.Context())
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, []string{"Zoned", "Later", "Awesome singles"},
		[]string{got[0].Title, got[1].Title, got[2].Title})
}

// Case: several favourites, which must list newest first.
func TestFavourites_ListIsNewestFirst(t *testing.T) {
	s := openTemp(t)
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
	s := openTemp(t)
	fav := playlistFavourite(time.Now())
	fav.Kind = "artist"

	_, err := s.Favourites().Add(t.Context(), fav)
	assert.Error(t, err)
}

// Case: an empty list, which is a first run rather than a fault.
func TestFavourites_AnEmptyListIsFine(t *testing.T) {
	got, err := openTemp(t).Favourites().List(t.Context())

	require.NoError(t, err)
	assert.Empty(t, got)
}

// Case: the cache cleared with favourites stored. Favourites are not cache
// and must survive.
func TestFavourites_ClearingTheCacheKeepsThem(t *testing.T) {
	s := openTemp(t)
	fillCache(t, s)
	_, err := s.Favourites().Add(t.Context(), playlistFavourite(time.Now()))
	require.NoError(t, err)

	require.NoError(t, s.Cache().Clear(t.Context()))

	assert.Equal(t, 1, countIn(t, s, "favourite"))
}

// Case: removing one favourite of two, then removing it again. The other
// stays, and the second removal is not a fault.
func TestFavourites_RemoveDeletesOneRow(t *testing.T) {
	s := openTemp(t)
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

// Case: asking which favourites are stored. One stored and one not answer
// true and false, in the order asked.
func TestFavourites_StoredSaysWhichAreThere(t *testing.T) {
	s := openTemp(t)
	stored := playlistFavourite(time.Now())
	_, err := s.Favourites().Add(t.Context(), stored)
	require.NoError(t, err)
	other := playlistFavourite(time.Now())
	other.ItemID = "libraryplaylist:p.other"

	got, err := s.Favourites().Stored(t.Context(), other, stored)

	require.NoError(t, err)
	assert.Equal(t, []bool{false, true}, got)
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

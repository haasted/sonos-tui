package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// FavouriteKind is what a favourite is, as the favourite table's CHECK names it.
type FavouriteKind string

const (
	FavouriteTrack    FavouriteKind = "track"
	FavouriteAlbum    FavouriteKind = "album"
	FavouritePlaylist FavouriteKind = "playlist"
	FavouriteStation  FavouriteKind = "station"
)

// Favourite is one thing the reader marked. URI and Metadata are what a
// speaker is given to play it; Service, Account and ItemID say when two
// favourites are the same thing.
type Favourite struct {
	ID       int64
	Kind     FavouriteKind
	Title    string
	Artist   string // empty where the kind has none
	Album    string // empty where the kind has none
	URI      string
	Metadata string
	Service  int
	Account  int
	ItemID   string
	AddedAt  time.Time
}

// Favourites is the handle on the favourite table.
type Favourites struct {
	db *sql.DB
}

// Favourites returns the handle on the reader's favourites.
func (s *Store) Favourites() *Favourites {
	return &Favourites{db: s.db}
}

// Add stores fav, and reports false when the same thing is already there.
// Marking twice is not a fault, so the row already stored is kept as it is.
func (f *Favourites) Add(ctx context.Context, fav Favourite) (bool, error) {
	result, err := f.db.ExecContext(ctx, `
		INSERT INTO favourite
			(kind, title, artist, album, uri, metadata, service, account, item_id, added_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (service, account, item_id) DO NOTHING`,
		fav.Kind, fav.Title, fav.Artist, fav.Album, fav.URI, fav.Metadata,
		fav.Service, fav.Account, fav.ItemID, fav.AddedAt.Format(timeFormat))
	if err != nil {
		return false, fmt.Errorf("adding favourite %q: %w", fav.Title, err)
	}

	added, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("adding favourite %q: %w", fav.Title, err)
	}
	return added == 1, nil
}

// List returns every favourite, newest first.
func (f *Favourites) List(ctx context.Context) ([]Favourite, error) {
	rows, err := f.db.QueryContext(ctx, `
		SELECT id, kind, title, artist, album, uri, metadata, service, account, item_id, added_at
		FROM favourite ORDER BY added_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("reading favourites: %w", err)
	}
	defer rows.Close()

	var favourites []Favourite
	for rows.Next() {
		var fav Favourite
		var addedAt string
		if err := rows.Scan(&fav.ID, &fav.Kind, &fav.Title, &fav.Artist, &fav.Album,
			&fav.URI, &fav.Metadata, &fav.Service, &fav.Account, &fav.ItemID, &addedAt); err != nil {
			return nil, fmt.Errorf("reading a favourite: %w", err)
		}
		if fav.AddedAt, err = time.Parse(timeFormat, addedAt); err != nil {
			return nil, fmt.Errorf("reading when %q was added: %w", fav.Title, err)
		}
		favourites = append(favourites, fav)
	}
	return favourites, rows.Err()
}

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

// Add stores the favourites in one transaction, and reports how many were
// new. One already stored keeps its row and only takes the new added time,
// so marking it again brings it to the top of the list.
func (f *Favourites) Add(ctx context.Context, favs ...Favourite) (int, error) {
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	added := 0
	for _, fav := range favs {
		var stored bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM favourite WHERE service = ? AND account = ? AND item_id = ?)`,
			fav.Service, fav.Account, fav.ItemID).Scan(&stored); err != nil {
			return 0, fmt.Errorf("looking for favourite %q: %w", fav.Title, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO favourite
				(kind, title, artist, album, uri, metadata, service, account, item_id, added_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (service, account, item_id) DO UPDATE SET added_at = excluded.added_at`,
			fav.Kind, fav.Title, fav.Artist, fav.Album, fav.URI, fav.Metadata,
			fav.Service, fav.Account, fav.ItemID, formatTime(fav.AddedAt)); err != nil {
			return 0, fmt.Errorf("adding favourite %q: %w", fav.Title, err)
		}
		if !stored {
			added++
		}
	}
	return added, tx.Commit()
}

// Stored reports, for each of favs, whether a favourite of the same service,
// account and id is stored already.
func (f *Favourites) Stored(ctx context.Context, favs ...Favourite) ([]bool, error) {
	stored := make([]bool, len(favs))
	for i, fav := range favs {
		if err := f.db.QueryRowContext(ctx, `
			SELECT EXISTS (SELECT 1 FROM favourite WHERE service = ? AND account = ? AND item_id = ?)`,
			fav.Service, fav.Account, fav.ItemID).Scan(&stored[i]); err != nil {
			return nil, fmt.Errorf("looking for favourite %q: %w", fav.Title, err)
		}
	}
	return stored, nil
}

// Remove deletes the favourite with the given id. An id not stored is not a
// fault: the favourite is gone either way.
func (f *Favourites) Remove(ctx context.Context, id int64) error {
	if _, err := f.db.ExecContext(ctx, `DELETE FROM favourite WHERE id = ?`, id); err != nil {
		return fmt.Errorf("removing favourite %d: %w", id, err)
	}
	return nil
}

// Restore stores a removed favourite again, with its id and added time, so
// it is back in its old place. It fails if another favourite holds its id or
// its service, account and item id.
func (f *Favourites) Restore(ctx context.Context, fav Favourite) error {
	if _, err := f.db.ExecContext(ctx, `
		INSERT INTO favourite
			(id, kind, title, artist, album, uri, metadata, service, account, item_id, added_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fav.ID, fav.Kind, fav.Title, fav.Artist, fav.Album, fav.URI, fav.Metadata,
		fav.Service, fav.Account, fav.ItemID, formatTime(fav.AddedAt)); err != nil {
		return fmt.Errorf("restoring favourite %q: %w", fav.Title, err)
	}
	return nil
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
		if fav.AddedAt, err = parseTime(addedAt); err != nil {
			return nil, fmt.Errorf("reading when %q was added: %w", fav.Title, err)
		}
		favourites = append(favourites, fav)
	}
	return favourites, rows.Err()
}

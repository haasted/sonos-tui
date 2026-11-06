// How the application drives the favourites modal: opening it, reading the
// favourites and the media type shown last, and starting a favourite on the
// speaker in view. favourites.go holds the list itself.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/store"
	"github.com/haasted/sonos-sandbox/internal/ui/nowplaying"
	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

// errNoStore is what the modal says when the program runs without its
// database, which only a test does.
var errNoStore = errors.New("no database")

// favouriteStationNotQueued answers n or N on a station: stations play in
// place of the queue, and cannot be put into it.
const favouriteStationNotQueued = "a station cannot be queued"

// What marking a track as a favourite answers. The reader can see the track
// under the cursor, so the notices do not name it.
const (
	favouriteMarked     = "added as a favourite"
	favouriteMarkedNot  = "already a favourite"
	favouriteNoService  = "not from a music service, cannot be marked"
	favouriteMarkFailed = "could not mark as a favourite"

	// Marking several tracks says only that they were added. One already a
	// favourite has moved to the top, which the list shows; a skipped one is
	// counted, since nothing else shows it.
	favouriteMarkedSeveral = "added as favourites"
	favouriteMarkedSkipped = "added as favourites, %d cannot be marked"
)

// What removing a favourite answers, in the queue's words for a removed track.
const (
	favouriteRemoved      = "removed"
	favouriteRemoveFailed = "could not remove the favourite"
)

// openFavourites opens the favourites modal and reads what it lists. Every
// open reads again: it is a local file, and a list read earlier may be stale.
func (m appModel) openFavourites() (tea.Model, tea.Cmd) {
	if _, ok := m.activeSpeaker(); !ok {
		return m, nil
	}

	m.favourites = m.favourites.Open()
	m.favourites.Content = favouritesContent{
		listView: listView[store.Favourite]{
			rows:     m.favourites.ContentRows(),
			width:    m.favourites.ContentColumns(),
			openedAt: time.Now(),
		},
		loading: true,
	}

	m, sweeping := m.restartSweepChain()
	hint := restartListHint(m.favouritesSlot())
	return m, tea.Batch(sweeping, hint, loadFavourites(m.ctx, m.db))
}

// loadFavourites reads every favourite and the media type shown last.
func loadFavourites(ctx context.Context, db *store.Store) tea.Cmd {
	return func() tea.Msg {
		if db == nil {
			return favouritesLoadedMsg{err: errNoStore}
		}
		favourites, err := db.Favourites().List(ctx)
		if err != nil {
			return favouritesLoadedMsg{err: err}
		}
		kind, err := db.Settings().Get(ctx, store.SettingFavouritesKind, "")
		if err != nil {
			slog.Warn("reading the favourites' media type", "err", err)
		}
		return favouritesLoadedMsg{favourites: favourites, kind: store.FavouriteKind(kind)}
	}
}

// applyFavouritesLoaded puts the favourites read into the modal, if it is
// still open to show them.
func (m appModel) applyFavouritesLoaded(msg favouritesLoadedMsg) (tea.Model, tea.Cmd) {
	content, ok := m.favourites.Content.(favouritesContent)
	if !ok || !m.favourites.IsOpen() {
		return m, nil
	}
	m.favourites.Content = content.loaded(msg)
	if content := m.favourites.Content.(favouritesContent); content.overflows() {
		return m, restartListChain(m.favouritesSlot())
	}
	return m, nil
}

// applyFavouriteKindChosen remembers the media type the reader switched to,
// writing it in the background.
func (m appModel) applyFavouriteKindChosen(msg favouriteKindChosenMsg) (tea.Model, tea.Cmd) {
	if m.db == nil {
		return m, nil
	}
	db, ctx := m.db, m.ctx
	return m, func() tea.Msg {
		if err := db.Settings().Set(ctx, store.SettingFavouritesKind, string(msg.kind)); err != nil {
			slog.Warn("remembering the favourites' media type", "err", err)
		}
		return nil
	}
}

// applyFavouriteAction starts a favourite on the speaker in view, and so on
// its group. enter closes the modal, as it does in the queue; n and N leave
// it open, so several can be added in a row.
func (m appModel) applyFavouriteAction(msg favouriteActionMsg) (tea.Model, tea.Cmd) {
	speaker, ok := m.activeSpeaker()
	if !ok {
		return m, nil
	}
	fav := msg.favourite
	station := fav.Kind == store.FavouriteStation
	if station && msg.action != favouritePlay {
		return m, notify(panes.NoticeInfo, favouriteStationNotQueued)
	}

	playable := sonos.Playable{URI: fav.URI, Metadata: fav.Metadata, Title: fav.Title}
	if msg.action == favouritePlay {
		m.favourites = m.favourites.Close()
		slot := m.favouritesSlot()
		*slot.pulse, *slot.hint = nextGen(), nextGen()
	}
	return m, startFavourite(speaker, playable, msg.action, station)
}

// applyFavouriteMark stores the tracks the queue asked for as favourites, in
// the background, and says what happened.
func (m appModel) applyFavouriteMark(msg favouriteMarkMsg) (tea.Model, tea.Cmd) {
	favs, skipped := trackFavourites(msg.tracks, time.Now())
	return m, storeFavourites(m.ctx, m.db, favs, len(msg.tracks), skipped)
}

// storeFavourites stores favs in the background, and answers with the notice
// for asked marks, skipped of which could not be.
func storeFavourites(ctx context.Context, db *store.Store, favs []store.Favourite, asked, skipped int) tea.Cmd {
	return func() tea.Msg {
		if len(favs) == 0 {
			return panes.NoticeMsg{Level: panes.NoticeInfo, Text: favouriteNoService}
		}
		if db == nil {
			return panes.NoticeMsg{Level: panes.NoticeError, Text: favouriteMarkFailed + ": " + errNoStore.Error()}
		}

		added, err := db.Favourites().Add(ctx, favs...)
		if err != nil {
			slog.Warn("marking favourites failed", "favourites", len(favs), "err", err)
			return panes.NoticeMsg{Level: panes.NoticeError, Text: favouriteMarkFailed}
		}
		return panes.NoticeMsg{Level: panes.NoticeInfo, Text: markedNotice(asked, added, skipped), Confirmation: true}
	}
}

// applyFavouriteChoose offers the track and the queue's playlist or album to
// mark. Only what can be marked is offered, and one choice needs no chooser.
func (m appModel) applyFavouriteChoose(msg favouriteChooseMsg) (tea.Model, tea.Cmd) {
	rows := markRowsFor(msg.track, msg.source, time.Now())
	switch len(rows) {
	case 0:
		return m, storeFavourites(m.ctx, m.db, nil, 1, 1)
	case 1:
		return m, storeFavourites(m.ctx, m.db, []store.Favourite{rows[0].fav}, 1, 0)
	}
	m.marking.Content = markContent{rows: rows}
	m.marking = m.marking.Open()
	return m, nil
}

// applyFavouriteChosen closes the chooser and stores what was chosen.
func (m appModel) applyFavouriteChosen(msg favouriteChosenMsg) (tea.Model, tea.Cmd) {
	m.marking = m.marking.Close()
	return m, storeFavourites(m.ctx, m.db, []store.Favourite{msg.fav}, 1, 0)
}

// markedNotice is what marking says. One track says whether it was new; many
// say only that they were added, and how many could not be.
func markedNotice(asked, added, skipped int) string {
	switch {
	case asked == 1 && added == 1:
		return favouriteMarked
	case asked == 1:
		return favouriteMarkedNot
	case skipped == 0:
		return favouriteMarkedSeveral
	default:
		return fmt.Sprintf(favouriteMarkedSkipped, skipped)
	}
}

// trackFavourites is the favourite rows for tracks, each a microsecond older
// than the one before, so the list shows them in the order given. Tracks not
// from a music service are left out and counted.
func trackFavourites(tracks []sonos.TrackMetadata, now time.Time) (favs []store.Favourite, skipped int) {
	for i, track := range tracks {
		fav, err := trackFavourite(track, now.Add(-time.Duration(i)*time.Microsecond))
		if err != nil {
			slog.Info("not marking a track", "track", track.Title, "err", err)
			skipped++
			continue
		}
		favs = append(favs, fav)
	}
	return favs, skipped
}

// applyFavouriteFaded takes the faded favourites off the list, if the modal
// is still open to show them.
func (m appModel) applyFavouriteFaded(msg favouriteFadedMsg) (tea.Model, tea.Cmd) {
	content, ok := m.favourites.Content.(favouritesContent)
	if !ok || !m.favourites.IsOpen() {
		return m, nil
	}
	content, cmd := content.faded(msg.gen)
	m.favourites.Content = content
	return m, cmd
}

// applyFavouriteRemove deletes a removed favourite in the background. A
// failed delete leaves the row, so it is back on the next open.
func (m appModel) applyFavouriteRemove(msg favouriteRemoveMsg) (tea.Model, tea.Cmd) {
	db, ctx, fav := m.db, m.ctx, msg.favourite
	return m, func() tea.Msg {
		if db == nil {
			return panes.NoticeMsg{Level: panes.NoticeError, Text: favouriteRemoveFailed + ": " + errNoStore.Error()}
		}
		if err := db.Favourites().Remove(ctx, fav.ID); err != nil {
			slog.Warn("removing a favourite failed", "favourite", fav.Title, "err", err)
			return panes.NoticeMsg{Level: panes.NoticeError, Text: favouriteRemoveFailed}
		}
		return panes.NoticeMsg{Level: panes.NoticeInfo, Text: favouriteRemoved, Confirmation: true}
	}
}

// trackFavourite is the favourite row for a track in the queue. Only a track
// from a music service has the service, account and id a row needs.
func trackFavourite(track sonos.TrackMetadata, now time.Time) (store.Favourite, error) {
	playable, err := sonos.TrackPlayable(track)
	if err != nil {
		return store.Favourite{}, err
	}
	ref, _ := sonos.ParseServiceURI(track.URI) // TrackPlayable succeeded, so the URI parses
	return store.Favourite{
		Kind:     store.FavouriteTrack,
		Title:    track.Title,
		Artist:   track.Artist,
		Album:    track.Album,
		URI:      playable.URI,
		Metadata: playable.Metadata,
		Service:  ref.Service,
		Account:  ref.Account,
		ItemID:   ref.ItemID,
		AddedAt:  now,
	}, nil
}

// sourceFavourite is the favourite row for the playlist or album a queue was
// started from, or false for one that cannot be marked: not a playlist or an
// album, or not from a music service.
func sourceFavourite(source sonos.QueueSource, now time.Time) (store.Favourite, bool) {
	kind, known := source.Kind()
	ref, parsed := sonos.ParseServiceURI(source.URI)
	if !known || !parsed {
		return store.Favourite{}, false
	}
	fav := store.Favourite{
		Kind:     store.FavouritePlaylist,
		Title:    source.Title,
		URI:      source.URI,
		Metadata: sonos.StripArtwork(source.Metadata),
		Service:  ref.Service,
		Account:  ref.Account,
		ItemID:   ref.ItemID,
		AddedAt:  now,
	}
	if kind == sonos.ContainerAlbum {
		fav.Kind, fav.Artist = store.FavouriteAlbum, source.Creator
	}
	return fav, true
}

// markableSource reports whether the queue's playlist or album can be marked.
func markableSource(source sonos.QueueSource) bool {
	_, ok := sourceFavourite(source, time.Time{})
	return ok
}

// startFavourite sends a favourite to the speaker that owns playback, and
// says what happened.
func startFavourite(speaker speakerModel, playable sonos.Playable, action favouriteAction, station bool) tea.Cmd {
	return func() tea.Msg {
		device := speaker.transport()
		var err error
		var said string
		switch {
		case action == favouritePlay && station:
			err, said = device.PlayStation(speaker.ctx, playable), "playing "+playable.Title
		case action == favouritePlay:
			err, said = device.ReplaceQueue(speaker.ctx, playable, sonos.PlayModeNormal), "playing "+playable.Title
		case action == favouritePlayNext:
			err, said = device.PlayNext(speaker.ctx, playable), playable.Title+" plays next"
		default:
			err, said = device.AddToQueue(speaker.ctx, playable), "added "+playable.Title+" to the queue"
		}
		if err != nil {
			slog.Warn("starting a favourite failed", "speaker", speaker.device.Speaker, "favourite", playable.Title, "err", err)
			return speaker.failed(err)
		}
		return withConfirmation(speaker.to(nowplaying.TrackMsg{Track: trackFromState(speaker.shown())}), said)
	}
}

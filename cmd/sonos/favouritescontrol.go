// How the application drives the favourites modal: opening it, reading the
// favourites and the media type shown last, and starting a favourite on the
// speaker in view. favourites.go holds the list itself.
package main

import (
	"context"
	"errors"
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

package main

// TODO: TEMPORARY. Two hard-coded playlists on z, Z, c and C, to prove a
// playlist can be loaded from kept identifiers. Delete this file, its test,
// its call in speaker.go and its help rows once favourites can do the same.

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/haasted/sonos-sandbox/internal/ui/nowplaying"
)

// TODO: TEMPORARY, see the top of this file.
var (
	trialAwesomeSingles = sonos.ServiceContainer{
		Kind:    sonos.ContainerPlaylist,
		ID:      "libraryplaylist:p.V7VYpk7FYWLeW",
		Title:   "Awesome singles",
		Service: 204,
		Account: 2,
	}
	trialMetallica = sonos.ServiceContainer{
		Kind:    sonos.ContainerPlaylist,
		ID:      "libraryplaylist:p.ZOAXddOUZE1LE",
		Title:   "Metallica",
		Service: 204,
		Account: 2,
	}
)

// trialChoice is the playlist and mode a key plays, or false for any other
// key. TODO: TEMPORARY, see the top of this file.
func trialChoice(key string) (sonos.ServiceContainer, sonos.PlayMode, bool) {
	switch key {
	case "z":
		return trialAwesomeSingles, sonos.PlayModeNormal, true
	case "Z":
		return trialAwesomeSingles, sonos.PlayModeShuffleNoRepeat, true
	case "c":
		return trialMetallica, sonos.PlayModeNormal, true
	case "C":
		return trialMetallica, sonos.PlayModeShuffleNoRepeat, true
	}
	return sonos.ServiceContainer{}, "", false
}

// trialKey replaces the queue with the key's playlist and plays it, or
// reports false for any other key. TODO: TEMPORARY, see the top of this file.
func (m speakerModel) trialKey(key string) (tea.Cmd, bool) {
	playlist, mode, ok := trialChoice(key)
	if !ok {
		return nil, false
	}
	return func() tea.Msg {
		if err := m.transport().ReplaceQueue(m.ctx, playlist, mode); err != nil {
			slog.Warn("playing a trial playlist failed",
				"speaker", m.device.Speaker, "playlist", playlist.Title, "err", err)
			return m.failed(err)
		}
		said := "playing " + playlist.Title
		if mode == sonos.PlayModeShuffleNoRepeat {
			said = "shuffling " + playlist.Title
		}
		return withConfirmation(m.to(nowplaying.TrackMsg{Track: trackFromState(m.shown())}), said)
	}, true
}

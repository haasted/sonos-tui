// Starting something on a speaker: a favourite or a search result, played at
// once, after the playing track, or at the end of the queue.
package main

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-tui/internal/sonos"
	"github.com/haasted/sonos-tui/internal/ui/nowplaying"
)

// startAction is what a key asks to do with the favourite or search result
// under the cursor.
type startAction int

const (
	startPlay         startAction = iota // replace the queue and play; a station just plays
	startPlayNext                        // put it after the playing track
	startAddToQueue                      // put it at the end of the queue
	startPlayShuffled                    // replace the queue and play it shuffled; an album or playlist only
)

// startPlayable sends a favourite or a search result to the speaker that
// owns playback, and says what happened.
func startPlayable(speaker speakerModel, playable sonos.Playable, action startAction, station bool) tea.Cmd {
	return func() tea.Msg {
		device := speaker.transport()
		var err error
		var said string
		switch {
		case action == startPlay && station:
			err, said = device.PlayStation(speaker.ctx, playable), "playing "+playable.Title
		case action == startPlay:
			err, said = device.ReplaceQueue(speaker.ctx, playable, sonos.PlayModeNormal), "playing "+playable.Title
		case action == startPlayShuffled:
			err, said = device.ReplaceQueue(speaker.ctx, playable, sonos.PlayModeShuffleNoRepeat), "playing "+playable.Title+" shuffled"
		case action == startPlayNext:
			err, said = device.PlayNext(speaker.ctx, playable), playable.Title+" plays next"
		default:
			err, said = device.AddToQueue(speaker.ctx, playable), "added "+playable.Title+" to the queue"
		}
		if err != nil {
			slog.Warn("starting to play failed", "speaker", speaker.device.Speaker, "title", playable.Title, "err", err)
			return speaker.failed(err)
		}
		return withConfirmation(speaker.to(nowplaying.TrackMsg{Track: trackFromState(speaker.shown())}), said)
	}
}

// startAlbumFrom replaces the queue with an album and plays it from the track
// at position, counted from 1, and says what happened.
func startAlbumFrom(speaker speakerModel, album sonos.Playable, position int, track string) tea.Cmd {
	return func() tea.Msg {
		if err := speaker.transport().ReplaceQueueFrom(speaker.ctx, album, position); err != nil {
			slog.Warn("starting an album from a track failed", "speaker", speaker.device.Speaker, "title", album.Title, "err", err)
			return speaker.failed(err)
		}
		said := "playing " + album.Title + " from " + track
		return withConfirmation(speaker.to(nowplaying.TrackMsg{Track: trackFromState(speaker.shown())}), said)
	}
}

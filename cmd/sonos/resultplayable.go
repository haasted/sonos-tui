// How a result from the Sonos cloud becomes something a speaker plays, and a
// favourite. The two libraries know nothing of each other, so the step
// between them is here.
package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/haasted/sonos-tui/internal/sonos"
	"github.com/haasted/sonos-tui/internal/sonoscloud"
	"github.com/haasted/sonos-tui/internal/store"
)

// serviceTypeOffset is what the content API's service type adds to the sid
// the speakers use, after it is shifted: type = sid × 256 + 7.
const serviceTypeOffset = 7

// resultPlayable is what a speaker is given for a search result: an album as
// a service container, a track on its own. Other types cannot be played yet.
func resultPlayable(r sonoscloud.Result) (sonos.Playable, error) {
	service, account, err := speakerService(r.ID)
	if err != nil {
		return sonos.Playable{}, fmt.Errorf("playing %q: %w", r.Name, err)
	}
	switch r.Type {
	case sonoscloud.TypeAlbum:
		return sonos.ServiceContainer{
			Kind: sonos.ContainerAlbum, ID: r.ID.ObjectID, Title: r.Name, Service: service, Account: account,
		}.Playable()
	case sonoscloud.TypeTrack:
		track := sonos.ServiceTrack{
			ID: r.ID.ObjectID, Title: r.Name, Artist: artistNames(r), Service: service, Account: account,
		}
		if r.Album != nil {
			track.Album = r.Album.Name
		}
		return track.Playable()
	default:
		return sonos.Playable{}, fmt.Errorf("playing %q: a %s cannot be played yet", r.Name, strings.ToLower(string(r.Type)))
	}
}

// resultFavourite is a search result as a favourite. The service, account and
// item id are read from the URI it plays by, as for a favourite marked from
// the queue, so the same track or album is the same favourite either way.
func resultFavourite(r sonoscloud.Result, now time.Time) (store.Favourite, error) {
	playable, err := resultPlayable(r)
	if err != nil {
		return store.Favourite{}, err
	}
	ref, ok := sonos.ParseServiceURI(playable.URI)
	if !ok {
		return store.Favourite{}, fmt.Errorf("marking %q: %q names no service item", r.Name, playable.URI)
	}
	fav := store.Favourite{
		Kind:     store.FavouriteAlbum,
		Title:    r.Name,
		Artist:   artistNames(r),
		URI:      playable.URI,
		Metadata: playable.Metadata,
		Service:  ref.Service,
		Account:  ref.Account,
		ItemID:   ref.ItemID,
		AddedAt:  now,
	}
	if r.Type == sonoscloud.TypeTrack {
		fav.Kind = store.FavouriteTrack
		if r.Album != nil {
			fav.Album = r.Album.Name
		}
	}
	return fav, nil
}

// speakerService is a result's service as the speakers number it (sid), and
// the household's account on it (sn), which the content API calls accountId.
func speakerService(id sonoscloud.ResultID) (service, account int, err error) {
	serviceType, err := strconv.Atoi(id.ServiceID)
	if err != nil || serviceType&0xff != serviceTypeOffset {
		return 0, 0, fmt.Errorf("unknown service %q", id.ServiceID)
	}
	account, err = strconv.Atoi(id.AccountID)
	if err != nil || account <= 0 {
		return 0, 0, fmt.Errorf("unknown account %q", id.AccountID)
	}
	return serviceType >> 8, account, nil
}

// artistNames is a result's artists, as one line.
func artistNames(r sonoscloud.Result) string {
	names := make([]string, 0, len(r.Artists))
	for _, a := range r.Artists {
		names = append(names, a.Name)
	}
	return strings.Join(names, ", ")
}

package main

import (
	"fmt"
	"time"

	"github.com/haasted/sonos-tui/internal/store"
)

// mockFavourites is a made-up favourites list across every media type,
// newest first. The ids are real Apple Music ids, so a later step can browse
// into them through the web app.
func mockFavourites() []store.Favourite {
	type row struct {
		kind          store.FavouriteKind
		title, artist string
		itemID        string
	}
	rows := []row{
		{store.FavouriteAlbum, "Swordfishtrombones (2023 Remaster)", "Tom Waits", "album:1695048997"},
		{store.FavouriteTrack, "Golden Years", "David Bowie", "song:1195105202"},
		{store.FavouriteAlbum, "Station to Station (2016 Remaster)", "David Bowie", "album:1195104858"},
		{store.FavouritePlaylist, "David Bowie Essentials", "", "playlist:pl.606df299487e4c0e9022793836927a49"},
		{store.FavouriteAlbum, "Hyperspace (2020)", "Beck", "album:1527187716"},
		{store.FavouriteTrack, "Life On Mars?", "David Bowie", "song:1039798013"},
		{store.FavouriteAlbum, "Blackstar", "David Bowie", "album:1676286487"},
		{store.FavouritePlaylist, "Bowie in Berlin", "", "playlist:pl.18df52da188a4d39be68a43ae1456771"},
		{store.FavouriteStation, "David Bowie & Similar Artists", "", "radio:ra.551695"},
		{store.FavouriteAlbum, "Rain Dogs (2023 Remaster)", "Tom Waits", "album:1695046955"},
		{store.FavouriteTrack, "Lazarus", "David Bowie", "song:1676286742"},
		{store.FavouriteAlbum, "Closing Time (Remastered)", "Tom Waits", "album:1485071375"},
		{store.FavouriteStation, "Tom Waits Radio", "", "radio:ra.83964"},
		{store.FavouriteAlbum, "Colors", "Beck", "album:1440881121"},
		{store.FavouriteTrack, "Downtown Train (2023 Remaster)", "Tom Waits", "song:1695048155"},
		{store.FavouriteAlbum, "Low (2017 Remaster)", "David Bowie", "album:1347895593"},
		{store.FavouriteAlbum, "Be Here Now - Single", "Beck", "album:1783905690"},
		{store.FavouriteAlbum, "Big Time (Live)", "Tom Waits", "album:1443540897"},
		{store.FavouriteAlbum, "Boots on the Ground - Single", "Massive Attack & Tom Waits", "album:1886123355"},
		{store.FavouriteAlbum, "Grave Diggers: Tom Waits - EP", "Tom Waits", "album:1650272960"},
		{store.FavouriteAlbum, "Saw Lightning - Single", "Beck", "album:1460177651"},
	}
	at := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	favourites := make([]store.Favourite, len(rows))
	for i, r := range rows {
		favourites[i] = store.Favourite{
			ID: int64(len(rows) - i), Kind: r.kind, Title: r.title, Artist: r.artist,
			Service: 204, Account: 2, ItemID: r.itemID, AddedAt: at.Add(-time.Duration(i) * time.Hour),
		}
	}
	return favourites
}

// knownTracks are the real track lists of some mock favourites, so browsing
// them looks like the real thing. The rest get placeholder tracks.
var knownTracks = map[string][]string{
	"album:1195104858": {"Station to Station", "Golden Years", "Word on a Wing", "TVC15", "Stay", "Wild Is the Wind"},
	"album:1695048997": {"Underground", "Shore Leave", "Dave the Butcher", "Johnsburg, Illinois", "16 Shells From a 30.6",
		"Town With No Cheer", "In the Neighborhood", "Just Another Sucker on the Vine", "Frank's Wild Years",
		"Swordfishtrombone", "Down, Down, Down", "Soldier's Things", "Gin Soaked Boy", "Trouble's Braids", "Rainbirds"},
	"album:1676286487": {"★", "'Tis a Pity She Was a Whore", "Lazarus", "Sue (Or in a Season of Crime)",
		"Girl Loves Me", "Dollar Days", "I Can't Give Everything Away"},
	"playlist:pl.606df299487e4c0e9022793836927a49": {"Heroes", "Life On Mars?", "Space Oddity", "Starman", "Let's Dance",
		"Changes", "Ziggy Stardust", "Ashes to Ashes", "Under Pressure", "Rebel Rebel", "Modern Love", "China Girl",
		"Golden Years", "Fame", "Sound and Vision", "The Jean Genie", "Young Americans", "Lazarus", "Blackstar",
		"Where Are We Now?", "Suffragette City", "Moonage Daydream", "Rock 'n' Roll Suicide", "Station to Station"},
}

// childrenOf is what a mock favourite holds, as tracks: its real track list
// when it is known, or a few placeholders.
func childrenOf(parent store.Favourite) []store.Favourite {
	titles := knownTracks[parent.ItemID]
	if titles == nil {
		for i := range 8 {
			titles = append(titles, fmt.Sprintf("Track %d", i+1))
		}
	}
	artist := parent.Artist
	if artist == "" {
		artist = "David Bowie" // the mock playlists are all Bowie's
	}
	children := make([]store.Favourite, len(titles))
	for i, title := range titles {
		children[i] = store.Favourite{ID: int64(1000 + i), Kind: store.FavouriteTrack, Title: title, Artist: artist}
	}
	return children
}

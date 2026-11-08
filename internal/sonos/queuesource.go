package sonos

import "slices"

// VariousArtists is the artist of an album whose tracks share no artist.
const VariousArtists = "Various Artists"

// QueueSource is the playlist or album a speaker's queue was last started
// from, as the speaker reports it in r:EnqueuedTransportURI and its metadata.
// Only a replace changes it: an album added to the end later is not named, so
// it says what the queue started as, not all it holds.
type QueueSource struct {
	Title    string
	Creator  string // the album's artist; empty for a playlist
	Class    string // the DIDL class, such as object.container.playlistContainer
	URI      string
	Metadata string // the DIDL as the speaker sent it, desc included
}

// Kind says whether the source is a playlist or an album, or false for a
// container that is neither.
func (s QueueSource) Kind() (ContainerKind, bool) {
	for kind, class := range containerClasses {
		if s.Class == class {
			return kind, true
		}
	}
	return "", false
}

// AlbumArtist is the artist that more than half of an album's tracks share,
// or VariousArtists when no artist does. Only the tracks from the album are
// counted, if any are; a queue can hold tracks added after it.
func AlbumArtist(album string, tracks []TrackMetadata) string {
	from := slices.DeleteFunc(slices.Clone(tracks), func(t TrackMetadata) bool { return t.Album != album })
	if len(from) == 0 {
		from = tracks
	}
	counts := map[string]int{}
	known := 0
	for _, track := range from {
		if track.Artist != "" {
			counts[track.Artist]++
			known++
		}
	}
	if known == 0 {
		return ""
	}
	for artist, count := range counts {
		if count*2 > known {
			return artist
		}
	}
	return VariousArtists
}

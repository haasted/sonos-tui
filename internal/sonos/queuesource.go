package sonos

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

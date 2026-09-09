package sonos

// Exported wrappers for internal functions, available only during testing.
// This file is compiled exclusively when running `go test`.

var ExtractHeader = extractHeader

const EventChanBuffer = eventChanBuffer

// CallbackBaseURL is the listener's own address, so a test can post a NOTIFY
// to it the way a speaker would. Test-only: production code has no reason to
// know where the listener sits, only to hand out per-subscription URLs.
func (em *EventManager) CallbackBaseURL() string { return em.listener.baseURL }

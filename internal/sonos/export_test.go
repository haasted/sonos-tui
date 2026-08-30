package sonos

// Exported wrappers for internal functions, available only during testing.
// This file is compiled exclusively when running `go test`.

var ExtractHeader = extractHeader

const EventChanBuffer = eventChanBuffer

package sonos_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: the request itself, which must name the track by a 1-based queue
// position rather than a time offset or an absolute track URI.
func TestSeekTrack_AddressesTheTrackByPosition(t *testing.T) {
	body, server := captureRequest(t)
	defer server.Close()

	require.NoError(t, clientFor(server).SeekTrack(context.Background(), 7))

	assert.Contains(t, *body, "<Unit>TRACK_NR</Unit>")
	assert.Contains(t, *body, "<Target>7</Target>")
	assert.Contains(t, *body, "u:Seek", "the action is Seek")
}

// Case: a position that is not a track. A queue counts from one.
func TestSeekTrack_RefusesPositionsThatAreNotTracks(t *testing.T) {
	server := serverThatMustNotBeCalled(t)
	defer server.Close()

	assert.Error(t, clientFor(server).SeekTrack(context.Background(), 0))
	assert.Error(t, clientFor(server).SeekTrack(context.Background(), -1))
}

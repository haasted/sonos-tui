package sonos_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: moving a track DOWN, which needs the off-by-one. InsertBefore is read
// against the queue with the track still in it, so naming the destination
// alone would land the track one place short.
func TestMoveQueueTrack_MovingDownNamesOnePlaceFurther(t *testing.T) {
	body, server := captureRequest(t)
	defer server.Close()

	_, err := clientFor(server).MoveQueueTrack(context.Background(), 3, 7, 12)
	require.NoError(t, err)

	assert.Contains(t, *body, "<StartingIndex>3</StartingIndex>")
	assert.Contains(t, *body, "<InsertBefore>8</InsertBefore>", "7 is short by one while the track is still at 3")
	assert.Contains(t, *body, "<NumberOfTracks>1</NumberOfTracks>")
	assert.Contains(t, *body, "<UpdateID>12</UpdateID>")
}

// Case: moving a track UP, which needs no correction: everything between the
// track and its destination shifts down rather than up.
func TestMoveQueueTrack_MovingUpNamesTheDestination(t *testing.T) {
	body, server := captureRequest(t)
	defer server.Close()

	_, err := clientFor(server).MoveQueueTrack(context.Background(), 7, 3, 12)
	require.NoError(t, err)

	assert.Contains(t, *body, "<StartingIndex>7</StartingIndex>")
	assert.Contains(t, *body, "<InsertBefore>3</InsertBefore>")
}

// Case: a move of one place in each direction, which is where an off-by-one
// hides. Down by one must not be a move to nowhere.
func TestMoveQueueTrack_ASinglePlaceInEitherDirection(t *testing.T) {
	down, server := captureRequest(t)
	_, err := clientFor(server).MoveQueueTrack(context.Background(), 4, 5, 0)
	require.NoError(t, err)
	assert.Contains(t, *down, "<InsertBefore>6</InsertBefore>", "4 to 5 must clear the track's own place")
	server.Close()

	up, server := captureRequest(t)
	defer server.Close()
	_, err = clientFor(server).MoveQueueTrack(context.Background(), 5, 4, 0)
	require.NoError(t, err)
	assert.Contains(t, *up, "<InsertBefore>4</InsertBefore>")
}

// Case: the service it is sent to. The reorder that answers with a version
// lives on the Queue service, not on AVTransport, and the two are different
// actions rather than two names for one.
func TestMoveQueueTrack_GoesToTheQueueService(t *testing.T) {
	body, path, server := captureCall(t)
	defer server.Close()

	_, err := clientFor(server).MoveQueueTrack(context.Background(), 3, 7, 12)
	require.NoError(t, err)

	assert.Equal(t, "/MediaRenderer/Queue/Control", *path)
	assert.Contains(t, *body, "u:ReorderTracks")
	assert.Contains(t, *body, "<QueueID>0</QueueID>", "0 is the speaker's own queue")
}

// Case: the version the speaker answers with, which is the whole reason for
// using this service. Without it a second write carries a stale version and
// is refused.
func TestMoveQueueTrack_ReturnsTheVersionTheSpeakerMovedTo(t *testing.T) {
	server := soapServer(t, 41)
	defer server.Close()

	version, err := clientFor(server).MoveQueueTrack(context.Background(), 3, 7, 40)
	require.NoError(t, err)
	assert.Equal(t, 41, version)
}

// Case: a move that is not one. Sending it would cost a request and an event
// for a queue nobody changed. The version is handed back untouched, so a run
// of moves can carry it on rather than treating a no-op as a break.
func TestMoveQueueTrack_AMoveToTheSamePlaceSendsNothing(t *testing.T) {
	server := serverThatMustNotBeCalled(t)
	defer server.Close()

	version, err := clientFor(server).MoveQueueTrack(context.Background(), 4, 4, 12)
	assert.NoError(t, err)
	assert.Equal(t, 12, version, "the caller's version must survive a move that sends nothing")
}

// Case: positions that are not tracks. A queue counts from one.
func TestMoveQueueTrack_RefusesPositionsThatAreNotTracks(t *testing.T) {
	server := serverThatMustNotBeCalled(t)
	defer server.Close()

	_, err := clientFor(server).MoveQueueTrack(context.Background(), 0, 3, 0)
	assert.Error(t, err)
	_, err = clientFor(server).MoveQueueTrack(context.Background(), 3, 0, 0)
	assert.Error(t, err)
}

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

	require.NoError(t, clientFor(server).MoveQueueTrack(context.Background(), 3, 7, 12))

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

	require.NoError(t, clientFor(server).MoveQueueTrack(context.Background(), 7, 3, 12))

	assert.Contains(t, *body, "<StartingIndex>7</StartingIndex>")
	assert.Contains(t, *body, "<InsertBefore>3</InsertBefore>")
}

// Case: a move of one place in each direction, which is where an off-by-one
// hides. Down by one must not be a move to nowhere.
func TestMoveQueueTrack_ASinglePlaceInEitherDirection(t *testing.T) {
	down, server := captureRequest(t)
	require.NoError(t, clientFor(server).MoveQueueTrack(context.Background(), 4, 5, 0))
	assert.Contains(t, *down, "<InsertBefore>6</InsertBefore>", "4 to 5 must clear the track's own place")
	server.Close()

	up, server := captureRequest(t)
	defer server.Close()
	require.NoError(t, clientFor(server).MoveQueueTrack(context.Background(), 5, 4, 0))
	assert.Contains(t, *up, "<InsertBefore>4</InsertBefore>")
}

// Case: a move that is not one. Sending it would cost a request and an event
// for a queue nobody changed.
func TestMoveQueueTrack_AMoveToTheSamePlaceSendsNothing(t *testing.T) {
	server := serverThatMustNotBeCalled(t)
	defer server.Close()

	assert.NoError(t, clientFor(server).MoveQueueTrack(context.Background(), 4, 4, 0))
}

// Case: positions that are not tracks. A queue counts from one.
func TestMoveQueueTrack_RefusesPositionsThatAreNotTracks(t *testing.T) {
	server := serverThatMustNotBeCalled(t)
	defer server.Close()

	assert.Error(t, clientFor(server).MoveQueueTrack(context.Background(), 0, 3, 0))
	assert.Error(t, clientFor(server).MoveQueueTrack(context.Background(), 3, 0, 0))
}

package sonos_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-tui/internal/sonos"
)

// Case: the queue changing, which is the event this service was subscribed to
// for. ContainerUpdateIDs is positional pairs, and the container id holds a
// colon of its own.
func TestContentDirectoryEvent_NamesTheQueue(t *testing.T) {
	event, err := sonos.ParseContentDirectoryEvent(propertyset(
		`<ContainerUpdateIDs>Q:0,42</ContainerUpdateIDs>`))
	require.NoError(t, err)

	updateID, named := event.QueueUpdate()
	assert.True(t, named, "an event naming Q:0 is an event about the queue")
	assert.Equal(t, 42, updateID)
}

// Case: an event about some other container. It says nothing about the queue,
// which is not the same as saying the queue is unchanged.
func TestContentDirectoryEvent_AnotherContainerIsNotTheQueue(t *testing.T) {
	event, err := sonos.ParseContentDirectoryEvent(propertyset(
		`<ContainerUpdateIDs>SQ:3,7</ContainerUpdateIDs>`))
	require.NoError(t, err)

	_, named := event.QueueUpdate()
	assert.False(t, named, "SQ:3 is a saved queue, not the speaker's own")
}

// Case: several containers in one event, which the speaker sends as one flat
// list rather than as a property each.
func TestContentDirectoryEvent_ReadsEveryPair(t *testing.T) {
	event, err := sonos.ParseContentDirectoryEvent(propertyset(
		`<ContainerUpdateIDs>SQ:3,7,Q:0,42,R:0/0,9</ContainerUpdateIDs>`))
	require.NoError(t, err)

	require.Len(t, event.Containers, 3)
	assert.Equal(t, sonos.ContainerUpdate{ID: "SQ:3", UpdateID: 7}, event.Containers[0])
	assert.Equal(t, sonos.ContainerUpdate{ID: "Q:0", UpdateID: 42}, event.Containers[1])
	assert.Equal(t, sonos.ContainerUpdate{ID: "R:0/0", UpdateID: 9}, event.Containers[2])
}

// Case: the system-wide counter, which rises on any change and so cannot say
// what changed.
func TestContentDirectoryEvent_ReadsSystemUpdateID(t *testing.T) {
	event, err := sonos.ParseContentDirectoryEvent(propertyset(
		`<SystemUpdateID>118</SystemUpdateID>`))
	require.NoError(t, err)

	assert.True(t, event.HasSystem)
	assert.Equal(t, 118, event.SystemUpdateID)
	assert.Empty(t, event.Containers, "a system change names no container")
}

// Case: a property this package does not know. Sonos sends several of its own,
// and one of them must not cost us the properties beside it.
func TestContentDirectoryEvent_IgnoresPropertiesItDoesNotKnow(t *testing.T) {
	event, err := sonos.ParseContentDirectoryEvent(propertyset(
		`<FavoritesUpdateID>RINCON_1,12</FavoritesUpdateID>`,
		`<ShareListUpdateID>ABC,3</ShareListUpdateID>`,
		`<ContainerUpdateIDs>Q:0,5</ContainerUpdateIDs>`))
	require.NoError(t, err)

	updateID, named := event.QueueUpdate()
	assert.True(t, named)
	assert.Equal(t, 5, updateID)
}

// Case: a list that ends mid-pair. The pairs before it are still true, and
// they are the ones a caller acts on.
func TestContentDirectoryEvent_KeepsWhatItCanReadFromABrokenList(t *testing.T) {
	event, err := sonos.ParseContentDirectoryEvent(propertyset(
		`<ContainerUpdateIDs>Q:0,42,SQ:3</ContainerUpdateIDs>`))
	require.NoError(t, err)

	updateID, named := event.QueueUpdate()
	assert.True(t, named)
	assert.Equal(t, 42, updateID)
	assert.Len(t, event.Containers, 1, "the dangling id is dropped, not guessed at")
}

// Case: an update id that is not a number, which must not become zero — zero
// is a real id, and a queue read at it would look current.
func TestContentDirectoryEvent_DropsAnUnreadableUpdateID(t *testing.T) {
	event, err := sonos.ParseContentDirectoryEvent(propertyset(
		`<ContainerUpdateIDs>Q:0,later</ContainerUpdateIDs>`))
	require.NoError(t, err)

	_, named := event.QueueUpdate()
	assert.False(t, named, "an unreadable id must not read as update 0")
}

// Case: a body that is not a propertyset at all.
func TestContentDirectoryEvent_RejectsRubbish(t *testing.T) {
	_, err := sonos.ParseContentDirectoryEvent([]byte("not xml at all"))
	assert.Error(t, err)
}

// propertyset wraps properties in the GENA envelope a NOTIFY arrives in.
func propertyset(properties ...string) []byte {
	body := `<?xml version="1.0"?><e:propertyset xmlns:e="urn:schemas-upnp-org:event-1-0">`
	for _, property := range properties {
		body += "<e:property>" + property + "</e:property>"
	}
	return []byte(body + "</e:propertyset>")
}

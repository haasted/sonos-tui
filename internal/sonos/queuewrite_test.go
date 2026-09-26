package sonos_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a run of writes with nothing read between them, which is what a
// reader pressing a key twice quickly produces. The second must carry the
// version the first created. The speaker refuses a superseded version, and
// the event that would otherwise teach the new one arrives long after the
// second keypress -- so without this the second edit fails rather than lags.
func TestQueueWrite_ASecondWriteCarriesTheVersionTheFirstReturned(t *testing.T) {
	sent, server := versionCounter(t, 90)
	defer server.Close()

	device := deviceFor(server)
	require.NoError(t, device.RemoveQueueTrack(context.Background(), 3))
	require.NoError(t, device.RemoveQueueTrack(context.Background(), 5))

	assert.Equal(t, []int{0, 91}, *sent,
		"the second removal must send what the first answered, not the version it started with")
	assert.Equal(t, 92, device.State().QueueUpdateID)
}

// Case: the same across the two kinds of write. A move and a removal share
// one version, so a move following a removal is refused unless the removal's
// answer was kept.
func TestQueueWrite_AMoveCarriesTheVersionARemovalReturned(t *testing.T) {
	sent, server := versionCounter(t, 40)
	defer server.Close()

	device := deviceFor(server)
	require.NoError(t, device.RemoveQueueTrack(context.Background(), 2))
	require.NoError(t, device.MoveQueueTrack(context.Background(), 4, 7))

	assert.Equal(t, []int{0, 41}, *sent)
}

// Case: a write the speaker refuses. The stored version must not move: the
// refusal means the speaker is somewhere this device does not know about, and
// the read that follows is what finds out where.
func TestQueueWrite_ARefusedWriteLeavesTheVersionAlone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
			`<s:Body><s:Fault><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0">`+
			`<errorCode>1028</errorCode></UPnPError></detail></s:Fault></s:Body></s:Envelope>`)
	}))
	defer server.Close()

	device := deviceFor(server)
	require.Error(t, device.RemoveQueueTrack(context.Background(), 3))
	assert.Equal(t, 0, device.State().QueueUpdateID)
}

// versionCounter answers each write with the next version, and records the
// version each request carried.
func versionCounter(t *testing.T, from int) (*[]int, *httptest.Server) {
	t.Helper()

	var sent []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		sent = append(sent, updateIDIn(t, string(raw)))
		writeSOAP(w, from+len(sent))
	}))
	return &sent, server
}

var updateIDPattern = regexp.MustCompile(`<UpdateID>(-?\d+)</UpdateID>`)

func updateIDIn(t *testing.T, body string) int {
	t.Helper()

	match := updateIDPattern.FindStringSubmatch(body)
	require.NotNil(t, match, "the request carried no UpdateID: %s", body)
	version, err := strconv.Atoi(match[1])
	require.NoError(t, err)
	return version
}

// Case: a selection with gaps in it, which is several calls. They must go
// from the bottom of the queue upwards: removing a track shifts every
// position below it, so going the other way would send positions that have
// moved. Adjacent tracks go as one run rather than one call each.
func TestRemoveQueueTracks_TakesRunsFromTheBottomUp(t *testing.T) {
	runs, server := runRecorder(t)
	defer server.Close()

	removed, err := deviceFor(server).RemoveQueueTracks(context.Background(), []int{3, 4, 5, 9, 11})
	require.NoError(t, err)

	assert.Equal(t, 5, removed)
	assert.Equal(t, [][2]int{{11, 1}, {9, 1}, {3, 3}}, *runs,
		"the runs must be highest first, and 3 to 5 must go in one call")
}

// Case: positions given in no order, which is what a reader picking rows out
// of sequence produces. The order they arrive in must not reach the speaker.
func TestRemoveQueueTracks_SortsWhatItIsGiven(t *testing.T) {
	runs, server := runRecorder(t)
	defer server.Close()

	_, err := deviceFor(server).RemoveQueueTracks(context.Background(), []int{9, 3, 11, 5, 4})
	require.NoError(t, err)

	assert.Equal(t, [][2]int{{11, 1}, {9, 1}, {3, 3}}, *runs)
}

// Case: a position named twice. Removing it twice would take the track that
// moved into its place, which is the bug this whole scheme exists to avoid.
func TestRemoveQueueTracks_RemovesAPositionOnlyOnce(t *testing.T) {
	runs, server := runRecorder(t)
	defer server.Close()

	removed, err := deviceFor(server).RemoveQueueTracks(context.Background(), []int{7, 7})
	require.NoError(t, err)

	assert.Equal(t, 1, removed)
	assert.Equal(t, [][2]int{{7, 1}}, *runs)
}

// Case: nothing to remove. It must not reach the speaker at all.
func TestRemoveQueueTracks_SendsNothingForAnEmptySelection(t *testing.T) {
	server := serverThatMustNotBeCalled(t)
	defer server.Close()

	removed, err := deviceFor(server).RemoveQueueTracks(context.Background(), nil)
	assert.NoError(t, err)
	assert.Zero(t, removed)
}

// Case: a speaker that refuses part way. The count says how many really left,
// which is what the reader has to be told: the queue is now in a state nobody
// asked for.
func TestRemoveQueueTracks_ReportsHowManyLeftBeforeARefusal(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		calls++
		if calls > 1 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
				`<s:Body><s:Fault><detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0">`+
				`<errorCode>1028</errorCode></UPnPError></detail></s:Fault></s:Body></s:Envelope>`)
			return
		}
		writeSOAP(w, 5)
	}))
	defer server.Close()

	// Position 11 goes, and the run at 3 is refused.
	removed, err := deviceFor(server).RemoveQueueTracks(context.Background(), []int{3, 4, 5, 11})
	require.Error(t, err)
	assert.Equal(t, 1, removed, "only the first run left the queue")
}

// runRecorder records the run each request asks for, as start and count.
func runRecorder(t *testing.T) (*[][2]int, *httptest.Server) {
	t.Helper()

	var runs [][2]int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		body := string(raw)
		runs = append(runs, [2]int{numberIn(t, body, "StartingIndex"), numberIn(t, body, "NumberOfTracks")})
		writeSOAP(w, len(runs))
	}))
	return &runs, server
}

func numberIn(t *testing.T, body, element string) int {
	t.Helper()

	pattern := regexp.MustCompile(`<` + element + `>(-?\d+)</` + element + `>`)
	match := pattern.FindStringSubmatch(body)
	require.NotNil(t, match, "the request carried no %s: %s", element, body)
	value, err := strconv.Atoi(match[1])
	require.NoError(t, err)
	return value
}

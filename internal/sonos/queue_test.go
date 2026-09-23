package sonos_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: a queue longer than one Browse page. Browse returns at most
// RequestedCount items, so a single call is correct only by luck on a short
// queue — the failure mode is silently returning the first page and looking
// perfectly fine until someone loads a real playlist.
func TestQueue_PagesUntilItHasEverything(t *testing.T) {
	const total = 250
	server, calls := browseServer(t, total)
	defer server.Close()

	queue, err := clientFor(server).GetQueue(context.Background())
	require.NoError(t, err)

	assert.Len(t, queue.Items, total, "every page must be fetched, not just the first")
	assert.Greater(t, *calls, 1, "a 250-item queue cannot arrive in one page")
	assert.Equal(t, "Track 1", queue.Items[0].Title)
	assert.Equal(t, "Track 250", queue.Items[total-1].Title)
}

// Case: positions, which must line up with AVTransport's 1-based CurrentTrack
// so the two can be compared without an off-by-one.
func TestQueue_PositionsAreOneBasedAndContiguous(t *testing.T) {
	server, _ := browseServer(t, 5)
	defer server.Close()

	queue, err := clientFor(server).GetQueue(context.Background())
	require.NoError(t, err)

	for i, item := range queue.Items {
		assert.Equal(t, i+1, item.Position)
	}
}

// Case: an empty queue, which a speaker playing radio may well report. It must
// come back empty rather than erroring or looping.
func TestQueue_EmptyQueueReturnsNothing(t *testing.T) {
	server, _ := browseServer(t, 0)
	defer server.Close()

	queue, err := clientFor(server).GetQueue(context.Background())
	require.NoError(t, err)

	assert.Empty(t, queue.Items)
}

// Case: a speaker claiming a total it will not serve. Paging on Total alone
// would spin forever, so an empty page has to end the loop.
func TestQueue_StopsWhenAPageComesBackEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, browseResponse("", 0, 999))
	}))
	defer server.Close()

	queue, err := clientFor(server).GetQueue(context.Background())
	require.NoError(t, err)

	assert.Empty(t, queue.Items, "a claimed total must not produce phantom items")
}

// Case: the DIDL document, which carries many items. Parsing it the way a
// single track is parsed keeps only the first and loses the rest silently.
func TestQueue_EveryItemInTheDocumentIsParsed(t *testing.T) {
	server, _ := browseServer(t, 3)
	defer server.Close()

	queue, err := clientFor(server).GetQueue(context.Background())
	require.NoError(t, err)

	require.Len(t, queue.Items, 3)
	assert.Equal(t, "Artist 2", queue.Items[1].Artist)
	assert.Equal(t, "Album 2", queue.Items[1].Album)
	assert.Equal(t, "0:03:21", queue.Items[1].Duration)
	assert.Contains(t, queue.Items[1].URI, "track-2")
}

func clientFor(server *httptest.Server) *sonos.SonosClient {
	return &sonos.SonosClient{BaseURL: server.URL, HTTPClient: server.Client()}
}

// browseServer serves a queue of n items, honouring StartingIndex so paging is
// actually exercised rather than assumed.
func browseServer(t *testing.T, n int) (*httptest.Server, *int) {
	t.Helper()

	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		body := string(raw)

		start := valueBetween(body, "<StartingIndex>", "</StartingIndex>")
		count := valueBetween(body, "<RequestedCount>", "</RequestedCount>")

		var didl strings.Builder
		didl.WriteString(`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">`)
		served := 0
		for i := start; i < n && served < count; i++ {
			fmt.Fprintf(&didl, `<item id="Q:0/%d"><res duration="0:03:21">x-sonos-http:track-%d.mp3</res>`+
				`<dc:title>Track %d</dc:title><dc:creator>Artist %d</dc:creator><upnp:album>Album %d</upnp:album></item>`,
				i+1, i+1, i+1, i+1, i+1)
			served++
		}
		didl.WriteString(`</DIDL-Lite>`)

		fmt.Fprint(w, browseResponse(didl.String(), served, n))
	}))
	return server, &calls
}

func browseResponse(didl string, returned, total int) string {
	escaped := strings.NewReplacer("<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(didl)
	return fmt.Sprintf(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <u:BrowseResponse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">
      <Result>%s</Result>
      <NumberReturned>%d</NumberReturned>
      <TotalMatches>%d</TotalMatches>
      <UpdateID>1</UpdateID>
    </u:BrowseResponse>
  </s:Body>
</s:Envelope>`, escaped, returned, total)
}

func valueBetween(body, open, close string) int {
	start := strings.Index(body, open)
	if start < 0 {
		return 0
	}
	rest := body[start+len(open):]
	end := strings.Index(rest, close)
	if end < 0 {
		return 0
	}
	var n int
	_, _ = fmt.Sscanf(rest[:end], "%d", &n)
	return n
}

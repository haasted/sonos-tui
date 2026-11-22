// How the application drives the search modal: opening it, finding the
// household in the Sonos cloud, and running a search. search.go holds the
// modal itself.
package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/haasted/sonos-tui/internal/sonos"
	"github.com/haasted/sonos-tui/internal/sonoscloud"
	"github.com/haasted/sonos-tui/internal/store"
	"github.com/haasted/sonos-tui/internal/ui/panes"
)

// What the search modal says when a search cannot run or fails.
const (
	searchNoCloud     = "The Sonos cloud is not available."
	searchSignedOut   = "Not signed in to Sonos cloud services."
	searchNoHousehold = "The Sonos cloud does not know this household."
	searchNoSpeaker   = "No speaker has loaded yet."
	searchFailed      = "The Sonos cloud did not answer."
)

// What a key on a search result answers when nothing is sent. A station that
// is queued answers as it does in the favourites.
const (
	searchStationNotPlayed = "a station cannot be played yet"
	searchArtistNotPlayed  = "an artist cannot be played"
	searchArtistNotQueued  = "an artist cannot be queued"
	searchStationNotMarked = "a station cannot be marked yet"
	searchArtistNotMarked  = "an artist cannot be marked yet"
	searchNotAvailable     = "not available to play"
	searchNotStarted       = "could not start it"
	searchMoreFailed       = "could not load the rest of the album"
)

// errNoSpeaker says there is no speaker to ask for its household.
var errNoSpeaker = errors.New("no speaker loaded")

// musicCloud is what the app needs of the Sonos cloud, so a test can stand in
// for it.
type musicCloud interface {
	SignedIn() bool
	SessionEnds() time.Time
	Household(ctx context.Context, localID string) (sonoscloud.Household, error)
	SignIn(ctx context.Context, username, password string) error
	SignOut(ctx context.Context) error
	Search(ctx context.Context, household string, service int, query string) (sonoscloud.SearchResults, error)
	OpenArtist(ctx context.Context, household string, service int, objectID string) (sonoscloud.Artist, error)
	OpenAlbum(ctx context.Context, household string, service int, objectID string) (sonoscloud.Album, error)
	RestOfAlbum(ctx context.Context, household string, service int, album sonoscloud.Album) (sonoscloud.Album, error)
}

var _ musicCloud = &sonoscloud.Client{}

type (
	// cloudHouseholdMsg is the household lookup made at startup: the
	// speakers' own household id, and whether the cloud knew it.
	cloudHouseholdMsg struct {
		localID string
		err     error
	}

	// searchDoneMsg is a search's answer. gen tells it from the answer to an
	// older query.
	searchDoneMsg struct {
		gen     int
		query   string
		localID string
		results sonoscloud.SearchResults
		err     error
	}
)

// openSearch opens the search modal. It shows the last search again, or
// opens with the box ready when there has been none.
func (m appModel) openSearch() (tea.Model, tea.Cmd) {
	// With no valid session, the sign-in comes first; the search opens after.
	if m.cloud != nil && !m.cloud.SignedIn() {
		return m.openSignIn(followSearch)
	}
	m.search = m.search.Open()
	content, _ := m.search.Content.(searchContent)
	content.listView.openedAt = time.Now()
	content = content.resized(m.search.ContentRows(), m.search.ContentColumns()).(searchContent)
	switch {
	case m.cloud == nil:
		content = content.failed(searchNoCloud)
	case content.asked == "" || content.problem != "":
		content.filtering, content.problem = true, ""
	}
	m.search.Content = content

	m, sweeping := m.restartSweepChain()
	hint := restartListHint(m.searchSlot())
	return m, tea.Batch(sweeping, hint)
}

// applySearchAsk starts a search, and stops the one still running: only the
// last query's answer is shown.
func (m appModel) applySearchAsk(msg searchAskMsg) (tea.Model, tea.Cmd) {
	content, ok := m.search.Content.(searchContent)
	if !ok {
		return m, nil
	}
	if m.searchCancel != nil {
		m.searchCancel()
	}
	switch {
	case m.cloud == nil:
		m.search.Content = content.failed(searchNoCloud)
		return m, nil
	case !m.cloud.SignedIn():
		m.search.Content = content.failed(searchSignedOut)
		return m.openSignIn(followSearch)
	}

	var device *sonos.Device
	if speaker, ok := m.activeSpeaker(); ok {
		device = speaker.device
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.searchCancel, m.searchRun = cancel, nextGen()
	m.search.Content = content.searching(msg.query)
	return m, runSearch(ctx, m.cloud, device, m.localHousehold, msg.query, m.searchRun)
}

// applySearchDone shows a search's answer, unless a later query has been
// asked since.
func (m appModel) applySearchDone(msg searchDoneMsg) (tea.Model, tea.Cmd) {
	if msg.localID != "" {
		m.localHousehold = msg.localID
	}
	content, ok := m.search.Content.(searchContent)
	if !ok || msg.gen != m.searchRun {
		return m, nil
	}
	m.searchCancel = nil
	if msg.err != nil {
		m.search.Content = content.failed(searchProblem(msg.err))
		return m.signInWhenSignedOut(msg.err)
	}
	content = content.found(msg.query, msg.results)
	m.search.Content = content
	if m.search.IsOpen() && content.overflows() {
		return m, restartListChain(m.searchSlot())
	}
	return m, nil
}

// applySearchAction starts a result on the speaker in view, as the favourites
// start theirs, and closes the search after a play. What cannot be started is
// said, and nothing is sent.
func (m appModel) applySearchAction(msg searchActionMsg) (tea.Model, tea.Cmd) {
	speaker, ok := m.activeSpeaker()
	if !ok {
		return m, nil
	}
	r, action := msg.result, msg.action
	plays := action == startPlay || action == startPlayShuffled
	switch {
	case r.Type == sonoscloud.TypeProgram && plays:
		return m, notify(panes.NoticeInfo, searchStationNotPlayed)
	case r.Type == sonoscloud.TypeProgram:
		return m, notify(panes.NoticeInfo, favouriteStationNotQueued)
	case r.Type == sonoscloud.TypeArtist && plays:
		return m, notify(panes.NoticeInfo, searchArtistNotPlayed)
	case r.Type == sonoscloud.TypeArtist:
		return m, notify(panes.NoticeInfo, searchArtistNotQueued)
	case unavailable(r):
		return m, notify(panes.NoticeInfo, searchNotAvailable)
	}
	// enter on an album's track plays the album, from that track.
	if msg.from != nil && action == startPlay {
		album, err := resultPlayable(msg.from.album)
		if err != nil {
			slog.Warn("starting an album from a track", "err", err)
			return m, notify(panes.NoticeWarn, searchNotStarted)
		}
		m = m.closeSearch()
		return m, startAlbumFrom(speaker, album, msg.from.position, r.Name)
	}
	playable, err := resultPlayable(r)
	if err != nil {
		slog.Warn("starting a search result", "err", err)
		return m, notify(panes.NoticeWarn, searchNotStarted)
	}

	// One track has nothing to shuffle, so shift+enter plays it as enter does.
	if action == startPlayShuffled && r.Type == sonoscloud.TypeTrack {
		action = startPlay
	}
	if plays {
		m = m.closeSearch()
	}
	return m, startPlayable(speaker, playable, action, false)
}

// closeSearch closes the search after a play, as the favourites close, and
// ends the chains that ran while it was open.
func (m appModel) closeSearch() appModel {
	m.search = m.search.Close()
	slot := m.searchSlot()
	*slot.pulse, *slot.hint = nextGen(), nextGen()
	return m
}

// applySearchOpen reads the artist or album the search stepped into, in the
// background. The answer goes to the level it was asked for, if that is still
// open; going back leaves it with no level to land in.
func (m appModel) applySearchOpen(msg searchOpenMsg) (tea.Model, tea.Cmd) {
	content, ok := m.search.Content.(searchContent)
	if !ok {
		return m, nil
	}
	switch {
	case m.cloud == nil:
		m.search.Content = content.openFailed(msg.gen, searchNoCloud)
		return m, nil
	case !m.cloud.SignedIn():
		m.search.Content = content.openFailed(msg.gen, searchSignedOut)
		return m.openSignIn(followSearch)
	}
	var device *sonos.Device
	if speaker, ok := m.activeSpeaker(); ok {
		device = speaker.device
	}
	if msg.result.Type == sonoscloud.TypeAlbum {
		return m, openAlbum(m.ctx, m.cloud, device, m.localHousehold, msg.result.ID.ObjectID, msg.gen)
	}
	return m, openArtist(m.ctx, m.cloud, device, m.localHousehold, msg.result.ID.ObjectID, msg.gen)
}

// applyAlbumOpened shows an album's tracks, or why there are none.
func (m appModel) applyAlbumOpened(msg albumOpenedMsg) (tea.Model, tea.Cmd) {
	if msg.localID != "" {
		m.localHousehold = msg.localID
	}
	content, ok := m.search.Content.(searchContent)
	if !ok {
		return m, nil
	}
	if msg.err != nil {
		m.search.Content = content.openFailed(msg.gen, searchProblem(msg.err))
		return m.signInWhenSignedOut(msg.err)
	}
	content = content.openedAlbum(msg.gen, msg.album)
	m.search.Content = content
	if m.search.IsOpen() && content.overflows() {
		return m, restartListChain(m.searchSlot())
	}
	return m, nil
}

// applySearchMore reads the rest of a cut-off album, in the background.
func (m appModel) applySearchMore(msg searchMoreMsg) (tea.Model, tea.Cmd) {
	content, ok := m.search.Content.(searchContent)
	if !ok {
		return m, nil
	}
	switch {
	case m.cloud == nil:
		m.search.Content = content.moreFailed(msg.gen)
		return m, notify(panes.NoticeWarn, searchMoreFailed)
	case !m.cloud.SignedIn():
		m.search.Content = content.moreFailed(msg.gen)
		return m.openSignIn(followSearch)
	}
	var device *sonos.Device
	if speaker, ok := m.activeSpeaker(); ok {
		device = speaker.device
	}
	return m, readRestOfAlbum(m.ctx, m.cloud, device, m.localHousehold, msg.album, msg.gen)
}

// applyAlbumMore shows the whole album, or says the rest could not be read
// and leaves the row to try again.
func (m appModel) applyAlbumMore(msg albumMoreMsg) (tea.Model, tea.Cmd) {
	if msg.localID != "" {
		m.localHousehold = msg.localID
	}
	content, ok := m.search.Content.(searchContent)
	if !ok {
		return m, nil
	}
	if msg.err != nil {
		m.search.Content = content.moreFailed(msg.gen)
		if errors.Is(msg.err, sonoscloud.ErrSignedOut) {
			return m.openSignIn(followSearch)
		}
		slog.Warn("reading the rest of an album", "err", msg.err)
		return m, notify(panes.NoticeWarn, searchMoreFailed)
	}
	content = content.moreLoaded(msg.gen, msg.album)
	m.search.Content = content
	if m.search.IsOpen() && content.overflows() {
		return m, restartListChain(m.searchSlot())
	}
	return m, nil
}

// readRestOfAlbum reads a cut-off album to its end, finding the household
// first if the startup lookup has not.
func readRestOfAlbum(ctx context.Context, cloud musicCloud, device *sonos.Device, localID string, album sonoscloud.Album, gen int) tea.Cmd {
	return func() tea.Msg {
		localID, household, err := findHousehold(ctx, cloud, device, localID)
		if err != nil {
			return albumMoreMsg{gen: gen, localID: localID, err: err}
		}
		whole, err := cloud.RestOfAlbum(ctx, household.ID, sonoscloud.AppleMusic, album)
		return albumMoreMsg{gen: gen, localID: localID, album: whole, err: err}
	}
}

// openAlbum reads an album of Apple Music and its tracks, finding the
// household first if the startup lookup has not.
func openAlbum(ctx context.Context, cloud musicCloud, device *sonos.Device, localID, objectID string, gen int) tea.Cmd {
	return func() tea.Msg {
		localID, household, err := findHousehold(ctx, cloud, device, localID)
		if err != nil {
			return albumOpenedMsg{gen: gen, localID: localID, err: err}
		}
		album, err := cloud.OpenAlbum(ctx, household.ID, sonoscloud.AppleMusic, objectID)
		return albumOpenedMsg{gen: gen, localID: localID, album: album, err: err}
	}
}

// applyArtistOpened shows an artist's albums, or why there are none.
func (m appModel) applyArtistOpened(msg artistOpenedMsg) (tea.Model, tea.Cmd) {
	if msg.localID != "" {
		m.localHousehold = msg.localID
	}
	content, ok := m.search.Content.(searchContent)
	if !ok {
		return m, nil
	}
	if msg.err != nil {
		m.search.Content = content.openFailed(msg.gen, searchProblem(msg.err))
		return m.signInWhenSignedOut(msg.err)
	}
	content = content.opened(msg.gen, msg.artist)
	m.search.Content = content
	if m.search.IsOpen() && content.overflows() {
		return m, restartListChain(m.searchSlot())
	}
	return m, nil
}

// openArtist reads an artist of Apple Music and its albums, finding the
// household first if the startup lookup has not.
func openArtist(ctx context.Context, cloud musicCloud, device *sonos.Device, localID, objectID string, gen int) tea.Cmd {
	return func() tea.Msg {
		localID, household, err := findHousehold(ctx, cloud, device, localID)
		if err != nil {
			return artistOpenedMsg{gen: gen, localID: localID, err: err}
		}
		artist, err := cloud.OpenArtist(ctx, household.ID, sonoscloud.AppleMusic, objectID)
		return artistOpenedMsg{gen: gen, localID: localID, artist: artist, err: err}
	}
}

// applySearchMark stores a result as a favourite, as a does in the queue, and
// says whether it was one already. What cannot be marked is said instead.
func (m appModel) applySearchMark(msg searchMarkMsg) (tea.Model, tea.Cmd) {
	r := msg.result
	switch {
	case r.Type == sonoscloud.TypeProgram:
		return m, notify(panes.NoticeInfo, searchStationNotMarked)
	case r.Type == sonoscloud.TypeArtist:
		return m, notify(panes.NoticeInfo, searchArtistNotMarked)
	case unavailable(r):
		return m, notify(panes.NoticeInfo, searchNotAvailable)
	}
	fav, err := resultFavourite(r, time.Now())
	if err != nil {
		slog.Warn("marking a search result", "err", err)
		return m, notify(panes.NoticeWarn, favouriteMarkFailed)
	}
	return m, storeFavourites(m.ctx, m.db, []store.Favourite{fav}, 1, 0)
}

// signInWhenSignedOut opens the sign-in when err says the server no longer
// takes the session, as one that has run out or been signed out elsewhere.
func (m appModel) signInWhenSignedOut(err error) (tea.Model, tea.Cmd) {
	if errors.Is(err, sonoscloud.ErrSignedOut) {
		return m.openSignIn(followSearch)
	}
	return m, nil
}

// lookUpHousehold finds the household in the Sonos cloud once, in the
// background, from the first speaker that loads, so a search need not wait
// for it. With no valid session the cloud is not asked at all.
func (m appModel) lookUpHousehold(device *sonos.Device) (appModel, tea.Cmd) {
	if m.cloudAsked || m.cloud == nil || device == nil || !m.cloud.SignedIn() {
		return m, nil
	}
	m.cloudAsked = true
	ctx, cloud := m.ctx, m.cloud
	return m, func() tea.Msg {
		localID, _, err := findHousehold(ctx, cloud, device, "")
		return cloudHouseholdMsg{localID: localID, err: err}
	}
}

// applyCloudHousehold keeps the speakers' household id the lookup read. The
// cloud keeps the household itself.
func (m appModel) applyCloudHousehold(msg cloudHouseholdMsg) (tea.Model, tea.Cmd) {
	if msg.localID != "" {
		m.localHousehold = msg.localID
	}
	if msg.err != nil {
		slog.Warn("finding the household in the Sonos cloud", "err", msg.err)
	}
	return m, nil
}

// runSearch searches Apple Music for query, finding the household first if
// the startup lookup has not.
func runSearch(ctx context.Context, cloud musicCloud, device *sonos.Device, localID, query string, gen int) tea.Cmd {
	return func() tea.Msg {
		localID, household, err := findHousehold(ctx, cloud, device, localID)
		if err != nil {
			return searchDoneMsg{gen: gen, query: query, localID: localID, err: err}
		}
		results, err := cloud.Search(ctx, household.ID, sonoscloud.AppleMusic, query)
		return searchDoneMsg{gen: gen, query: query, localID: localID, results: results, err: err}
	}
}

// findHousehold finds the speakers' household in the cloud. localID is the
// speakers' own household id; when it is empty, device is asked for it.
func findHousehold(ctx context.Context, cloud musicCloud, device *sonos.Device, localID string) (string, sonoscloud.Household, error) {
	if localID == "" {
		if device == nil {
			return "", sonoscloud.Household{}, errNoSpeaker
		}
		id, err := device.HouseholdID(ctx)
		if err != nil {
			return "", sonoscloud.Household{}, err
		}
		localID = id
	}
	household, err := cloud.Household(ctx, localID)
	return localID, household, err
}

// searchProblem is what the modal says about a failed search.
func searchProblem(err error) string {
	switch {
	case errors.Is(err, sonoscloud.ErrSignedOut):
		return searchSignedOut
	case errors.Is(err, sonoscloud.ErrHouseholdUnknown), errors.Is(err, sonoscloud.ErrWrongHousehold):
		return searchNoHousehold
	case errors.Is(err, errNoSpeaker):
		return searchNoSpeaker
	default:
		slog.Warn("searching the Sonos cloud", "err", err)
		return searchFailed
	}
}

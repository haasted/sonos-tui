package sonos

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

const eventChanBuffer = 8

// Device wraps a Client with the identity and last-known state of one
// physical Sonos speaker. Client stays a stateless SOAP transport; Device
// adds the "what do we currently believe about this speaker" layer on top.
type Device struct {
	Speaker

	client *SonosClient

	mu    sync.RWMutex
	state DeviceState

	queueGen atomic.Int64 // guards queue reads, so a slow one cannot overwrite a newer

	events chan Event
}

// DeviceState is a point-in-time snapshot of what Device knows about the
// speaker, without going to the network.
type DeviceState struct {
	Transport TransportInfo
	Current   TrackMetadata
	RelTime   string

	Volume int
	Muted  bool

	// Source is what is loaded on the speaker — a queue, a station, a line-in.
	// Distinct from Current, which is whatever is coming out of it right now.
	Source SourceInfo

	Track  int // 1-based position in the queue, as the speaker counts it
	Tracks int // how many the speaker says the queue holds

	// Queue is the speaker's queue as it was last read. A slice, so copying
	// DeviceState copies a header rather than the items -- which means every
	// copy shares one backing array, and the queue must always be replaced
	// wholesale rather than edited in place.
	Queue []QueueItem

	// QueueKnown says the queue above was read, as against not read yet. An
	// empty queue is a real answer and cannot be told from a missing one by
	// length, nor by QueueUpdateID, whose zero is also a real value.
	QueueKnown bool

	// QueueSource names the playlist or album last loaded into the queue, kept
	// while a station plays so the queue keeps its name. Approximate: an edit
	// to the queue since may have left it describing what the queue was.
	QueueSource string

	// QueueUpdateID is the speaker's own version number for its queue. It
	// rises whenever the queue changes, so a copy read at a lower id is
	// stale. Zero until an event or a Browse has reported one.
	QueueUpdateID int
}

// Live reports playback with no meaningful duration: a broadcast, or a station
// between tracks. It asks about the DURATION rather than the source kind,
// because an algorithmic station can play discrete tracks whose clock works
// perfectly well — and it does not ask whether playback is running, since a
// paused stream's clock is no more meaningful than a playing one's.
//
// Requiring a known source is what keeps a speaker whose state has not loaded
// yet from being reported as a stream: everything is zero at that point,
// including the duration.
func (s DeviceState) Live() bool {
	return s.Source.URI != "" &&
		ParseDuration(s.Current.Duration) == 0 &&
		s.Source.Kind() != SourceTrack
}

// Playing reports whether the speaker is actively playing. Every other
// transport state — paused, stopped, transitioning — counts as idle.
func (s DeviceState) Playing() bool {
	return s.Transport.State == transportPlaying
}

// EventKind says which service produced an event. Volume events carry no
// playback position, so consumers must not treat them as a position update.
type EventKind int

const (
	EventAVTransport EventKind = iota
	EventRenderingControl
	EventContentDirectory
	EventZoneGroupTopology
)

type Event struct {
	Kind      EventKind
	Speaker   Speaker
	Transport TransportInfo
	Current   TrackMetadata
	Raw       AVTransportEvent

	// Directory is set on a ContentDirectory event only. It says which
	// containers changed, and carries no playback state at all.
	Directory ContentDirectoryEvent

	// Topology is set on a ZoneGroupTopology event only, and holds the whole
	// grouping of the household that sent it: the event carries the answer
	// rather than word that the answer changed.
	Topology Topology
}

// NewDevice creates a Device for the given discovered speaker.
func NewDevice(speaker Speaker) *Device {
	return NewDeviceWithClient(speaker, NewClient(speaker.IP))
}

// NewDeviceWithClient creates a Device using a pre-built Client, e.g. one
// pointed at a test server instead of the speaker's real IP.
func NewDeviceWithClient(speaker Speaker, client *SonosClient) *Device {
	return &Device{
		Speaker: speaker,
		client:  client,
		events:  make(chan Event, eventChanBuffer),
	}
}

func (d *Device) Events() <-chan Event {
	return d.events
}

func (d *Device) HandleAVTransportEvent(ctx context.Context, body []byte) error {
	parsed, err := ParseAVTransportEvent(body)
	if err != nil {
		return err
	}

	relTime := parsed.RelTime
	if relTime == "" {
		info, err := d.client.GetPositionInfo(ctx)
		if err != nil {
			slog.Warn("fetching position after event", "speaker", d.Speaker, "err", err)
		} else {
			relTime = info.RelTime
		}
	}

	d.mu.Lock()
	// Every field is guarded on having been reported rather than on being
	// non-empty. LastChange omits what did not change, and emptiness is a
	// reading in its own right -- these two are not the same question.
	if parsed.HasTransportState {
		d.state.Transport.State = parsed.TransportState
	}
	if parsed.HasSource {
		d.state.Source = parsed.Source
	}
	if parsed.HasQueueSource {
		d.state.QueueSource = parsed.QueueSource
	}
	if parsed.HasCurrent {
		d.state.Current = parsed.Current
	}
	if parsed.HasTrack {
		d.state.Track = parsed.Track
	}
	if parsed.HasTracks {
		d.state.Tracks = parsed.NumberOfTracks
	}
	d.state.RelTime = relTime
	snapshot := d.state
	d.mu.Unlock()

	event := Event{
		Kind:      EventAVTransport,
		Speaker:   d.Speaker,
		Transport: snapshot.Transport,
		Current:   snapshot.Current,
		Raw:       parsed,
	}

	select {
	case d.events <- event:
	default:
		slog.Warn("event channel full — dropping event", "speaker", d.Speaker)
	}

	return nil
}

// HandleRenderingControlEvent applies a volume/mute event and publishes it.
// Unlike AVTransport it never refetches position: volume changes say nothing
// about playback progress.
func (d *Device) HandleRenderingControlEvent(body []byte) error {
	parsed, err := ParseRenderingControlEvent(body)
	if err != nil {
		return err
	}

	d.mu.Lock()
	if parsed.HasVolume {
		d.state.Volume = parsed.Volume
	}
	if parsed.HasMute {
		d.state.Muted = parsed.Muted
	}
	snapshot := d.state
	d.mu.Unlock()

	event := Event{
		Kind:      EventRenderingControl,
		Speaker:   d.Speaker,
		Transport: snapshot.Transport,
		Current:   snapshot.Current,
	}

	select {
	case d.events <- event:
	default:
		slog.Warn("event channel full — dropping event", "speaker", d.Speaker)
	}

	return nil
}

// HandleContentDirectoryEvent records which containers changed and publishes
// the fact. It fetches nothing: the event says the queue moved, not what it
// now holds, and a Browse for every speaker on every change is a cost the
// caller should choose rather than inherit.
func (d *Device) HandleContentDirectoryEvent(body []byte) error {
	parsed, err := ParseContentDirectoryEvent(body)
	if err != nil {
		return err
	}

	d.mu.Lock()
	if updateID, named := parsed.QueueUpdate(); named {
		d.state.QueueUpdateID = updateID
	}
	snapshot := d.state
	d.mu.Unlock()

	event := Event{
		Kind:      EventContentDirectory,
		Speaker:   d.Speaker,
		Transport: snapshot.Transport,
		Current:   snapshot.Current,
		Directory: parsed,
	}

	select {
	case d.events <- event:
	default:
		slog.Warn("event channel full — dropping event", "speaker", d.Speaker)
	}

	return nil
}

// State returns a copy of the last-known state. It never touches the
// network — call Refresh first to populate it.
func (d *Device) State() DeviceState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

// Refresh fetches the current transport state from the speaker.
func (d *Device) Refresh(ctx context.Context) error {
	info, err := d.client.GetTransportInfo(ctx)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.state.Transport = *info
	d.mu.Unlock()
	return nil
}

func (d *Device) RefreshTrack(ctx context.Context) error {
	info, err := d.client.GetPositionInfo(ctx)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.state.Current = TrackMetadata{
		URI:         info.TrackURI,
		Duration:    info.TrackDuration,
		Title:       info.Title,
		Artist:      info.Artist,
		Album:       info.Album,
		AlbumArtURI: info.AlbumArtURI,
	}
	d.state.RelTime = info.RelTime
	d.mu.Unlock()
	return nil
}

func (d *Device) RefreshVolume(ctx context.Context) error {
	var (
		wg      sync.WaitGroup
		volume  int
		muted   bool
		volErr  error
		muteErr error
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		volume, volErr = d.client.GetVolume(ctx)
	}()
	go func() {
		defer wg.Done()
		muted, muteErr = d.client.GetMute(ctx)
	}()
	wg.Wait()

	if err := errors.Join(volErr, muteErr); err != nil {
		return err
	}

	d.mu.Lock()
	d.state.Volume = volume
	d.state.Muted = muted
	d.mu.Unlock()
	return nil
}

// Queue reads the speaker's playback queue. Deliberately not stored on
// DeviceState: nothing renders it yet, and a queue that went stale in state
// would be worse than one fetched when asked for.
func (d *Device) Queue(ctx context.Context) (Queue, error) {
	return d.client.GetQueue(ctx)
}

// RemoveQueueTrack takes one track out of this speaker's queue.
func (d *Device) RemoveQueueTrack(ctx context.Context, position int) error {
	return d.writeQueue(ctx, func(updateID int) (int, error) {
		return d.client.RemoveTrackRange(ctx, position, 1, updateID)
	})
}

// Topology reads how the whole household is grouped. Any speaker answers for
// all of them, so a caller needs only one of these however many it holds.
func (d *Device) Topology(ctx context.Context) (Topology, error) {
	return d.client.GetZoneGroupState(ctx)
}

// HandleZoneGroupTopologyEvent reads how the household is now grouped and
// publishes it. Nothing is stored on the device: the grouping is the
// household's and belongs to whoever holds one, where a copy per speaker
// would be the same answer written down five times.
//
// A body carrying no grouping is not an error. The first event after
// subscribing replays every variable the service has, and a later one
// carries only what changed, which may be none of our business.
func (d *Device) HandleZoneGroupTopologyEvent(body []byte) error {
	state, err := zoneGroupStateFrom(body)
	if err != nil {
		return err
	}
	if state == "" {
		return nil
	}

	topology, err := parseZoneGroupState(state)
	if err != nil {
		return err
	}

	select {
	case d.events <- Event{Kind: EventZoneGroupTopology, Speaker: d.Speaker, Topology: topology}:
	default:
		slog.Warn("event channel full — dropping event", "speaker", d.Speaker)
	}
	return nil
}

// JoinGroup makes this speaker follow another, and LeaveGroup takes it out
// of whatever group it is in. Both throw away what this speaker was doing --
// see the client for what that costs.
func (d *Device) JoinGroup(ctx context.Context, coordinator string) error {
	return d.client.JoinGroup(ctx, coordinator)
}

func (d *Device) LeaveGroup(ctx context.Context) error {
	return d.client.LeaveGroup(ctx)
}

// RemoveQueueTracks takes several tracks out of this speaker's queue, named
// by their 1-based positions. It answers with how many left, which is all of
// them unless the speaker refused part way through.
//
// The tracks go from the bottom of the queue upwards, in runs of adjacent
// ones. Upwards because removing a track shifts every position below it, and
// going up means no position still to be used has moved. In runs because the
// speaker takes a whole run in one call.
func (d *Device) RemoveQueueTracks(ctx context.Context, positions []int) (int, error) {
	var removed int
	for _, run := range queueRuns(positions) {
		err := d.writeQueue(ctx, func(updateID int) (int, error) {
			return d.client.RemoveTrackRange(ctx, run.start, run.count, updateID)
		})
		if err != nil {
			return removed, err
		}
		removed += run.count
	}
	return removed, nil
}

// QueueMove is one track going from one place in the queue to another, both
// 1-based positions.
type QueueMove struct{ From, To int }

// MoveQueueTracks makes a run of moves in order, and answers with how many
// were made. The caller works out the positions: each move must be read
// against the queue the one before it left, which needs the queue itself.
func (d *Device) MoveQueueTracks(ctx context.Context, moves []QueueMove) (int, error) {
	for i, move := range moves {
		if err := d.MoveQueueTrack(ctx, move.From, move.To); err != nil {
			return i, err
		}
	}
	return len(moves), nil
}

// MoveQueueTrack moves one track within this speaker's queue.
func (d *Device) MoveQueueTrack(ctx context.Context, from, to int) error {
	return d.writeQueue(ctx, func(updateID int) (int, error) {
		return d.client.MoveQueueTrack(ctx, from, to, updateID)
	})
}

// queueRun is a set of adjacent positions, which the speaker takes out in one
// call.
type queueRun struct {
	start int // 1-based, the lowest position in the run
	count int
}

// queueRuns groups positions into runs of adjacent tracks, the highest run
// first. A position named twice is dropped: acting on it again would reach
// whatever moved into it.
func queueRuns(positions []int) []queueRun {
	sorted := slices.Clone(positions)
	slices.Sort(sorted)
	sorted = slices.Compact(sorted)

	var runs []queueRun
	for i := 0; i < len(sorted); {
		last := i
		for last+1 < len(sorted) && sorted[last+1] == sorted[last]+1 {
			last++
		}
		runs = append(runs, queueRun{start: sorted[i], count: last - i + 1})
		i = last + 1
	}

	slices.Reverse(runs)
	return runs
}

// writeQueue runs one queue write at the version the device holds, and stores
// the version the speaker answers with.
//
// Storing it is what lets a run of writes go out one after another. The
// speaker refuses a write carrying a superseded version, and the
// ContentDirectory event that would otherwise teach the new one arrives long
// after the next keypress -- which is why a quick second edit used to fail
// rather than merely lag.
//
// A refresh landing between the read and the store leaves the older version
// here. That costs a refusal on the next write and nothing worse: the check
// exists so a wrong version is rejected rather than applied to the wrong
// track, and the event that follows puts the right one back.
func (d *Device) writeQueue(ctx context.Context, write func(updateID int) (int, error)) error {
	d.mu.RLock()
	updateID := d.state.QueueUpdateID
	d.mu.RUnlock()

	next, err := write(updateID)
	if err != nil {
		return err
	}

	d.mu.Lock()
	d.state.QueueUpdateID = next
	d.mu.Unlock()
	return nil
}

// RefreshQueue reads the queue and stores it. Guarded by a generation, because
// a queue can be announced twice within a second while a read takes about a
// tenth of one: two reads are then in flight, and the slower one must not
// overwrite the newer answer it raced.
func (d *Device) RefreshQueue(ctx context.Context) error {
	generation := d.queueGen.Add(1)

	queue, err := d.client.GetQueue(ctx)
	if err != nil {
		return err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if generation != d.queueGen.Load() {
		slog.Debug("dropping a queue read a newer one overtook", "speaker", d.Speaker)
		return nil
	}
	d.state.Queue = queue.Items
	d.state.QueueUpdateID = queue.UpdateID
	d.state.QueueKnown = true
	return nil
}

func (d *Device) RefreshAll(ctx context.Context) error {
	refreshers := []func(context.Context) error{d.Refresh, d.RefreshTrack, d.RefreshVolume}
	errs := make([]error, len(refreshers))

	var wg sync.WaitGroup
	wg.Add(len(refreshers))
	for i, refresh := range refreshers {
		go func() {
			defer wg.Done()
			errs[i] = refresh(ctx)
		}()
	}
	wg.Wait()

	return errors.Join(errs...)
}

func (d *Device) AdjustVolume(ctx context.Context, adjustment int) (int, error) {
	d.mu.RLock()
	muted := d.state.Muted
	d.mu.RUnlock()

	if muted {
		if err := d.SetMuted(ctx, false); err != nil {
			return 0, fmt.Errorf("adjust volume: %w", err)
		}
	}

	volume, err := d.client.SetRelativeVolume(ctx, adjustment)
	if err != nil {
		return 0, fmt.Errorf("adjust volume: %w", err)
	}

	d.mu.Lock()
	d.state.Volume = volume
	d.mu.Unlock()
	return volume, nil
}

func (d *Device) SetMuted(ctx context.Context, muted bool) error {
	if err := d.client.SetMute(ctx, muted); err != nil {
		return fmt.Errorf("set mute: %w", err)
	}
	d.mu.Lock()
	d.state.Muted = muted
	d.mu.Unlock()
	return nil
}

func (d *Device) ToggleMute(ctx context.Context) error {
	d.mu.RLock()
	muted := d.state.Muted
	d.mu.RUnlock()
	return d.SetMuted(ctx, !muted)
}

// SetGroupMuted mutes or unmutes every speaker in this one's group. Sonos
// sets each member's own mute as it goes, so the members' events correct
// their states without this having to reach them.
func (d *Device) SetGroupMuted(ctx context.Context, muted bool) error {
	if err := d.client.SetGroupMute(ctx, muted); err != nil {
		return fmt.Errorf("set group mute: %w", err)
	}
	d.mu.Lock()
	d.state.Muted = muted
	d.mu.Unlock()
	return nil
}

// ToggleGroupMute flips the group's mute and reports where it landed. The
// current value is read from the speaker rather than from this Device: the
// group's mute is not this one's, and a caller acting on a stale copy would
// toggle it the wrong way.
func (d *Device) ToggleGroupMute(ctx context.Context) (bool, error) {
	muted, err := d.client.GetGroupMute(ctx)
	if err != nil {
		return false, fmt.Errorf("toggle group mute: %w", err)
	}
	if err := d.SetGroupMuted(ctx, !muted); err != nil {
		return false, err
	}
	return !muted, nil
}

// Play starts or resumes playback.
func (d *Device) Play(ctx context.Context) error {
	if err := d.client.Play(ctx); err != nil {
		return fmt.Errorf("play: %w", err)
	}
	d.mu.Lock()
	d.state.Transport.State = transportPlaying
	d.mu.Unlock()
	return nil
}

// Pause pauses playback.
func (d *Device) Pause(ctx context.Context) error {
	if err := d.client.Pause(ctx); err != nil {
		return fmt.Errorf("pause: %w", err)
	}
	d.mu.Lock()
	d.state.Transport.State = "PAUSED_PLAYBACK"
	d.mu.Unlock()
	return nil
}

func (d *Device) Next(ctx context.Context) error {
	if err := d.client.Next(ctx); err != nil {
		return fmt.Errorf("next track: %w", err)
	}
	d.mu.Lock()
	d.state.Transport.State = transportPlaying
	d.mu.Unlock()
	return nil
}

func (d *Device) Previous(ctx context.Context) error {
	if err := d.client.Previous(ctx); err != nil {
		return fmt.Errorf("previous track: %w", err)
	}
	d.mu.Lock()
	d.state.Transport.State = transportPlaying
	d.mu.Unlock()
	return nil
}

// ReplaceQueue empties the queue, fills it with p and plays it in mode. The
// mode is set every time: a shuffle left from before would otherwise carry
// over. Only a coordinator has a queue to replace.
//
// In order, it starts from the first track: the speaker keeps its old place
// in the queue across the replacement. Shuffled, the speaker picks the start.
func (d *Device) ReplaceQueue(ctx context.Context, p Playable, mode PlayMode) error {
	steps := []func() error{
		func() error { return d.client.RemoveAllTracksFromQueue(ctx) },
		func() error { return d.client.AddURIToQueue(ctx, p.URI, p.Metadata, queueEnd, false) },
		func() error { return d.client.SelectQueue(ctx, d.ID()) },
		func() error { return d.client.SetPlayMode(ctx, mode) },
	}
	if mode == PlayModeNormal {
		steps = append(steps, func() error { return d.client.SeekTrack(ctx, 1) })
	}
	steps = append(steps, func() error { return d.client.Play(ctx) })
	return d.run(p, transportPlaying, steps...)
}

// PlayNext puts p into the queue after the playing track. When the queue is
// not what plays, there is no playing track to follow, so it goes first.
func (d *Device) PlayNext(ctx context.Context, p Playable) error {
	position := 1
	if state := d.State(); strings.HasPrefix(state.Source.URI, queueURIScheme) && state.Track > 0 {
		position = state.Track + 1
	}
	return d.run(p, "",
		func() error { return d.client.AddURIToQueue(ctx, p.URI, p.Metadata, position, true) },
	)
}

// AddToQueue puts p at the end of the queue, leaving playback alone.
func (d *Device) AddToQueue(ctx context.Context, p Playable) error {
	return d.run(p, "",
		func() error { return d.client.AddURIToQueue(ctx, p.URI, p.Metadata, queueEnd, false) },
	)
}

// PlayStation plays p in place of whatever plays now. A station is not
// queued: the queue stays as it was, ready to return to.
func (d *Device) PlayStation(ctx context.Context, p Playable) error {
	return d.run(p, transportPlaying,
		func() error { return d.client.SetTransportURI(ctx, p.URI, p.Metadata) },
		func() error { return d.client.Play(ctx) },
	)
}

// run sends steps in order and stops at the first refusal, so nothing after
// it reaches the speaker. A non-empty transport is the state it leaves.
func (d *Device) run(p Playable, transport string, steps ...func() error) error {
	for _, step := range steps {
		if err := step(); err != nil {
			return fmt.Errorf("playing %s: %w", p.Title, err)
		}
	}
	if transport != "" {
		d.mu.Lock()
		d.state.Transport.State = transport
		d.mu.Unlock()
	}
	return nil
}

// PlayTrack jumps to the track at position within the queue and starts it,
// whatever the speaker was doing before -- a station, another track, or this
// one paused. Seek alone is not certain to resume a paused transport, so Play
// follows it regardless of state.
func (d *Device) PlayTrack(ctx context.Context, position int) error {
	if err := d.client.SeekTrack(ctx, position); err != nil {
		return fmt.Errorf("play track %d: %w", position, err)
	}
	if err := d.client.Play(ctx); err != nil {
		return fmt.Errorf("play track %d: %w", position, err)
	}
	d.mu.Lock()
	d.state.Transport.State = transportPlaying
	d.mu.Unlock()
	return nil
}

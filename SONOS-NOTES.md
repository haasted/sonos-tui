# Notes

Protocol findings about how Sonos actually behaves, as distinct from how the
docs say UPnP works. Everything here was verified against the real event
captures in `captured-data/` and `internal/sonos/testdata/` rather than taken
from documentation. Those were recorded by a `-capture` flag the program no
longer carries; the captures stand, the flag does not. The earliest
findings here predate that flag and came from the retired `cmd/listening`.

Design *decisions* live in `ADRs/`. This file is for observed facts.

## How tracks are identified

Sonos has two distinct identifier layers, and the track layer is the
surprising one: **there is no separate track ID**. The DIDL-Lite `<item id>`
for the currently playing track is literally `-1` in every capture. The URI
*is* the key.

### The track: `CurrentTrackURI`

```
x-sonos-http:librarytrack%3ai.8Wdz31psr2P52.mp4?sid=204&flags=8232&sn=2
x-sonosprog-http:song%3a1435934506.mp4?sid=204&flags=8232&sn=2
```

Decoded (`%3a` is `:`):

| Part | Meaning |
|---|---|
| `x-sonos-http:` | Scheme — how to fetch it. Others seen in the wild: `x-file-cifs:` (local NAS), `x-rincon-stream:` (line-in), `x-sonosprog-http:` |
| `librarytrack:i.8Wdz31psr2P52` | The opaque object ID, service-specific |
| `sid=204` | Which music service |
| `sn=2` | Which linked account on that service |
| `flags=8232` | Playback capability bits |

**There is no global track identifier anywhere in the payload.** The complete
DIDL metadata Sonos sends per track is:

```xml
<res protocolInfo="sonos.com-http:*:audio/mp4:*" duration="0:03:19">…URI…</res>
<r:streamInfo>bd:16,sr:44100,c:3,l:0,d:0</r:streamInfo>   <!-- bit depth, sample rate, channels -->
<upnp:albumArtURI>…</upnp:albumArtURI>
<dc:title>Lift Me Up</dc:title>
<dc:creator>Moby</dc:creator>
<upnp:album>Hotel</upnp:album>
<upnp:class>object.item.audioItem.musicTrack</upnp:class>
```

No ISRC, no MusicBrainz MBID, nothing else. The industry standards that would
answer "is this the same recording?" across sources are simply not exposed.

Two identifier namespaces appear within the same service (`sid=204`), and they
likely differ in scope — INFERRED from their naming and shape, not verified,
which would need a second account on the same service:

| Form | Guess |
|---|---|
| `song:1435934506` | The service's own catalog ID. Plain numeric, plausibly identical for every user of that service. |
| `librarytrack:i.8Wdzgx2sr2P52` | A reference into a personal library — the name says so — so probably account-scoped. |

Nothing maps across *services* either way.

This matters for anything that remembers tracks across sessions — playlists,
"add this later", play history (see ADR-0003). A durable scheme
needs both: the URI as a fast path for when service and account happen to
match, and `(title, creator, album)` text for everything else. Text matching
is fuzzy and imperfect, but it's the only service-independent handle Sonos
gives us, and it's what every cross-service music tool ends up doing.

### The source: `AVTransportURI`

Separately from the track, this is what's *loaded* on the speaker:

```
x-rincon-queue:RINCON_804AF293EC4C01400#0                    (the speaker's own queue)
x-sonosapi-radio:radio%3ara.u-5604...?sid=204&flags=0&sn=2   (a radio station)
```

`RINCON_...` is the speaker's hardware ID — the same one that appears in its
SSDP `USN`.

Unlike tracks, container objects *do* carry real DIDL `<item id>`s:

```
000c0000radio%3ara.u-56046cfa3b3f8ccb1abb70ff757aa95f
00060000libraryplaylist%3ap.V7VYpk7FYWLeW
```

The `000c0000` / `00060000` prefix is a Sonos-internal object-class namespace.

**Relevant to the UPnP 701 "transition not available" bug on
previous-track): `AVTransportURI` is the diagnostic that issue calls decisive.
`x-rincon-queue:` is a real queue where "previous" works; `x-sonosapi-radio:`
is a forward-only station where it cannot. The field arrives in every event and
now reaches `DeviceState.Source`, where `Kind()` classifies it.

Confirmed on hardware 2026-09-11. A personalised station on a SYMFONISK Picture
frame answered `Previous` with UPnP 701, "transition not available", while
`Next` worked. The official Sonos app greys its back button out for the same
station, so the speaker is refusing correctly rather than failing.

## SUBSCRIBE returns a full state snapshot

UPnP sends a complete snapshot of all evented state variables as the *first*
NOTIFY after a successful SUBSCRIBE. Confirmed in the captures: every
speaker's first event carries **25 fields**, while subsequent events carry
only the ~10 that changed.

The 25 fields:

```
InstanceID                TransportState            CurrentPlayMode
CurrentCrossfadeMode      NumberOfTracks            CurrentTrack
CurrentSection            CurrentTrackURI           CurrentTrackDuration
CurrentTrackMetaData      PlaybackStorageMedium     AVTransportURI
AVTransportURIMetaData    NextAVTransportURI        NextAVTransportURIMetaData
CurrentTransportActions   TransportStatus           TransportPlaySpeed
CurrentMediaDuration      RecordStorageMedium       PossiblePlaybackStorageMedia
PossibleRecordStorageMedia RecordMediumWriteStatus  CurrentRecordQualityMode
PossibleRecordQualityModes
```

This means `loadRoom`'s `Refresh()`/`RefreshTrack()` fetch data that arrives
again unprompted moments later. Three reasons that redundancy hasn't been
removed:

1. **AVTransport only.** Volume and mute belong to `RenderingControl`, a
   separate service this program doesn't subscribe to yet.
2. **`RelativeTimePosition` is absent** from all 25 — playback position is
   never evented. `Device.HandleEvent` already compensates by calling
   `GetPositionInfo` whenever an event omits it, which is always.
3. **It's asynchronous**, arriving after `Subscribe` returns, so
   `roomReadyMsg` would fire before any state existed.

`NextTrackMetaData` is empty across all captures, consistent with these being
radio sessions rather than queue playback.

## Volume range is 0-100, but that is convention rather than protocol

`SetVolume` range-checks 0-100 and the volume readout divides by 100. That
assumption is correct on real hardware, but it is a de facto standard, not
something UPnP guarantees.

UPnP publishes the authoritative answer per service in an **SCPD document**,
linked from the device description as
`<SCPDURL>/xml/RenderingControl1.xml</SCPDURL>`:

```xml
<name>Volume</name>
<dataType>ui2</dataType>
<allowedValueRange>
  <minimum>0</minimum>
  <maximum>100</maximum>
  <step>1</step>
</allowedValueRange>
```

Queried live against five different hardware classes — an amplifier, a
line-out Port, two Era models and an IKEA SYMFONISK — all agreed exactly:

| Room | Model | Volume |
|---|---|---|
| Køkken | Sonos Era 100 | 0-100 step 1 |
| Spisestue | Sonos Amp | 0-100 step 1 |
| Værelse | Sonos Era 300 | 0-100 step 1 |
| Soveværelse | SYMFONISK Picture frame | 0-100 step 1 |
| Ovenpå | Sonos Port | 0-100 step 1 |

**Decision: hardcode 0-100 and do not read the SCPD.** Reading it would mean
an extra fetch and parse per speaker at load, to discover a number that is the
same on every Sonos device sold. Not worth the complexity.

Two things to know if that ever stops holding. UPnP explicitly permits a
vendor-defined maximum — that is precisely why `allowedValueRange` exists — so
a future product, or any non-Sonos UPnP renderer, need not use 0-100. And
nothing in this codebase reads SCPDs at all today: `internal/sonos/description.go`
parses the device description only for room name and model, ignoring the
service list entirely. The range is asserted, never asked for.

## Discovery timing

Measured startup, five speakers, with `MX: 3` in the SSDP M-SEARCH:

| Speaker | Found at | `loadRoom` duration |
|---|---|---|
| .51 | 78ms | 79ms |
| .55 | 110ms | 43ms |
| .62 | 253ms | 36ms |
| .57 | 335ms | 49ms |
| .59 | 502ms | 45ms |

Speakers replied across ~425ms despite `MX: 3` permitting a 3-second spread,
so Sonos does not draw uniformly from the MX window. `MX` was subsequently
lowered to `1` — worth re-measuring to see whether it actually tightens.

Two things this measurement settled:

**`loadRoom` calls genuinely run concurrently.** Bubbletea spawns a goroutine
per `Cmd` (`handleCommands` in `tea.go`). Proof in the numbers above: `.55`
started 32ms *after* `.51` but finished 4ms *before* it. Out-of-order
completion is impossible under sequential execution. Later speakers only look
sequential because they arrive 82–167ms apart while each `loadRoom` takes
36–79ms — there is simply nothing left to overlap with.

**Discovery's channel outlives the useful work.** The last speaker arrives at
~500ms, but `discoverTimeout` is 5s and `Discover`'s channel doesn't close
until that deadline, so `discoveryDoneMsg` fires ~4.5s after everything is
already on screen. ADR-0001 flags this same point as a "related, smaller
optimization".

## Radio: two schemes, and the station name lives in the URI metadata

Verified 2026-09-09 against a 31-event capture.

`AVTransportURI` identifies what kind of thing is loaded, and there is more
than one radio scheme:

| Scheme | Example | What it is |
|---|---|---|
| `x-sonosapi-hls:` | DR P1, DR P6 | Broadcast radio |
| `x-sonosapi-radio:` | "Akylas" | Algorithmic station |
| `x-rincon-queue:` | `RINCON_...#0` | The speaker's own queue |
| `x-sonos-http:`, `x-sonosprog-http:` | | Individual tracks |

The station name is `dc:title` inside `AVTransportURIMetaData` (and inside
`CurrentTrackMetaData`). No special parsing is needed — it is an ordinary DIDL
`<item>`.

**`r:streamContent` is not the live track.** One event in 31 had a non-empty
value and it was `ZPSTR_CONNECTING`, a status sentinel. Both captured stations
left it empty throughout, along with `dc:creator` and `upnp:album`: broadcast
radio frequently carries **no per-track metadata at all**, just the station
name and `CurrentTrackDuration` of `0:00:00`.

**The queue has no name.** `x-rincon-queue:` carries no `dc:title`. The
playlist a queue was populated *from* lives in `EnqueuedTransportURIMetaData`.
It *is* in the SUBSCRIBE snapshot once something has been loaded: see "What
the queue was started from", below. (An earlier reading here said it was not;
the 2026-11-06 capture showed it was.)

**Captured bodies are double-escaped**: `LastChange` once, the DIDL inside it
again. Two unescape passes to read one by eye; `parseDIDLTrack` already handles
this.

## The queue belongs to the speaker, not to what is playing

Verified 2026-09-09 by browsing `Q:0` on a speaker playing DR P6.

A speaker's queue survives being interrupted. Starting a radio station does not
clear it, empty it, or hide it — `Browse` on `Q:0` returned twenty tracks that
had been queued before the station started, while the station played on.

That makes the queue a property of the *speaker*, not a description of what is
currently coming out of it. `AVTransportURI` says what the source is; `Q:0` says
what is queued, and the two are unrelated while a station is playing.

**Two counts that only sometimes agree.** AVTransport reports `NumberOfTracks`
and `CurrentTrack` for the *current source*. `Browse` reports what the queue
holds. Observed simultaneously on one speaker:

```
source          x-sonosapi-hls:...     (DR P6)
NumberOfTracks  1                      the stream is one "track"
CurrentTrack    1
Q:0 items       20                     a queue from before the station started
```

They agree only when `AVTransportURI` is `x-rincon-queue:` — i.e. when the queue
*is* the source. Treating a mismatch as an error is wrong: for radio it is the
normal state.

**Queue item URIs name the music service, not the queue.** Every item came back
as `x-sonos-http:song%3a<id>.mp4?sid=204&flags=8232&sn=2` — the same scheme an
individually played track uses. `sid` identifies the service (204 here, against
290 for the DR radio stream), so items from different services in one queue can
be told apart. `flags` varies between items in the same queue, and what it means
is not established.


### What the queue was started from: `r:EnqueuedTransportURI`

Verified 2026-11-06 on one speaker (Ovenpå), with `make notify-capture` on
`/MediaRenderer/AVTransport/Event` while the queue was edited step by step.

AVTransport carries a pair, `r:EnqueuedTransportURI` and
`r:EnqueuedTransportURIMetaData`, naming what was last loaded *in place of*
the queue's contents. It is the speaker's only word on where a queue came from.

| Action | The pair afterwards |
|---|---|
| SUBSCRIBE while a playlist plays | in the snapshot (37 fields): that playlist |
| replace the queue with playlist A | briefly empty while the queue clears, then A |
| append an album to the end | still A |
| play next with a whole playlist | still A |
| move tracks, remove a track | still A |
| play a radio station | the **station** (`x-sonosapi-hls:`, class `audioBroadcast`) |
| a playlist started from the phone app | that playlist, in the same form as from this program |
| SUBSCRIBE again, nothing changed | in the snapshot: the last playlist |

**Only a replace changes it.** After an append and a play next, the queue held
171 tracks from three sources and the pair still named the first playlist. It
means "what this queue was started from", not "what it holds". Nothing in the
events says whether the queue was added to since, so a client cannot tell a
pure playlist queue from a mixed one.

**A station takes it over** while the queue, untouched, still holds the old
tracks (see the section above). So the pair names a station even when the
queue modal shows playlist tracks. What it says on returning to the queue
without loading anything was not captured.

**It survives a restart of the program.** The SUBSCRIBE snapshot carries it,
so it is known from the first event, not only after the next change.

**It is ready to store as a favourite.** For a service playlist the URI is the
`x-rincon-cpcontainer:` form and the metadata is DIDL with the container's item
id (`00060000libraryplaylist%3ap.…`) and the service's desc
(`SA_RINCON52231_X_#Svc52231-0-Token`): exactly what playing it again needs. A
station's metadata, by contrast, had item id `-1`.

## ContentDirectory tells you the queue changed, and nothing more

Verified 2026-09-18 against five speakers, with `make queue-events`.

The queue is served by `ContentDirectory:1`, which lives under `/MediaServer`,
not `/MediaRenderer` like AVTransport and RenderingControl. Its event path is
`/MediaServer/ContentDirectory/Event`.

### What the speaker says it events

From the SCPD at `/xml/ContentDirectory1.xml`. Two standard variables, and ten
Sonos adds:

```
SystemUpdateID          ui4       rises on any change anywhere
ContainerUpdateIDs      string    which containers changed
ShareIndexInProgress    boolean   + ShareIndexLastError
UserRadioUpdateID       string    + SavedQueuesUpdateID, ShareListUpdateID
RecentlyPlayedUpdateID  string    + RadioFavoritesUpdateID, RadioLocationUpdateID
Browseable              boolean   + FavoritesUpdateID
```

**There is no `LastChange`.** ContentDirectory:2 and :3 define one that carries
detail; this service does not have it. So the list above is the most a speaker
will ever tell you about its queue.

### The event is a doorbell, not a delivery

`ContainerUpdateIDs` is a flat, comma-separated list of positional pairs:

```
Q:0,42                     the speaker's own queue is now at version 42
SQ:3,7,Q:0,42,R:0/0,9      three containers in one event
```

A container id holds a colon of its own, so the pairs cannot be split on a
delimiter — they are counted, two fields at a time.

The event carries **a version number only**. Nothing says whether a track was
added, removed or moved, and nothing says how many there now are. The only
answer is to Browse.

**`Q:0` is the speaker's own queue.** `SQ:` is saved queues, `R:` radio, `FV:`
and `FV:2` favourites. An event naming one of those says nothing about the
queue, which is not the same as saying the queue is unchanged.

**Update id 0 is a real value**, seen as `FV:2=0`. It is not a marker for
"unknown", so an id that cannot be read must never fall back to 0: a stale
queue would then look current.

### The id from Browse is the id from the event

Browse returns `UpdateID` for the container it browsed. It agreed with the id
the event carried in every case observed (5 of 5), so the two can be compared
to tell a stale copy from a current one.

A full read of 131 to 151 items, which is two Browse calls, took **92 to 120
ms**.

### One id for each write, and the speaker may moderate them

Ids rise by one for each write. A single edit is one write:

```
remove one track     131 items   Q:0  7 -> 8
move a track         131 items   Q:0  8 -> 9
move to position 1   131 items   Q:0  9 -> 10
```

A wholesale replacement is more than one write. The first batch is **exactly
100 tracks**, observed twice, and the rest follows:

```
21:36:02.932  AVTransport       tracks=100          the first batch
21:36:06.113  ContentDirectory  Q:0 10 -> 12        id 11 never announced
21:36:06.233  browsed           151 items  id=12    the finished queue
```

The id jumped from 10 to 12, so the intermediate write did raise it, but the
speaker did not send an event for it. A reader that browsed on the event got
the complete queue and never saw the 100-track state.

**This moderation is real but is not a promise.** An earlier replacement on the
same speaker sent both ids, 740 ms apart. So a read can still land on a queue
that is part-written: complete, well-formed, and about to be replaced. The
defence is that the next event corrects it, which at about 100 ms a read costs
little. A delay before reading is not needed.

**AVTransport knows first.** It reported the replacement 3.2 seconds before
ContentDirectory did. Its number during those seconds is the part-written
queue, so it is the earlier signal and the wrong one.

### The speaker checks the id you send it, and refuses with 1028

Verified 2026-09-24 against a real speaker, four removals out of four.

`RemoveTrackFromQueue` carries the version the caller believes the queue is
at. The speaker **enforces** it. The client no longer sends that action, but
the rule is the service's and not the action's: `Queue:1` enforces it the same
way, measured the same way. A removal aimed one version behind is refused
with **UPnP error 1028**; the identical removal sent again with the current
version succeeds:

```
position 9   sent 247 (one behind)   UPnP error 1028
position 9   sent 248 (current)      ok
position 12  sent 248 (one behind)   UPnP error 1028
position 12  sent 249 (current)      ok
```

Only the id differed between the two attempts, so this also fixes what 1028
means: **the queue has moved on since you read it.** The code is Sonos's own —
it is not in the UPnP AVTransport:1 list, which stops at 718.

**This is a real guard, not decoration**, and it is worth what it costs. A
queue addressed by position is dangerous exactly when it has changed
underneath the reader, and this is the speaker refusing to act on a stale
picture rather than deleting whatever moved into that slot.

**What it implies for a fast run of writes, not yet observed.** A caller's id
only advances when its re-read lands. So a second write sent before then
carries a stale id and must be refused. Anything sending writes faster than it
re-reads should expect 1028 rather than success.

### Removing the playing track advances rather than stops it

Verified 2026-09-23 by removing the track playing on a real speaker.

The speaker advanced to whatever track slid into the vacated playing
position, the same way `Next` would, rather than stopping or refusing the
removal. The action names nothing about playback, only the track to take out
of the queue — the advance is the speaker's own reaction to losing the track
it was on, not something the removal asked for. It was `RemoveTrackFromQueue`
that was measured; the client sends `RemoveTrackRange` now, and the reaction
belongs to the track going away rather than to either action.

### Only ContentDirectory sees a reorder

A move inside the queue keeps the length, and AVTransport carries no field that
changes:

```
21:34:33.970  AVTransport       tracks=131 track=2    every field unchanged
21:34:33.990  ContentDirectory  Q:0=9                 a new version
```

AVTransport does fire, so a reader watching it is woken with nothing to look
at. This is the case that makes the subscription worth its cost.

A move to position 1 also shifts `CurrentTrack` from 2 to 3, because a track is
now ahead of the one playing. That is a side effect of the reorder. Read as a
signal it says the listener skipped forward, which is wrong.

### The first event after SUBSCRIBE is a value, not an inventory

GENA replays the current value of each evented variable. The current value of
`ContainerUpdateIDs` is **whichever containers changed last**, which may be any
container or none of interest:

```
run 1   Ovenpå  R:=1      the queue had not changed recently
run 2   Ovenpå  Q:0=7     the queue was the last thing to change
```

So a program learns the queue's id at subscription only by luck. Browse once at
startup to establish it.

## There is a second queue service, and it answers with the new version

A speaker advertises 16 services. The client speaks to three of them. One of
the others, `Queue:1` at `/MediaRenderer/Queue/Control`, holds a second set of
queue actions, and they are not the same actions.

The difference is what they return. Read the argument directions from the SCPD
of a real speaker:

```
AVTransport:1
  ReorderTracksInQueue       in  StartingIndex NumberOfTracks InsertBefore UpdateID
                             out (nothing)
  RemoveTrackFromQueue       in  ObjectID UpdateID
                             out (nothing)
  RemoveTrackRangeFromQueue  in  UpdateID StartingIndex NumberOfTracks
                             out NewUpdateID

Queue:1
  ReorderTracks              in  QueueID StartingIndex NumberOfTracks InsertBefore UpdateID
                             out NewUpdateID
  RemoveTrackRange           in  QueueID UpdateID StartingIndex NumberOfTracks
                             out NewUpdateID
  RemoveAllTracks            in  QueueID UpdateID
                             out NewUpdateID
  ReplaceAllTracks           in  QueueID UpdateID ContainerURI ContainerMetaData
                                 CurrentTrackIndex NewCurrentTrackIndices
                                 NumberOfURIs EnqueuedURIsAndMetaData
                             out NewQueueLength NewUpdateID
```

This explains a problem that looked like a race. The speaker refuses a write
carrying a stale version with UPnP 1028, and the two AVTransport actions the
client uses tell it nothing about the version they just created. So a second
write cannot be correct until a Browse or an event says what the version is
now. Two quick edits are therefore not slow: the second is **refused**.

`Queue:1` hands the new version back on every action that changes the queue, so
a run of edits can carry its own version forward and never wait.

`ReplaceAllTracks` writes a whole queue in one call, which is the only form
that expresses an edit with a gap in it. It needs the metadata of every track
written back, and `parseDIDLTracks` keeps five fields of it: a queue rebuilt
from what the client parses would lose the `<desc>` element that names the
music service account.

### What Queue:1 takes, measured

Read from the SCPD and then sent to a real speaker, against a queue of 23
tracks at version 32:

```
QueueID          0            a ui4, and 0 is the local queue
StartingIndex    1-based      the same as AVTransport, NOT Browse's 0-based offset
UpdateID         32           the SAME counter ContentDirectory reports
```

`Browse` is the trap. Its `StartingIndex` is a pagination offset, which UPnP
defines as counting from 0 in every service, so what it accepts says nothing
about what `ReorderTracks` means by a track index. Reading a 0 there and
sending a 0 to `ReorderTracks` is refused with **402**, not 1028.

The version chain works:

```
ReorderTracks  index 1, insertBefore 3, version 32   ACCEPTED, NewUpdateID 33
  the track moved from position 1 to position 2, and the queue read version 33
ReorderTracks  moving it back, version 33            ACCEPTED, with no read between
ReorderTracks  index 1, insertBefore 3, version 32   REFUSED, UPnP 1028
```

So a run of edits can carry its own version forward and never wait for a
Browse or an event. This also confirms the `InsertBefore` convention that
`MoveQueueTrack` was written to and could not check: moving down names one
place past the destination, and a track sent from index 1 with `InsertBefore`
3 lands at position 2.

The convention holds at the END of the queue too, which is where it looked
most likely to break: `InsertBefore` one past the last track is accepted and
lands the track at the very bottom, not short of it. Verified 2026-09-27 by
sending tracks down with N, singly and three at a time.

### Version 0 means "do not check"

```
ReorderTracks  index 1, insertBefore 3, version 0    ACCEPTED
```

A zero is not treated as a version at all, so the speaker applies the edit
whatever state its queue is in. This is a footgun rather than a shortcut: the
version check is the only thing standing between removing the track the reader
saw and removing whatever has since moved into its place. Send the freshest
version there is, and let 1028 say when it was not fresh enough.

`AttachQueue` was not called. A `Browse` that works answers the same question,
and `AttachQueue` may have an effect of its own.

### Which speakers have Queue:1

Every speaker asked has it:

```
Køkken        Sonos Era 100              yes
Ovenpå        Sonos Port                 yes
Soveværelse   SYMFONISK Picture frame    yes
Spisestue     Sonos Amp                  yes
Værelse       Sonos Era 300              yes
```

Køkken's is confirmed by a write it accepted, the others by what they
advertise. The sample is all S2 hardware, from 2018 to 2023, so it says
nothing about an S1-era player. Read the `serviceList` of a speaker's
description and branch on what is there, rather than on a model or a year.

AVTransport's `RemoveTrackRangeFromQueue` returns `NewUpdateID` on all of
them, so removal needs no second service and no branch at all. `Queue:1` is
only needed for a reorder that chains, since AVTransport's
`ReorderTracksInQueue` returns nothing.

## A group is named after the speaker that coordinates it

`GetZoneGroupState` on any one speaker answers for the whole household: every
group, its members, and which of them coordinates it. The document arrives
escaped inside the SOAP body, the way DIDL does.

```
ZoneGroupState
  ZoneGroups
    ZoneGroup Coordinator="RINCON_…" ID="RINCON_…:3844401901"
      ZoneGroupMember UUID Location ZoneName …26 more attributes
  VanishedDevices
```

The group's `ID` is its coordinator's id and a number. Captured twice, before
and after grouping two rooms:

```
apart    ZoneGroup RINCON_74CA…:3844401901   [Ovenpå]
         ZoneGroup RINCON_804A…:2157789794   [Køkken]

joined   ZoneGroup RINCON_74CA…:3844401901   [Ovenpå, Køkken]
```

Køkken joining left Ovenpå's group id untouched and took Køkken's own away.
So the coordinator IS the group's identity and the number carries nothing: a
model can key on the coordinator and never read the id.

A member carries twenty-nine attributes. Three of them say where it is and
what it is called; the rest are firmware versions, wifi channels, microphone
and calibration state.

### The topology covers one household, and the network may hold more

Discovery found five speakers. The topology listed four, and `VanishedDevices`
was empty. The fifth belongs to a SEPARATE Sonos system on the same network.

So a discovered speaker missing from the topology is not necessarily off: it
may answer to another household entirely, and asking it would return a
different topology. Anything matching speakers to groups has to allow for
finding neither.

### Bonded sets are unrecorded here

No `Invisible`, `ChannelMapSet` or `HTSatChanMapSet` appears in either
capture, which agrees with the household having no stereo pair, surround set
or sub. How a bonded zone is marked is therefore still unknown, and nothing
in this repository should claim otherwise until one has been captured.

### The topology event carries the whole grouping

`ZoneGroupTopology` at `/ZoneGroupTopology/Event` events `ZoneGroupState`
itself, escaped, holding the entire household. Nothing has to be read back:
the event IS the answer, unlike ContentDirectory, which only names the
containers that changed.

Captured 2026-09-27 by grouping one pair while another stayed grouped, then
ungrouping it again.

### Every speaker in a household reports the change, and they disagree on order

One regrouping produced four notifications, one per speaker in that
household. They agree on the grouping and differ on how they write it down:

```
from Ovenpå        ZoneGroup[Ovenpå, Køkken]     ZoneGroup[Soveværelse, Spisestue]
from Køkken        ZoneGroup[Køkken, Ovenpå]     ZoneGroup[Soveværelse, Spisestue]
from Soveværelse   ZoneGroup[Soveværelse, …]     ZoneGroup[Ovenpå, Køkken]
from Spisestue     ZoneGroup[Spisestue, …]       ZoneGroup[Ovenpå, Køkken]
```

Each speaker lists ITSELF first, both among the members of its group and
among the groups. So neither order carries meaning, and two reports of one
arrangement are not equal as strings or as structs. Anything asking whether
the topology changed has to compare it as a set.

The speaker whose own group changed also sends `ZoneGroupName`,
`ZoneGroupID` and `ZonePlayerUUIDsInGroup`; the others send only
`ZoneGroupState`. The first event after SUBSCRIBE carries eleven variables,
which is the usual full replay.

### A separate household hears nothing

The fifth speaker on the network belongs to another Sonos system. Its own
first event reported one group holding only itself, and it sent NOTHING for
either change in the other household -- no empty update, no mention.

So a household is a real boundary in the eventing too: a speaker's silence
says nothing about anyone else's arrangement.


## The music services a system offers, and what that does not say

Verified 2026-10-27 against Ovenpå and Værelse, with `make music-services`.

`MusicServices` (`/MusicServices/Control`) action `ListAvailableServices`
takes no arguments and answers with three things:

- `AvailableServiceDescriptorList`: escaped XML, one `<Service>` per service.
  93 here. Each carries `Id` (the sid in every URI), `Name`, `Uri` and
  `SecureUri` (the service's SMAPI endpoint), `Capabilities` (a bitmask, not
  decoded), a `<Policy Auth=... PollInterval=...>` and a `<Manifest>` URL.
- `AvailableServiceTypeList`: comma-separated numbers, 92 here.
- `AvailableServiceListVersion`: `RINCON_<id>:<n>`, the same on both speakers.

**Auth** is one of `AppLink` (51), `Anonymous` (32) or `DeviceLink` (10).
Apple Music is `AppLink`; DR LYD and myTuner Radio are `Anonymous`, so they
need no account at all.

**Every service type is sid*256+7.** All 92 numbers undo cleanly, and each
names a listed service. That confirms the formula behind the `desc` token
(`SA_RINCON52231_...` for Apple Music, 204) for every service, not just one.

**The type list is not a list of accounts.** It holds every offered service
except TuneIn (254), whose replacement "TuneIn (New)" (333) is in it. Nothing
in the answer says which services this household has signed in to: that is
still only visible from the sid/sn pairs in favourites, queues and sources.

**Available, as the experiment counts it, means it streams here:** the auth is
`Anonymous`, or a sid/sn pair for the service shows up in the household's
favourites (FV:2) or the speaker's queue. Measured on Ovenpå: 33 of 93, three
in use (Apple Music sn 2, DR LYD sn 3, myTuner Radio sn 5) and 30 that need no
account. A service signed in to but never used in a favourite or a queue is
missed: nothing on the speaker names it.

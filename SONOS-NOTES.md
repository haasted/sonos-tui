# Notes

Protocol findings about how Sonos actually behaves, as distinct from how the
docs say UPnP works. Everything here was verified against the real event
captures in `captured-data/` and `internal/sonos/testdata/`, recorded with
`cmd/sonos -capture`, rather than taken from documentation. The earliest
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
"add this later", play history (see ADR-0003 and `son-3af`). A durable scheme
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

**Relevant to `son-a30`** (the UPnP 701 "transition not available" bug on
previous-track): `AVTransportURI` is the diagnostic that issue calls decisive.
`x-rincon-queue:` is a real queue where "previous" works; `x-sonosapi-radio:`
is a forward-only station where it cannot. The field already arrives in every
event — `ParseAVTransportEvent` currently parses and then discards it.

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
   separate service this program doesn't subscribe to yet (`son-yop`).
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

Verified 2026-09-09 against a 31-event capture (`cmd/sonos -capture`).

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
playlist a queue was populated *from* lives in `EnqueuedTransportURIMetaData`,
which is **not** among the 25 fields of the SUBSCRIBE snapshot — it appears
only when the queue source changes while already subscribed.

**Captured bodies are double-escaped**: `LastChange` once, the DIDL inside it
again. Two unescape passes to read one by eye; `parseDIDLTrack` already handles
this.

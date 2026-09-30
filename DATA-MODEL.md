# Data model

The structs this project defines, what each is for, and how they relate.

Written as a map for someone arriving at the code. It describes the shape of
the data, not the behaviour — for *why* particular decisions were made, see the
`ADRs/` directory and the beads issue tracker; for what the speakers actually
send on the wire, see `SONOS-NOTES.md`.

## The one organising idea

Types are layered, and the layers only ever depend downward:

```
cmd/sonos          appModel, speakerModel, messages
                        │  owns UI components, projects domain onto them
internal/ui/…      Tabs, Modal, nowplaying.Model, marquee.Line
                        │  knows nothing about Sonos
internal/sonos     Device, DeviceState, Event, SonosClient
                        │  knows nothing about terminals
                   the speakers
```

`internal/ui` contains no Sonos concept and `internal/sonos` contains no
terminal concept. Everything that has to know about both lives in `cmd/sonos`,
which is where the projection from one to the other happens. Three of the
categories below are on one side of that line, one is on the other, and the
line is the most load-bearing thing in the codebase.

---

## 1. Speaker identity — who a speaker is

| Type | Where | What it is |
|---|---|---|
| `Speaker` | `sonos/discovery.go` | A discovered speaker: IP, description URL, USN |
| `DeviceDescription` | `sonos/description.go` | What `device_description.xml` says: UDN, room name, model |

`Speaker` is the identity, and it is **copied by value everywhere** — sent on
channels, embedded in `Device`, stored in the cache. Its `description` field is
therefore a *pointer to* an `atomic.Pointer`, not an `atomic.Pointer` itself:
discovery fills the description in later, on a background fetch, and every copy
of the `Speaker` has to see that when it lands. An `atomic.Pointer` copied by
value would give each copy its own slot and the fill would reach only one of
them.

`DeviceDescription.UDN` is the identity half of the SSDP `USN` (`USN` is the
UDN plus `::<service>`). That prefix relationship is what lets the cache prove
a remembered IP still belongs to the same physical speaker rather than to
whatever took its DHCP lease.

## 2. Speaker state — what a speaker is doing

| Type | Where | What it is |
|---|---|---|
| `Device` | `sonos/device.go` | One speaker: identity, transport, and last-known state |
| `DeviceState` | `sonos/device.go` | A point-in-time snapshot of that state |
| `TransportInfo` | `sonos/transport.go` | Playback state — `PLAYING`, `PAUSED_PLAYBACK`, `STOPPED` |
| `SourceInfo` | `sonos/events.go` | What is *loaded* — a queue, a station, a line-in |
| `TrackMetadata` | `sonos/events.go` | What is *playing* — title, artist, album, radio show |

`Device` is the only pointer type in the domain and the only mutable one. It
embeds `Speaker` for identity, holds a `*SonosClient` for talking to the
speaker, and guards a `DeviceState` behind a mutex. `State()` returns a copy, so
callers can never mutate what the `Device` believes.

**`SourceInfo` and `TrackMetadata` answer different questions**, and conflating
them is the mistake this model exists to prevent. A radio station is the
*source*; the song it happens to be playing is the *track*. An algorithmic
station plays discrete tracks with real durations, while a broadcast station
plays nothing that can be called a track at all — it sends no title, artist or
album, only a station name on the source and a programme description on the
track.

`SourceInfo.Kind()` classifies by URI scheme into `SourceTrack`,
`SourceBroadcast`, `SourceStation`, `SourceLineIn` or `SourceUnknown`.
`DeviceState.Live()` deliberately asks about the *duration* rather than the
kind, because "is a stream" and "has a usable clock" are separate questions.

## 3. Events — what the speakers push

| Type | Where | What it is |
|---|---|---|
| `AVTransportEvent` | `sonos/events.go` | A parsed AVTransport `LastChange` |
| `RenderingControlEvent` | `sonos/renderingevents.go` | A parsed RenderingControl `LastChange` — volume and mute |
| `Event` | `sonos/device.go` | What a `Device` publishes to its consumer |

The pipeline runs **NOTIFY body → parsed event → `Event`**, and the split at
its far end is deliberate.

`Event` exists because consumers should not care which service a change came
from, only what changed. Its `Kind` field is the exception: a RenderingControl
event carries no playback position, so treating it as one would reset the clock
to a stale value.

Two small internal types serve the XML: `val` (an element with a `val`
attribute, the shape every `LastChange` variable takes) and `channelVal` (the
same plus a `channel` attribute, because RenderingControl reports volume once
per audio channel and only `Master` is wanted).

## 4. Talking to speakers — SOAP and subscriptions

| Type | Where | What it is |
|---|---|---|
| `SonosClient` | `sonos/client.go` | A stateless SOAP transport pointed at one speaker |
| `TrackInfo` | `sonos/transport.go` | The reply to `GetPositionInfo` |
| `UPnPError` | `sonos/client.go` | A decoded SOAP fault, with the UPnP error code |
| `soapFault` | `sonos/client.go` | The wire shape `UPnPError` is decoded from |
| `EventManager` | `sonos/eventmanager.go` | Owns the callback listener and every subscription |
| `eventListener` | `sonos/eventlistener.go` | The HTTP server speakers POST NOTIFYs to |
| `route` | `sonos/eventlistener.go` | One subscription's destination: a device and a service |
| `subscription` | `sonos/eventmanager.go` | One live GENA subscription and its renewal loop |

`SonosClient` holds no state — it is a transport, and `Device` is the layer that
remembers anything. That separation is why a `Device` can be pointed at a test
server by constructing it with a different client.

**`TrackInfo` and `TrackMetadata` describe similar things and are not the
same.** `TrackInfo` is the SOAP reply shape (flat, all strings, includes queue
position and elapsed time); `TrackMetadata` is the parsed DIDL-Lite shape that
events produce. They meet in `DeviceState`, which stores the event shape.

`UPnPError` matters because Sonos reports refused actions as HTTP 500 with the
reason in the body. Matching on `Code` with `errors.As` is how a caller tells
"that speaker is unreachable" from "that action is not available on a radio
station".

`route` is what makes the listener multi-speaker: each subscription gets a URL
whose path is a hash of the device and service, so an arriving NOTIFY is
attributed without inspecting its contents.

## 5. Persistence — what survives a restart

| Type | Where | What it is |
|---|---|---|
| `SpeakerCache` | `sonos/speakercache.go` | The speaker list from a previous run, plus the context to judge it |

`SpeakerCache` carries a `Subnet` and a `Selected` USN alongside the speakers. The
subnet is a *speed* optimisation and never a correctness mechanism: it cheaply
rules out a cache recorded on a different network, but only a real call to a
speaker proves an entry is still good.

It holds `[]Speaker` outright, because it is the *mapped* form: `LoadCache`
reads a `store.Snapshot` and turns each row into a `Speaker`, and `SaveCache`
goes the other way. That mapping is the only place the two vocabularies meet,
and it is why `internal/sonos` imports `internal/store`.

### The SQLite store — `internal/store`

| Type | Where | What it is |
|---|---|---|
| `Store` | `store/store.go` | The open database, migrated to the schema this build knows |
| `Cache` | `store/cache.go` | The only handle on the cached speakers |
| `Snapshot` | `store/cache.go` | A whole cache: the speakers, and the context that judges them |
| `CachedSpeaker` | `store/cache.go` | One cached speaker, as eight plain strings |

One SQLite file under `os.UserConfigDir()`, holding the speaker cache and,
later, playlists and remembered tracks — see ADR-0003. It sits *below*
`internal/sonos` in the layering and knows nothing about Sonos or terminals.

**`store.Cache` exists to be narrow.** ADR-0003 put disposable cache data and
irreplaceable user data in one file, and the mitigation it owed is this type:
it carries no method that accepts SQL, so code holding one can clear the cache
and reach nothing else. Widening that means editing the type, not calling it
wrongly.

The schema version lives in `PRAGMA user_version` — four bytes SQLite keeps in
the file header and never reads itself. `migrations` is append-only, and each
step commits with its version bump in one transaction, so an interrupted
migration leaves the file at a version it really has.

`CachedSpeaker` is eight plain strings rather than anything richer, which is
the layering holding: this package cannot name a `DeviceDescription` without
knowing what a speaker is. `Save` replaces every row in one transaction, since
the speakers on screen at exit are the whole truth and a reader must never see
half of them.

`store.Cache` is the handle and `sonos.SpeakerCache` is what one read through
it becomes. Neither is the other's replacement; they sit on opposite sides of
the mapping, which is why neither is called plain `Cache`.

## 6. UI components — terminal, no Sonos

| Type | Where | What it is |
|---|---|---|
| `Tabs` | `ui/panes/tabs.go` | A tab bar with content, a divider, and its animations |
| `Tab` | `ui/panes/tabs.go` | One entry: title, content model, playing flag |
| `Modal` | `ui/panes/modal.go` | An overlay, hovering or full-screen |
| `NoticeMsg` / `notice` | `ui/panes/notice.go` | A transient message set into the divider rule |
| `NoticeEntrance` | `ui/panes/notice.go` | Whether a notice opens out of the rule, and in which style |
| `Model` | `ui/nowplaying/nowplaying.go` | The now-playing widget |
| `Track` | `ui/nowplaying/nowplaying.go` | What the widget renders — *not* a domain type |
| `TrackMsg` / `VolumeMsg` | `ui/nowplaying/nowplaying.go` | Updates to the widget |
| `Line` | `ui/marquee/marquee.go` | Text plus a window, scrolled by advancing an offset |
| `Ramp` | `ui/flashsweep/flashsweep.go` | The base and flash colours a glint travels between |
| `Breath` | `ui/flashsweep/breath.go` | A breathing light, as a pair of gaussians rather than a sine |
| `Reveal` / `RevealStyle` | `ui/flashsweep/reveal.go` | A centre-out text entrance, and which of four looks it wears |

**`nowplaying.Track` is a projection, not a domain object.** It looks like
`TrackMetadata` and is not: it holds a `StreamLabel`, a `Volume` and durations
as `time.Duration`, because those are what the widget draws. `cmd/sonos` builds
one from a `DeviceState`, and that function is where every Sonos concept is
translated into a rendering concept.

`Tab.Playing` is a plain `bool` on the tab rather than something read off the
content, because `panes` must not know what a speaker is. Keeping it correct is
the owner's job — `appModel` re-derives it after every message rather than
setting it at each site that changes playback.

`Modal` reports whether the frame needs the alternate screen and lets the
caller act on it, rather than building a view itself. A reusable component
should not be tied to one program's rendering lifecycle.

Several near-identical `…TickMsg` structs (`scrollTickMsg`, `fadeTickMsg`,
`positionTickMsg`, `dividerTickMsg`, `tabSweepTickMsg`, `noticeTickMsg`,
`labelTickMsg`) each carry a single `gen int`. That field is not decoration: a
tick whose generation no longer matches is discarded, which is what stops an
animation from being driven by a chain that should have died. Two separate bugs
in this project's history were exactly that failure.

## 7. Effects core — `internal/ui/movy`

Vendored from movy-go (MIT), used by the `experiments/uitest` effect demos.

| Type | What it is |
|---|---|
| `Rgb` / `V3` | A colour as bytes, and as floats for arithmetic |
| `Frame` | A render target with solid, glow and scanline layers |
| `RenderSurface` | A colour map plus a shadow map, sized to a grid |
| `Sprite`, `SpriteFrame`, `SpriteFrameSet`, `FrameAnimation` | Sprite animation |
| `SpritePool` | Reuse of sprite instances |
| `IndexAnimator` | Steps an index with a loop mode — once, forward, backwards, bounce |

`Rgb` and `V3` being separate is the point: `Rgb` is what a terminal cell needs,
`V3` is what blending needs without rounding at every step.

## 8. Application — `cmd/sonos`

| Type | What it is |
|---|---|
| `appModel` | The root Bubbletea model: tabs, help modal, event manager, frame height |
| `speakerModel` | One speaker's tab: its `Device`, its widget, its volume debounce |
| `helpContent` / `helpKey` | The help popup's content and one row of its key table |

`appModel` owns a `panes.Tabs` whose every `Tab.Content` is a `speakerModel`,
and each `speakerModel` owns a `*sonos.Device` and a `nowplaying.Model`. That
chain is the whole application:

```
appModel ── Tabs ── Tab.Content ── speakerModel ── Device      (domain)
                                              └─── nowplaying.Model  (view)
```

### Messages

| Type | What it carries |
|---|---|
| `speakerFoundMsg` | Discovery found a speaker |
| `speakerReadyMsg` | Its state loaded, with anything that failed |
| `speakerVerifiedMsg` | A cached speaker answered, with anything that failed |
| `speakerGoneMsg` | A cached speaker did not answer |
| `discoveryDoneMsg` | Discovery finished |
| `speakerMsg` | **A result addressed to the speaker that issued it** |
| `volumeCommitMsg` | A volume debounce window closing |

`speakerMsg` is the one worth understanding. Every speaker command is a network
round trip, and its result carries no speaker of its own. Without an address it
reaches whichever tab is active *when the reply lands*, which repaints the wrong
room. It wraps the payload with the `*Device` that issued it, and `appModel`
routes it by pointer.

## 9. Support and playground

`internal/log` holds `Buffer` (log lines collected in memory and printed on
exit, so they never interleave with the TUI) and `ElapsedHandler` (a `slog`
handler stamping time since start).

`experiments/uitest` defines around thirty structs — `…Effect` types, mock tracks, demo
root models. They are a playground for iterating on look and feel, deliberately
disposable, and nothing outside `experiments/uitest` depends on them.

`cmd/listening`, an earlier event-listening prototype with its own subscription
and cache types, has been deleted — see ADR-0004. Everything it did now lives in
`internal/sonos` and `cmd/sonos`.

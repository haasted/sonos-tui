# 2. Subscribing to live speaker state via GENA eventing

## Status

Accepted — implemented in `internal/sonos` (`subscribe.go`, `eventlistener.go`, `eventmanager.go`, `Device.HandleEvent`/`Events`) and wired into `cmd/sonos`.

## Context

`Device` currently only learns a speaker's state by asking for it: `Refresh()` polls `GetTransportInfo`/`GetPositionInfo`, and `Play()`/`Pause()` optimistically guess the resulting state locally. Both are either stale (state drifts between polls) or approximate (a command might not actually take effect, and nothing notices).

UPnP defines an eventing mechanism for exactly this — **GENA** (General Event Notification Architecture), layered on plain HTTP:

1. `SUBSCRIBE` a service's event URL (e.g. a speaker's `/MediaRenderer/AVTransport/Event`), with a `CALLBACK` header pointing at a URL on *our* machine.
2. The speaker replies with a `SID` (subscription ID) and a `TIMEOUT`.
3. From then on, whenever relevant state changes, the speaker itself opens an HTTP connection back to our `CALLBACK` URL and sends a `NOTIFY`. The body is a `LastChange` element — XML, itself further-escaped like the DIDL-Lite metadata `GetPositionInfo` already unescapes — bundling *every* field that just changed into one push.
4. Before `TIMEOUT` elapses, a `SUBSCRIBE` carrying the same `SID` renews it, or the subscription silently lapses and `NOTIFY`s stop arriving.
5. `UNSUBSCRIBE` with the `SID` tells the speaker to stop calling us — being a "good citizen" means doing this on shutdown rather than leaving dangling subscriptions for the speaker to eventually time out on its own.

The part that matters architecturally: this makes our program an HTTP **server** as well as a client — the speaker calls back into us. Nothing in this codebase has needed an inbound HTTP surface before; `Discover`'s UDP listener is the closest existing precedent for "background goroutine owning a network resource."

Scope for this first pass was **AVTransport only** (see the amendment below, which added RenderingControl) (play/pause/track-change state — the service that actually changes during normal listening). `RenderingControl` (volume/mute) is deliberately deferred: both services share identical GENA mechanics and a single bundled `LastChange` variable, so the subscribe/listen/renew plumbing built here is expected to be directly reusable — only the inner `LastChange` schema differs.

## Options considered

### Callback listener topology

One `http.Server` per `Device`, or one shared server for the whole process?

Per-`Device` means binding a port per subscribed speaker for no benefit — GENA's natural topology is one listener, many subscriptions, distinguished by path or `SID`.

**Decision: one shared listener**, started lazily on first subscription, with incoming `NOTIFY`s demuxed by a per-device path.

### Event delivery to calling code

How should a parsed event actually reach whatever's using `Device`?

- Transparently refresh `Device`'s existing cached `DeviceState` only — no new API surface, but silently loses the "something just changed" signal a reactive UI needs.
- Cache refresh **and** an explicit event channel.
- Channel only, dropping the cache — breaks every existing caller of `State()` for no reason.

**Decision: cache refresh and an event channel.** `Device` keeps updating `DeviceState` in place (same pattern as the existing optimistic update after `Play()`/`Pause()`), and separately exposes a receive-only channel of a plain, UI-framework-agnostic `sonos.Event`. Bubbletea defines `type Msg any`, so any concrete Go value already satisfies it — a small bridge goroutine can forward straight from this channel into a running `*tea.Program` via `Send(event)`, and a model's `Update()` type-switches on `sonos.Event` directly. No adapter/wrapper type needed, and `internal/sonos` never imports Bubbletea.

### Subscription renewal

Build automatic renewal now, or ship a single `SUBSCRIBE` that just expires (~30 min, typically) and revisit renewal later?

Deferring is less code today, but means the feature silently goes quiet a half hour in — a worse first experience than the modest cost of a renewal timer, and "revisit later" has a habit of not happening.

**Decision: build it now** — a timer that re-`SUBSCRIBE`s with the existing `SID` before `TIMEOUT` elapses.

### Service scope for v1

`AVTransport` only, `RenderingControl` only, or both together?

Both together doubles the `LastChange`-parsing surface before either is proven; `RenderingControl`-only skips the service that's actually dynamic during normal use.

**Decision: `AVTransport` only** (see Context) — `RenderingControl` is expected to be a small follow-on addition once this works.

## Decision

```mermaid
flowchart TD
    Discover[Discover finds a speaker] --> Subscribe["SUBSCRIBE to AVTransport Event URL<br/>CALLBACK: shared listener + per-device path"]
    Subscribe --> SID[Speaker replies: SID + TIMEOUT]
    SID --> Listen[Shared HTTP listener waits for NOTIFY on that path]
    Listen -->|NOTIFY arrives| Parse[Unescape + parse LastChange XML]
    Parse --> UpdateState[Update Device's cached DeviceState]
    Parse --> Emit[Send sonos.Event on channel]
    Emit -.->|"tea.Cmd waits on the channel — tea.Msg is any"| UI[Any Bubbletea model's Update]
    SID --> Renew[Renewal timer]
    Renew -->|before TIMEOUT| Resubscribe[Re-SUBSCRIBE with same SID]
    Resubscribe --> Renew
    Shutdown(["Program exit / Ctrl+C"]) --> Unsubscribe[UNSUBSCRIBE every active SID]
    Unsubscribe --> Done([Clean exit])
```

### `cmd/listening` — the verification harness *(retired — see ADR-0004)*

A new, deliberately simple command: discover every speaker on the LAN, subscribe to all of them, log every event to the console as it arrives (`log/slog` with [`lmittmann/tint`](https://github.com/lmittmann/tint) for human-readable, colorized output rather than raw JSON), and unsubscribe from everything on `Ctrl+C` before exiting. No interactive input in this first version — purely observational, run it and watch. This doubles as both the day-to-day development tool for building this feature and the natural shape for the `-live`-gated integration test (mirroring `discovery_test.go`/`ssdp_eval_test.go`), alongside `httptest`-based unit tests for the outbound `SUBSCRIBE`/renewal flow and the inbound listener (synthetic `NOTIFY` POSTs, no real speaker needed).

### Questions resolved during implementation

- **`sonos.Event` shape**: `{ Speaker, Transport TransportInfo, Current TrackMetadata, Raw AVTransportEvent }`. The two fields a UI almost always wants are lifted out; `Raw` keeps everything else the speaker sent (next track, queue source, play mode) so nothing parsed is thrown away.
- **Per-device path/key for the shared listener's demuxing**: per-device path, derived from a hash of `Speaker.USN` (raw USNs are long `uuid:RINCON_...::urn:...` strings, so they're hashed to keep callback URLs short and path-safe). Routing on path rather than the `SID` header means a renewal that changes the SID, or a late NOTIFY from a dropped subscription, can't be misattributed to another speaker.
- **Subscribe timing relative to discovery**: as each speaker streams in, confirming the leaning. Subscription happens in the same command that does a room's initial state fetch.
- **Shared listener bind address/port**: OS-assigned ephemeral port, bound in `newEventListener` before any `SUBSCRIBE` goes out, so the `CALLBACK` URL always names a resolved port.
- **Signal handling**: no bespoke handler was needed. Bubbletea installs its own `SIGINT`/`SIGTERM` handlers and returns from `Run()` on either, so `EventManager.Close` runs after `Run()` returns and covers `q`, `Ctrl+C` and `kill` alike.

Two decisions were revised against what this ADR originally sketched, both for shutdown safety:

- **Event delivery is a non-blocking send, not a blocking one.** `HandleEvent` runs on the listener's own request goroutine; a consumer that has stopped receiving (the UI shutting down) would wedge that goroutine and in turn stall `Shutdown`. State is applied before the send, so a dropped event costs a redraw, not correctness.
- **The bridge to Bubbletea is a `tea.Cmd` that waits on the channel, not a goroutine calling `program.Send`.** Equivalent in effect, but it avoids handing a `*tea.Program` to code that constructs the model, which is awkward given the program needs the model to exist first.

## Amendment: RenderingControl picked up (son-6f7)

The deferred `RenderingControl` scope above has been implemented. Two
expectations recorded here proved right, one proved wrong.

**Right:** the subscribe/renew/unsubscribe plumbing was directly reusable, and
the callback listener needed no new inbound surface.

**Wrong: "only the inner `LastChange` schema differs."** Two things beyond the
schema had to change.

*The callback path was keyed per device, not per (device, service).*
`notifyPath` hashed the device USN alone, so a second subscription for the same
device would have collided on one path with no way to tell the two services'
NOTIFYs apart. Paths now hash `USN + "|" + service`. Note this keeps the
original decision to route by path rather than by the GENA `SID` header —
that reasoning (SID changes on renewal, risking misattribution) applies to
service dispatch exactly as it did to device dispatch, so the SID is still
never used for routing.

*`Device.HandleEvent` hardcoded `ParseAVTransportEvent`.* It is now
`HandleAVTransportEvent` and `HandleRenderingControlEvent`, chosen by the
route's service. This is not cosmetic: the AVTransport handler refetches
playback position via `GetPositionInfo` when the event omits it, which it
almost always does. Sending volume events down that path would have fired a
needless SOAP call on every volume change.

**The schema difference was also sharper than anticipated.** RenderingControl
uses the `.../RCS/` namespace rather than `.../AVT/`, and — the part worth
recording — `Volume` and `Mute` are repeated **once per channel**:

```xml
<Volume channel="Master" val="32"/>
<Volume channel="LF" val="100"/>
<Volume channel="RF" val="100"/>
```

A parser taking the first or last `<Volume>` would report 100 where the
speaker's actual volume is 32. Both fields are filtered to `Master`. This was
found by capturing a real event from a speaker rather than inferring the shape;
the capture is kept as a fixture in `internal/sonos/testdata/` and the parser is tested against it.

**Partial failure is tolerated.** `Subscribe` now attempts every service and
joins the errors rather than abandoning the device on the first failure — a
speaker with live playback events but no volume events is more useful than one
with neither.

## Consequences

`internal/sonos` gains real new complexity: an inbound HTTP surface, background goroutines (per-subscription renewal, the shared listener), and shutdown/cleanup concerns that don't exist anywhere else in the codebase yet. `Device`'s public API grows (an events channel alongside `State()`/`Refresh()`/`Play()`/`Pause()`), but nothing existing has to change — polling via `Refresh()` keeps working exactly as today for callers that don't care about live updates. `RenderingControl` eventing, once wanted, is expected to be a small addition on top of this same machinery.

## Amendment: `cmd/listening` retired (son-cg8)

This ADR built GENA eventing inside `cmd/listening` and describes it above as
the verification harness. It was exactly that, and it is now gone.

son-0rd moved the mechanics into `internal/sonos`, after which `cmd/listening`
was a second implementation of subscription handling that nothing exercised.
Every capability that justified it had moved to `cmd/sonos` — playback controls,
live events, the speaker cache, and finally raw NOTIFY capture, which was the
last thing it could do that `cmd/sonos` could not.

**ADR-0004 supersedes the harness section above.** The description of what
`cmd/listening` did is left intact rather than rewritten: it records accurately
what was decided at the time, and rewriting history to match the present would
lose the reasoning.

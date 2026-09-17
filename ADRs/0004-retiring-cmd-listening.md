# 4. Where eventing lives, and retiring `cmd/listening`

## Status

Accepted — `EventManager` lives in `internal/sonos`; `cmd/listening` is deleted.

## Context

ADR-0002 introduced GENA eventing and built it inside `cmd/listening`, a
command it explicitly describes as "the verification harness": discover every
speaker, subscribe to all of them, log what arrives, unsubscribe on exit. That
was the right shape for developing the feature — a place to watch real speakers
without a UI in the way.

The mechanics then had to move somewhere reusable, because `cmd/sonos` needed
them too. They were extracted into `internal/sonos` as `EventManager`,
`eventListener` and `Device.Handle*Event`, with this ADR deferred until now.

That extraction left `cmd/listening` as a **second implementation of the same
thing**: it still carries its own `subscribe`, `renew`, `unsubscribe`,
`handleNotify` and `http.Server`, and reaches into `internal/sonos` only for
`Speaker`, `Discover` and `ParseAVTransportEvent`. Two copies of subscription
handling, one of them exercised on every run and one of them not at all.

Meanwhile every capability that made it worth keeping moved into `cmd/sonos`:

- Playback controls
- Live GENA events
- The speaker cache
- Raw NOTIFY capture

The last of those mattered most: `-capture` was the final thing `cmd/listening`
could do that `cmd/sonos` could not, so until capture landed there, deleting it
would have been a capability loss.

## Decision

**Eventing lives in `internal/sonos`, not a package of its own.** Subscriptions
exist to keep `Device` state current; splitting them into, say,
`internal/gena` would mean a package whose only purpose is to call back into
`internal/sonos`, plus an exported surface between two halves of one idea. The
package boundary that earns its keep in this codebase is the one between the
domain and the terminal, not one inside the domain.

**`cmd/listening` is deleted rather than kept as a thin wrapper.** A wrapper
would be a second entrypoint to maintain, document and keep compiling, whose
distinguishing feature is that it does less. `cmd/uitest` earns its place by
doing something `cmd/sonos` deliberately cannot — iterate on look and feel with
disposable code. `cmd/listening` would earn its place only by being smaller.

**The refactor-then-delete step is skipped.** The plan originally specified
refactoring `cmd/listening` onto `EventManager` first, as a behaviour-preserving
check that the extraction had not regressed anything, and only then
deleting it. That check has since been overtaken: `cmd/sonos` exercises
`EventManager` against five real speakers with two subscriptions each, and the
capture corpus is direct evidence of NOTIFYs being routed and parsed
correctly. Refactoring a command in order to delete it would produce work with
no artifact and weaker evidence than already exists.

One caveat, recorded rather than glossed: **renewal is the one path neither
approach demonstrably verifies.** `renewInterval` is four minutes, and a typical
run is shorter. A refactored `cmd/listening` would have verified it no better
unless deliberately left running — which `make run` can do for free. Tracked
separately, since it is unrelated to this deletion.

## The resulting public surface

What `internal/sonos` now offers for eventing, the shape this ADR is ratifying:

```go
// Subscriptions
func NewEventManager(opts ...EventManagerOption) (*EventManager, error)
func WithCapture(capture CaptureFunc) EventManagerOption
func (em *EventManager) Subscribe(ctx context.Context, device *Device) error
func (em *EventManager) Close(ctx context.Context) error

// Consumption
func (d *Device) Events() <-chan Event
func (d *Device) State() DeviceState

// Inbound, called by the listener rather than by application code
func (d *Device) HandleAVTransportEvent(body []byte) error
func (d *Device) HandleRenderingControlEvent(body []byte) error
```

Three properties of that surface are deliberate:

- **`Subscribe` takes a `*Device`, not a `Speaker`.** A subscription's whole
  purpose is to update something; taking the thing it updates makes the wiring
  unambiguous and stops a caller subscribing to a speaker with nowhere to put
  the result.
- **`EventManager` owns the listener.** One HTTP server for the process, many
  subscriptions demultiplexed by path. Callers never see the listener, the port
  or the callback URLs.
- **Options rather than a widening constructor.** `WithCapture` was added later
  without disturbing existing callers, which is the pattern to follow for the
  next thing that needs to observe the raw stream.

## Consequences

`cmd/listening` and its `main.go`, `cache.go`, `cache_test.go` and `prompt.go`
are gone. Its JSON speaker cache was reimplemented properly in `internal/sonos`
and its capture ability in `cmd/sonos`; nothing else in it was unique.

ADR-0002's "`cmd/listening` — the verification harness" section is amended
rather than rewritten. It records accurately what was decided at the time, and
this ADR is the successor.

The repository now has one entrypoint that talks to speakers, `cmd/sonos`, and
one playground that does not, `experiments/uitest`.

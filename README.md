# sonos-tui

A terminal client for Sonos speakers that talks straight to them over your own
network — no cloud and no account needed to use it.

Two things it is trying to be. **Fast**: it remembers the speakers it found last
time, so your rooms are on screen before the network has finished answering, and
pressing a key acts on the speaker rather than on a spinner. **Pleasant to
live with**: it renders inline instead of taking over the terminal, gives the
screen back exactly as it found it, and spends a little effort on looking good
while it is open.

Still a sandbox — it is also where the local Sonos API gets mapped out, and
`SONOS-NOTES.md` is the field guide that has come out of that.

## Principles

Five rules every feature is held to. When a design has to bend one, that is a
decision to make out loud, not a side effect.

**Local first, cloud optional.** Everything the speakers can do on their own
works without signing in to anything: playback, the queue, grouping,
favourites, history, the local music library, radio that needs no account.
Features that need Sonos's cloud or a music service's login — searching Apple
Music, say — are additions you switch on by signing in, in one place, and can
switch off again. Without them the rest of the program is whole: nothing waits
for the cloud, and a cloud feature that fails takes only itself down, with a
notice, never the program.

**Private.** What you play and what you do stays on your machine. A cloud
session, once you sign in, is kept locally and goes only to the service it
belongs to; your password is not stored, and no secret is written to a log.
There are no metrics, no telemetry and no crash reports sent anywhere, and
nothing watches how you use the program. Your history and favourites are a
file on your own disk, for you alone.

**Fast and responsive.** A key acts at once. The program shows the result of a
command before the speaker has confirmed it and corrects itself if the speaker
disagrees, rather than making you watch a spinner. It remembers what it found
last time, so your rooms are on screen before the network has answered. Slow
work — the network, the database — happens in the background, never between a
keypress and the screen.

**Calm, uncluttered, and still.** The screen shows what you need and no more;
anything else is a key away. Things do not jump around: a heading keeps its
width as its contents change, a list keeps your place when it updates, a
notice borrows the line it sits on rather than pushing the layout down, and an
animation is brief and settles where it started. Motion is there to tell you
something happened, not to decorate.

**Keyboard first.** Everything can be done from the keyboard, without reaching
for the mouse. Keys are short, mirror each other where they can (a key and its
capital do related things), mean the same thing in every list, and are listed
under `?` wherever you are. The mouse is welcome where it is natural, like
scrolling a list, but never required.

## Layout

`cmd/sonos` is the program. Everything else exists to serve it.

| Package | What lives there |
|---|---|
| `cmd/sonos` | The client itself — the entrypoint you run |
| `internal/sonos` | The domain: discovery, SOAP, GENA events, device state, the speaker cache |
| `internal/ui/…` | Reusable terminal components — tabs, the now-playing panel, marquee, effects |
| `internal/log` | Logging buffered in memory, so it never interleaves with the UI |
| `experiments/` | A module of its own: the UI demos, and library evaluations whose dependencies stay out of the client |

`internal/ui` knows nothing about Sonos and `internal/sonos` knows nothing about
terminals; `cmd/sonos` is the only place that knows both. `DATA-MODEL.md` maps
the types and their relationships.

## Quickstart

```bash
make run            # discover your speakers and start the client
make test           # run everything (never cached)
make build          # build the binary
make uitest         # list the UI demos; make uitest <name> runs one
make help           # every target
```

Press `?` in the client for the keys.

Flags, for when something needs looking at:

```bash
go run ./cmd/sonos -logs=false          # discard logs instead of printing them on exit
```

`make run-nocache` wraps the one you reach for most — the
latter deletes the speaker cache first, to exercise a cold start.

## Speaker API coverage

| Feature | Method |
|---|---|
| Play | `c.Play()` |
| Pause | `c.Pause()` |
| Stop | `c.Stop()` |
| Next track | `c.Next()` |
| Previous track | `c.Previous()` |
| Playback state | `c.GetTransportInfo()` |
| Current track + metadata | `c.GetPositionInfo()` |
| Get volume | `c.GetVolume()` |
| Set volume | `c.SetVolume(40)` |
| Get mute | `c.GetMute()` |
| Set mute | `c.SetMute(true)` |
| Adjust volume relatively | `c.SetRelativeVolume(-2)` |
| Discover speakers | `sonos.Discover(ctx)` |
| Subscribe to live events | `events.Subscribe(ctx, device)` |

## How It Works

Sonos speakers expose a local HTTP server on **port 1400** implementing the
UPnP/SOAP standard. Commands are `POST` requests with an XML body and a
`SOAPAction` header. No cloud or authentication required — everything runs
entirely on the local network.

Speaker discovery uses **SSDP** (Simple Service Discovery Protocol) — a UDP
multicast to `239.255.255.250:1900` with the Sonos-specific service type
`urn:schemas-upnp-org:device:ZonePlayer:1`.

## Concepts

A speaker exposes several UPnP services. Two matter here, and the split between
them explains most of the code:

| | AVTransport | RenderingControl |
|---|---|---|
| Question | What's playing, and is it? | How loud, and how does it sound? |
| Variables | transport state, track, URI, duration, position | volume, mute, bass, treble, loudness, EQ |
| In this app | `AVTransportEvent`, the now-playing panel | `RenderingControlEvent`, volume and mute |

Both are standard UPnP AV services — Sonos implements them rather than having
invented them, though it extends both heavily.

Each is used in two directions. **Outbound**, commands are SOAP `POST`s to that
service's control endpoint. **Inbound**, we hold a **GENA** subscription to its
event endpoint, so a change made in the Sonos app or on the speaker itself
reaches us without polling. Events arrive as a `LastChange` document listing
only the variables that *changed* — with the exception of the first event after
subscribing, which is a full snapshot.

Two things about these events are worth knowing before reading the parsers:

- **RenderingControl reports `Volume` and `Mute` once per audio channel**, and
  the values differ. `Master` is the mixed output a user thinks of as "the
  volume"; `LF`/`RF` are per-driver trims usually pinned at 100. Taking the
  first element rather than filtering on `Master` yields 100 every time.
- **A RenderingControl event carries no playback position.** Treating one like
  an AVTransport event resets the clock to a stale value, which is why
  `sonos.Event` carries a `Kind`.

RenderingControl also carries far more than volume — subwoofer tuning, surround
setup, Trueplay calibration, night mode — all on a subscription already held,
should any of it become interesting.

For the wire-level detail see `SONOS-NOTES.md`; for the types these become, see
`DATA-MODEL.md`.

## Acknowledgements

Bubbletea skill from https://github.com/GGPrompts/my-plugins/blob/main/plugins/specialized/bubbletea/skills/bubbletea/SKILL.md

Bubbletea v2 skill from https://github.com/rigerc/bubbletea-v2-scaffold (Apache License 2.0), `.claude/skills/bubbletea-v2`

Bubbles v2 skill from https://github.com/rigerc/bubbletea-v2-scaffold (Apache License 2.0), `.claude/skills/bubbles-v2`

Terminal graphics core derived from https://github.com/M64GitHub/movy (MIT License, Copyright (c) 2025 M64), vendored via a Go port at `internal/ui/movy` — see `internal/ui/movy/LICENSE`
# experiments

Framework/library evaluations, kept as a record of what was tried and why it
was or wasn't adopted. Nothing here is imported by production code, and
production code never imports from here.

- `ssdp/` — evaluates `github.com/koron/go-ssdp` against the hand-rolled SSDP
  search in `internal/sonos/discovery.go`. Gated behind `-live` (needs real
  Sonos speakers on the network); see `make test-live`.
- `uitest/` also holds `spinners`, twenty-five candidates for the now-playing
  spinner shown side by side and animating. Nothing there is wired into the
  client: see `make uitest spinners`.
- `queueevents/` — watches ContentDirectory events to find out what a speaker
  reports when its queue changes, and prints which of the service's state
  variables the speaker says it events. Needs real speakers; see
  `make queue-events`.
- `notifycapture/` — subscribes itself to one event service and writes every
  raw NOTIFY body to a file. Its own subscription rather than EventManager's:
  the point is a body nothing can parse yet, and EventManager only hands out
  events it already understands. Needs real speakers; see
  `make notify-capture`.
- `discoverprobe/` — looks for speakers for 10 seconds by every means at once:
  the client's own `sonos.Discover`, SSDP on every IPv4 interface (the search
  sent again each second), and mDNS on each interface. Logs every answer, reads
  each speaker's description, and ends with how long each means took to find
  each speaker. For a machine that finds too few speakers. Console output only;
  see `make discover-probe` (`PROBE_ARGS=-for=20s` changes the time).
- `coverart/` — evaluates `github.com/blacktop/go-termimg` and
  `github.com/BourgeoisBear/rasterm` for rendering album art in the terminal.
  Runs against fixture images in `data/`, no live speaker calls.

## The module boundary

This tree is a Go module of its own. Its `go.mod` keeps the libraries the
evaluations need out of the one the client uses, which is the whole reason it
exists: `rasterm`, `go-termimg`, `nfnt/resize` and `koron/go-ssdp` have no
reader in the client and now have no entry in its `go.mod` either.

The module reads the parent's `internal/` packages. Go allows this because the
rule is about the import path, not about the module, and both sit under
`github.com/haasted/sonos-sandbox/`. A `require` and a `replace ../` carry it,
rather than a `go.work`: the tree then builds for anyone who runs it directly.

**`./...` from the repository root does not reach this tree.** A module boundary
stops the pattern, and a workspace does not change that. So every Makefile
target names this directory as well, through the `MODULES` variable. A target
that forgets it goes green while running none of these tests.

## What is here

`uitest` is the playground for look and feel — a harness of disposable demos
that draw the real components against mock data. It lives here rather than
beside the client because it is not the client, and because `termenv` has no
reader outside it.

`coverart` and `ssdp` are library evaluations, run against the images in
`data/` and against real speakers respectively.

## Running them

    make uitest                   list the demos
    make uitest reveal            run one
    make test                     from the root, which covers this tree too
    make test-live                the SSDP evaluation, which needs real speakers

Or directly:

    cd experiments && go test ./...
    cd experiments && go run ./uitest reveal

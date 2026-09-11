# experiments

Framework/library evaluations, kept as a record of what was tried and why it
was or wasn't adopted. Nothing here is imported by production code, and
production code never imports from here.

- `ssdp/` — evaluates `github.com/koron/go-ssdp` against the hand-rolled SSDP
  search in `internal/sonos/discovery.go`. Gated behind `-live` (needs real
  Sonos speakers on the network); see `make test-live`.
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

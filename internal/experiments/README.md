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

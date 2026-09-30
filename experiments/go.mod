module github.com/haasted/sonos-sandbox/experiments

go 1.26.3

require (
	charm.land/bubbles/v2 v2.2.1
	charm.land/bubbletea/v2 v2.0.9
	charm.land/lipgloss/v2 v2.0.6
	github.com/BourgeoisBear/rasterm v1.1.2
	github.com/blacktop/go-termimg v0.1.26
	github.com/haasted/sonos-sandbox v0.0.0
	github.com/hashicorp/mdns v1.0.7
	github.com/koron/go-ssdp v0.9.1
	github.com/lucasb-eyer/go-colorful v1.4.1
	github.com/muesli/termenv v0.16.0
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20260811164956-006e29f97886 // indirect
	github.com/charmbracelet/x/ansi v0.11.8 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.15 // indirect
	github.com/charmbracelet/x/mosaic v0.0.0-20260216111343-536eb63c1f4c // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/makeworld-the-better-one/dither/v2 v2.4.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.27 // indirect
	github.com/mattn/go-sixel v0.0.8 // indirect
	github.com/miekg/dns v1.1.72 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/soniakeys/quant v1.0.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/image v0.36.0 // indirect
	golang.org/x/mod v0.31.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/term v0.44.0 // indirect
	golang.org/x/tools v0.40.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// The evaluations read the parent's internal packages. A require with a replace
// rather than a go.work: the child then builds for anyone who runs it directly,
// without a workspace file having to exist.
replace github.com/haasted/sonos-sandbox => ../

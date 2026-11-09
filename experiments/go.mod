module github.com/haasted/sonos-sandbox/experiments

go 1.26.3

require (
	charm.land/bubbles/v2 v2.2.1
	charm.land/bubbletea/v2 v2.1.1
	charm.land/lipgloss/v2 v2.0.6
	github.com/BourgeoisBear/rasterm v1.1.2
	github.com/blacktop/go-termimg v0.1.26
	github.com/coder/websocket v1.8.15
	github.com/gocolly/colly/v2 v2.3.0
	github.com/haasted/sonos-sandbox v0.0.0
	github.com/hashicorp/mdns v1.0.7
	github.com/koron/go-ssdp v0.9.1
	github.com/lucasb-eyer/go-colorful v1.4.1
	github.com/muesli/termenv v0.16.0
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/stretchr/testify v1.12.1
	golang.org/x/term v0.47.0
)

require (
	github.com/PuerkitoBio/goquery v1.11.0 // indirect
	github.com/andybalholm/cascadia v1.3.3 // indirect
	github.com/antchfx/htmlquery v1.3.5 // indirect
	github.com/antchfx/xmlquery v1.5.0 // indirect
	github.com/antchfx/xpath v1.3.5 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/bits-and-blooms/bitset v1.24.6 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20261008173134-6b8d4baf91b4 // indirect
	github.com/charmbracelet/x/ansi v0.11.9 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.15 // indirect
	github.com/charmbracelet/x/mosaic v0.0.0-20260216111343-536eb63c1f4c // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kennygrant/sanitize v1.2.4 // indirect
	github.com/makeworld-the-better-one/dither/v2 v2.4.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.30 // indirect
	github.com/mattn/go-sixel v0.0.8 // indirect
	github.com/miekg/dns v1.1.72 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/nlnwa/whatwg-url v0.6.2 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/saintfish/chardet v0.0.0-20230101081208-5e3ef4b5456d // indirect
	github.com/soniakeys/quant v1.0.0 // indirect
	github.com/temoto/robotstxt v1.1.2 // indirect
	github.com/xo/terminfo v1.0.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/image v0.36.0 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	modernc.org/libc v1.79.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.61.1 // indirect
)

// The evaluations read the parent's internal packages. A require with a replace
// rather than a go.work: the child then builds for anyone who runs it directly,
// without a workspace file having to exist.
replace github.com/haasted/sonos-sandbox => ../

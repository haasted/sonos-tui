// Command uitest is a scratch harness for iterating on terminal UI layouts,
// independent of the real Sonos CLI. The first argument picks which layout
// to render, so new layouts can be added and compared side by side.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
)

// layout is one terminal UI variant, and a line saying what it shows. A name
// alone does not tell a reader what "effects2" or "interface1" holds.
type layout struct {
	about string
	run   func()
}

// layouts are the UI variants selectable by the first argument.
var layouts = map[string]layout{
	"blank":       {"a placeholder to start a new iteration from", layoutBlank},
	"breathe":     {"the breathing light, against a plain sine", layoutBreathe},
	"tabs":        {"the tab bar, with its divider and animations", layoutTabs},
	"tabs-bottom": {"the same tab bar along the bottom edge", layoutTabsBottom},
	"volume":      {"volume readouts, in every style and fade", layoutVolume},
	"effects":     {"lipgloss text effects: blink, pulse, shake, wave", layoutEffects},
	"effects2":    {"the vendored graphics core: reveal, glow, sprites", layoutEffects2},
	"interface1":  {"a now-playing panel for one room", layoutInterface1},
	"interface2":  {"an overview of every room at once", layoutInterface2},
	"interface3":  {"a now-playing panel with progress", layoutInterface3},
	"reveal":      {"the centre-out notice entrance, in four styles", layoutNoticeReveal},
	"spinners":    {"candidates for the now-playing spinner, side by side", layoutSpinners},
}

func main() {
	flag.Parse() // in debug builds, registers -ui-size (see uisize_debug.go); a no-op flag set otherwise

	if flag.NArg() < 1 {
		usage()
		os.Exit(1)
	}

	name := flag.Arg(0)
	l, ok := layouts[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown layout %q\n\n", name)
		usage()
		os.Exit(1)
	}

	l.run()
}

func usage() {
	names := make([]string, 0, len(layouts))
	width := 0
	for name := range layouts {
		names = append(names, name)
		width = max(width, len(name))
	}
	sort.Strings(names)

	fmt.Fprintln(os.Stderr, "usage: uitest [flags] <layout>")
	fmt.Fprintln(os.Stderr, "available layouts:")
	for _, name := range names {
		fmt.Fprintf(os.Stderr, "  %-*s  %s\n", width, name, layouts[name].about)
	}
}

// layoutBlank is a placeholder — swap in real layout iterations here.
func layoutBlank() {
	fmt.Println("blank layout — replace with real UI iteration")
}

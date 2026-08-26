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

// layout renders one terminal UI variant to stdout.
type layout func()

// layouts are the named UI variants selectable via the first CLI argument.
var layouts = map[string]layout{
	"blank":       layoutBlank,
	"tabs":        layoutTabs,
	"tabs-bottom": layoutTabsBottom,
	"effects":     layoutEffects,
	"interface1":  layoutInterface1,
	"interface2":  layoutInterface2,
	"interface3":  layoutInterface3,
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

	l()
}

func usage() {
	names := make([]string, 0, len(layouts))
	for name := range layouts {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Fprintln(os.Stderr, "usage: uitest [flags] <layout>")
	fmt.Fprintln(os.Stderr, "available layouts:")
	for _, name := range names {
		fmt.Fprintf(os.Stderr, "  %s\n", name)
	}
}

// layoutBlank is a placeholder — swap in real layout iterations here.
func layoutBlank() {
	fmt.Println("blank layout — replace with real UI iteration")
}

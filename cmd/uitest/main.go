// Command uitest is a scratch harness for iterating on terminal UI layouts,
// independent of the real Sonos CLI. The first argument picks which layout
// to render, so new layouts can be added and compared side by side.
package main

import (
	"fmt"
	"os"
	"sort"
)

// layout renders one terminal UI variant to stdout.
type layout func()

// layouts are the named UI variants selectable via the first CLI argument.
var layouts = map[string]layout{
	"blank":     layoutBlank,
	"bubbletea": layoutBubbletea,
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	name := os.Args[1]
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

	fmt.Fprintln(os.Stderr, "usage: uitest <layout>")
	fmt.Fprintln(os.Stderr, "available layouts:")
	for _, name := range names {
		fmt.Fprintf(os.Stderr, "  %s\n", name)
	}
}

// layoutBlank is a placeholder — swap in real layout iterations here.
func layoutBlank() {
	fmt.Println("blank layout — replace with real UI iteration")
}

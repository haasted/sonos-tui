//go:build debug

package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// uiSizeFlag lets `-ui-size WIDTH,HEIGHT` force the simulated terminal size
// for a layout, overriding real terminal resizes — for debugging layout/
// overflow behavior without needing to physically resize a terminal. Only
// registered in debug builds (`go build -tags debug`), so it doesn't exist
// at all in a release binary.
var uiSizeFlag = flag.String("ui-size", "", "debug: force UI width,height (e.g. 40,50), ignoring real terminal resizes")

// debugUISize parses -ui-size, if given. ok is false when the flag was left
// empty or isn't a valid "width,height" pair.
func debugUISize() (width, height int, ok bool) {
	if *uiSizeFlag == "" {
		return 0, 0, false
	}

	parts := strings.SplitN(*uiSizeFlag, ",", 2)
	if len(parts) != 2 {
		fmt.Fprintf(os.Stderr, "warning: -ui-size %q: want WIDTH,HEIGHT, e.g. 40,50 — ignoring\n", *uiSizeFlag)
		return 0, 0, false
	}

	w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		fmt.Fprintf(os.Stderr, "warning: -ui-size %q: want two positive integers — ignoring\n", *uiSizeFlag)
		return 0, 0, false
	}

	return w, h, true
}

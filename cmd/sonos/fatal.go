// How the program says why it cannot start or go on, for the reader and not
// only in the log, which is empty unless -logs is set.
package main

import (
	"errors"
	"fmt"
	"io"
)

// fatal is a reason the program stops: what failed, in words for the reader,
// and the error behind it, which says why in the system's own words.
type fatal struct {
	what string
	err  error
}

var _ error = fatal{}

func (f fatal) Error() string { return f.what + ": " + f.err.Error() }

func (f fatal) Unwrap() error { return f.err }

// report prints why the program stopped: what failed after the program's
// name, then the cause on a line of its own. Any other error is a sentence
// already, such as the terminal being too small, and is printed as it is.
func report(w io.Writer, err error) {
	var f fatal
	if errors.As(err, &f) {
		fmt.Fprintf(w, "sonos: %s\n  %v\n", f.what, f.err)
		return
	}
	fmt.Fprintln(w, err)
}

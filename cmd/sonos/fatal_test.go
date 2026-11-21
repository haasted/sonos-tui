package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Case: a fatal error, also when wrapped, and a plain one. A fatal error
// gives what failed after the program's name and the cause on its own line;
// a plain error is printed as it is.
func TestReport_SaysWhatFailedThenWhy(t *testing.T) {
	cause := errors.New("listen udp :0: bind: address already in use")
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"fatal":   {fatal{"cannot listen for events from the speakers", cause}, "sonos: cannot listen for events from the speakers\n  listen udp :0: bind: address already in use\n"},
		"wrapped": {fmt.Errorf("starting: %w", fatal{"the screen stopped working", cause}), "sonos: the screen stopped working\n  listen udp :0: bind: address already in use\n"},
		"plain":   {errors.New("sonos needs a terminal of at least 60×12 (this one is 40×10)"), "sonos needs a terminal of at least 60×12 (this one is 40×10)\n"},
	} {
		var out strings.Builder
		report(&out, tc.err)
		assert.Equal(t, tc.want, out.String(), name)
	}
}

// Case: a fatal error is still an error, with the cause behind it.
func TestFatal_KeepsTheCause(t *testing.T) {
	cause := errors.New("disk full")
	err := fatal{"cannot open the database at /x", cause}

	assert.ErrorIs(t, err, cause)
	assert.Equal(t, "cannot open the database at /x: disk full", err.Error())
}

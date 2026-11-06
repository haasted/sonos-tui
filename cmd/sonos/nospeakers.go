// What the program says when discovery ends with no speaker on screen. It
// tells a search that never left this computer from one nobody answered,
// and does not claim more than that.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

// noSpeakersLogHint ends every such text: how to get a log worth sharing.
// The log is printed when the program exits.
const noSpeakersLogHint = "For a log to share, run again with: sonos -logs"

// noSpeakersText is the screen shown when discovery found no speaker. Every
// line fits the narrowest terminal the program runs in, and all of them fit
// the player's rows, so nothing wraps or is cut.
func noSpeakersText(done discoveryDoneMsg) string {
	var lines []string
	switch {
	case errors.Is(done.err, sonos.ErrNoNetwork):
		lines = []string{
			"No speakers found: no network to search on.",
			"No interface is up with an IPv4 address.",
			"Check that this computer is on the network.",
		}
	case done.err != nil:
		lines = []string{
			"No speakers found: the search could not start.",
			"Error: " + errorText(done.err),
		}
	case done.report.Blocked():
		lines = []string{
			"No speakers found: the search could not be sent.",
			"Error: " + errorText(done.report.Unsent[0]) + ", on " + strings.Join(done.report.Interfaces, ", "),
			"Likely a VPN, or macOS Local Network access is off:",
			"Privacy & Security › Local Network. go run asks",
			"again for every new build.",
		}
	default:
		lines = []string{
			fmt.Sprintf("No Sonos speaker answered in %v.", done.took.Round(time.Second)),
			"Searched with SSDP and mDNS on " + strings.Join(done.report.Interfaces, ", ") + ".",
			"Check that the speakers are on, and on the same",
			"network as this computer.",
		}
	}

	// A blank line sets the hint apart, where the rows allow one.
	if len(lines) < playerHeightRows-1 {
		lines = append(lines, "")
	}
	lines = append(lines, noSpeakersLogHint)
	for i, line := range lines {
		lines[i] = clip(line, minTerminalWidth)
	}
	return strings.Join(lines, "\n")
}

// errorText is the operating system's own words for err, such as "no route
// to host", without the addresses the network layers wrap around them.
func errorText(err error) string {
	var syscall *os.SyscallError
	if errors.As(err, &syscall) {
		return syscall.Err.Error()
	}
	return err.Error()
}

package main

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/haasted/sonos-tui/internal/sonos"
)

// Case: every search failed to leave this computer, as behind a VPN. The
// screen says the search could not be sent, quotes the system's words, and
// names a VPN and the macOS permission as what to check.
func TestNoSpeakers_ABlockedSearchSaysSo(t *testing.T) {
	text := noSpeakersText(blockedDiscovery())

	assert.Contains(t, text, "could not be sent")
	assert.Contains(t, text, "Error: no route to host, on en0")
	assert.Contains(t, text, "VPN")
	assert.Contains(t, text, "Local Network")
	assert.NotContains(t, text, "answered", "a search that never left was said to go unanswered")
}

// Case: the searches went out and nobody answered. The screen says how long
// it looked and where, and what to check; it blames no VPN, which it cannot
// know about.
func TestNoSpeakers_AnUnansweredSearchSaysHowLongItLooked(t *testing.T) {
	text := noSpeakersText(discoveryDoneMsg{
		report: sonos.DiscoveryReport{Interfaces: []string{"en0"}, Searches: 2},
		took:   5*time.Second + 3*time.Millisecond,
	})

	assert.Contains(t, text, "No Sonos speaker answered in 5s.")
	assert.Contains(t, text, "on en0.")
	assert.Contains(t, text, "speakers are on")
	assert.NotContains(t, text, "VPN")
}

// Case: some searches failed and others went out. One search sent is enough
// for a speaker to have answered, so this is an unanswered search.
func TestNoSpeakers_OneSearchSentIsNotBlocked(t *testing.T) {
	text := noSpeakersText(discoveryDoneMsg{report: sonos.DiscoveryReport{
		Interfaces: []string{"en0"}, Searches: 2, Unsent: []error{hostUnreachable()},
	}})

	assert.Contains(t, text, "answered")
	assert.NotContains(t, text, "could not be sent")
}

// Case: no interface to search on at all.
func TestNoSpeakers_NoNetworkSaysSo(t *testing.T) {
	assert.Contains(t, noSpeakersText(discoveryDoneMsg{err: sonos.ErrNoNetwork}), "no network to search on")
}

// Case: every variant. Each ends with how to get a log, fits the player's
// rows, and has no line wider than the narrowest terminal the program allows,
// so nothing wraps into a row it does not have.
func TestNoSpeakers_EveryTextFitsAndEndsWithTheLogHint(t *testing.T) {
	long := discoveryDoneMsg{report: sonos.DiscoveryReport{
		Interfaces: []string{"en0", "en5", "bridge100", "utun7"}, Searches: 2,
		Unsent: []error{errors.New(strings.Repeat("an error far longer than any line ", 3)), hostUnreachable()},
	}}

	for name, done := range map[string]discoveryDoneMsg{
		"blocked":    blockedDiscovery(),
		"long":       long,
		"unanswered": {report: sonos.DiscoveryReport{Interfaces: []string{"en0"}, Searches: 2}},
		"no network": {err: sonos.ErrNoNetwork},
		"no start":   {err: errors.New("listing network interfaces: permission denied")},
	} {
		lines := strings.Split(noSpeakersText(done), "\n")
		assert.LessOrEqual(t, len(lines), playerHeightRows, name)
		assert.Equal(t, noSpeakersLogHint, lines[len(lines)-1], name)
		for _, line := range lines {
			assert.LessOrEqual(t, len([]rune(line)), minTerminalWidth, "%s: %q", name, line)
		}
	}
}

// Case: a run that found speakers, or filled its tabs from the cache. It
// shows the speakers and none of this.
func TestNoSpeakers_SpeakersOnScreenSayNothing(t *testing.T) {
	m := newAppModel(context.Background(), nil, cachedSpeakers(), "")
	model, _ := m.Update(blockedDiscovery())

	assert.NotContains(t, model.(appModel).View().Content, "No speakers found")
}

// blockedDiscovery is a run whose every search failed to send, as the VPN
// machine's did: "sendto: no route to host" on SSDP and on mDNS.
func blockedDiscovery() discoveryDoneMsg {
	return discoveryDoneMsg{
		report: sonos.DiscoveryReport{
			Interfaces: []string{"en0"}, Searches: 2,
			Unsent: []error{hostUnreachable(), hostUnreachable()},
		},
		took: 5 * time.Second,
	}
}

// hostUnreachable is the error a blocked send returns, wrapped as the
// network layers wrap it.
func hostUnreachable() error {
	return &net.OpError{Op: "write", Net: "udp4",
		Err: &os.SyscallError{Syscall: "sendto", Err: syscall.EHOSTUNREACH}}
}

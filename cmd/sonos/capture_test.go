package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Case: the bare -capture form, which must not swallow the next argument and
// must fall back to the default directory.
func TestCapture_BareFlagUsesTheDefaultDirectory(t *testing.T) {
	var dir captureDir
	require.NoError(t, dir.Set("true"))

	assert.Equal(t, defaultCaptureDir, string(dir))
	assert.True(t, dir.IsBoolFlag(), "without this, -capture would consume the following argument")
}

// Case: -capture=<dir>, for a deliberate session filed somewhere of its own.
func TestCapture_FlagAcceptsADirectory(t *testing.T) {
	var dir captureDir
	require.NoError(t, dir.Set("/tmp/radio-session"))

	assert.Equal(t, "/tmp/radio-session", string(dir))
}

// Case: a routed event carrying the REAL service value, which is a
// subscription path rather than a bare name. Using it unaltered builds a
// nested path whose directories do not exist and every write fails — silently,
// if logs are off. The first version of this test invented a tidy service name
// and so never exercised the case that actually occurs.
func TestCapture_FilenameNamesTheSpeakerAndService(t *testing.T) {
	dir := t.TempDir()

	path, err := writeCapture(dir, sonos.RawEvent{
		Speaker: sonos.NewSpeaker("192.168.1.10", "", "uuid:A", sonos.DeviceDescription{}),
		Service: "/MediaRenderer/RenderingControl/Event",
		Body:    []byte("<propertyset/>"),
	})

	require.NoError(t, err)
	assert.Contains(t, filepath.Base(path), "192.168.1.10")
	assert.Contains(t, filepath.Base(path), "RenderingControl")
	assert.True(t, strings.HasSuffix(path, ".xml"))
}

// Case: the filename itself, for every service the app subscribes to. A name
// containing a path separator writes into a directory that does not exist, so
// this is the invariant that matters rather than any particular label.
func TestCapture_FilenameNeverContainsAPathSeparator(t *testing.T) {
	dir := t.TempDir()

	for _, service := range []string{
		"/MediaRenderer/AVTransport/Event",
		"/MediaRenderer/RenderingControl/Event",
		"", // unrouted
	} {
		path, err := writeCapture(dir, sonos.RawEvent{Service: service, Body: []byte("<x/>")})
		require.NoError(t, err, "service %q", service)

		name := filepath.Base(path)
		assert.NotContains(t, name, "/", "service %q leaked a separator into the filename", service)
		assert.Equal(t, filepath.Join(dir, name), path, "the file must land directly in the capture directory")
	}
}

// Case: an unrouted NOTIFY, which must still be captured — being sent
// something we did not subscribe to is itself worth the evidence.
func TestCapture_UnroutedEventStillGetsAReadableName(t *testing.T) {
	dir := t.TempDir()

	path, err := writeCapture(dir, sonos.RawEvent{Body: []byte("<propertyset/>")})

	require.NoError(t, err)
	assert.Contains(t, filepath.Base(path), "unrouted")
	assert.Contains(t, filepath.Base(path), "unknown")
}

// Case: the body itself, which must reach disk byte for byte. A capture that
// normalised or re-encoded anything would be useless as a fixture.
func TestCapture_WritesTheBodyUnchanged(t *testing.T) {
	dir := t.TempDir()
	body := []byte("<propertyset>&lt;Event&gt; ræve å ø</propertyset>")

	path, err := writeCapture(dir, sonos.RawEvent{Service: "AVTransport", Body: body})
	require.NoError(t, err)

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, body, written)
}

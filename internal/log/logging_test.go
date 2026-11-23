package log_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haasted/sonos-tui/internal/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ansiEscape = "\x1b["

// TestSetup_File_AppendsToExistingFile checks that a run adds to the file and keeps the earlier lines.
func TestSetup_File_AppendsToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	require.NoError(t, os.WriteFile(path, []byte("earlier run\n"), 0o600))

	logRun(t, log.Options{File: path, Level: slog.LevelDebug}, "first message")
	logRun(t, log.Options{File: path, Level: slog.LevelDebug}, "second message")

	text := readFile(t, path)
	assert.True(t, strings.HasPrefix(text, "earlier run\n"), "the earlier content must stay first")
	assert.Contains(t, text, "first message")
	assert.Contains(t, text, "second message")
	assert.Equal(t, 2, strings.Count(text, "=== sonos-tui started"), "each run must start with its own line")
	assert.Less(t, strings.Index(text, "first message"), strings.Index(text, "second message"))
}

// TestSetup_File_WithColor checks that tint's colour codes go into the file when colours are on.
func TestSetup_File_WithColor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")

	logRun(t, log.Options{File: path, FileColor: true, Level: slog.LevelDebug}, "coloured")

	assert.Contains(t, readFile(t, path), ansiEscape)
}

// TestSetup_File_WithoutColor checks that the file holds plain text when colours are off.
func TestSetup_File_WithoutColor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")

	logRun(t, log.Options{File: path, FileColor: false, Level: slog.LevelDebug}, "plain")

	text := readFile(t, path)
	assert.Contains(t, text, "plain")
	assert.NotContains(t, text, ansiEscape)
}

// TestSetup_File_WritesBeforeStop checks that a record is in the file before the program ends.
func TestSetup_File_WritesBeforeStop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	stop := setup(t, log.Options{File: path, Level: slog.LevelDebug})

	slog.Info("live message")

	assert.Contains(t, readFile(t, path), "live message")
	require.NoError(t, stop())
}

// TestSetup_File_NewFileIsPrivate checks that a new log file can be read by its owner only.
func TestSetup_File_NewFileIsPrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")

	logRun(t, log.Options{File: path, Level: slog.LevelDebug}, "private")

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// TestSetup_File_OpenFails checks that a file that cannot be opened gives an error.
func TestSetup_File_OpenFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "app.log")

	_, err := log.Setup(log.Options{File: path, Level: slog.LevelDebug})

	assert.Error(t, err)
}

// setup calls log.Setup and puts the earlier default logger back when the test ends.
func setup(t *testing.T, opts log.Options) func() error {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	stop, err := log.Setup(opts)
	require.NoError(t, err)
	return stop
}

// logRun is one program run that logs msg once and then stops.
func logRun(t *testing.T, opts log.Options, msg string) {
	t.Helper()
	stop := setup(t, opts)
	slog.Info(msg)
	require.NoError(t, stop())
}

// readFile returns the content of the file at path.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

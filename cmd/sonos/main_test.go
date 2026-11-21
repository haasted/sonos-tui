package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const childDBEnv = "SONOS_TEST_MAIN_DB" // set in the child process to the -db path it starts with

// TestMainReportsFailedStart checks that a database that cannot be opened
// gives a message on stderr and exit status 1. main runs in a child process,
// because it calls os.Exit.
func TestMainReportsFailedStart(t *testing.T) {
	if path := os.Getenv(childDBEnv); path != "" {
		os.Args = []string{os.Args[0], "-db", path}
		main()
		return
	}

	// A regular file where the database folder must be, so the folder cannot be made.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(blocker, "sonos.db")

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainReportsFailedStart$")
	cmd.Env = append(os.Environ(), childDBEnv+"="+dbPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("exit: got %v, want status 1; stderr: %q", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "cannot open the database at "+dbPath) {
		t.Errorf("stderr: got %q, want it to name the database path", stderr.String())
	}
}

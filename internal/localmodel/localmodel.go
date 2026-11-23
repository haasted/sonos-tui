// Package localmodel runs the language model that macOS keeps on the machine,
// through Apple's fm command. Nothing given to it leaves the machine.
package localmodel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

const (
	// fmPath is where macOS puts fm.
	fmPath = "/usr/bin/fm"

	// waitDelay is how long a stopped fm may take to give back its output
	// before it is killed.
	waitDelay = time.Second
)

// Why fm cannot be used. An *UnavailableError holds one of these.
var (
	ErrNotInstalled   = errors.New("localmodel: fm is not installed")
	ErrNotAvailable   = errors.New("localmodel: the model is not available")
	ErrTermsNotAgreed = errors.New("localmodel: the fm terms are not agreed")
)

// fm runs the fm command: the engine the classifiers share.
type fm struct {
	path string
}

// UnavailableError says why fm cannot be used, with fm's own words.
type UnavailableError struct {
	Reason error  // ErrNotInstalled, ErrNotAvailable or ErrTermsNotAgreed
	Said   string // what fm printed, if it ran
}

// RunError is an fm run that failed. It names the subcommand only, never
// the prompt or the instructions.
type RunError struct {
	Command string
	Code    int
	Stderr  string
}

// validator is a result type that checks itself after it is decoded.
type validator interface {
	Validate() error
}

var (
	_ error = &UnavailableError{}
	_ error = &RunError{}
)

// defaultFM is the fm macOS installs, which most classifiers use.
var defaultFM = fm{path: fmPath}

// Available is nil when fm can answer, and otherwise an *UnavailableError.
func Available(ctx context.Context) error { return defaultFM.available(ctx) }

func (f fm) available(ctx context.Context) error {
	said, err := f.run(ctx, "", "available")
	switch {
	case errors.Is(err, ErrNotInstalled):
		return &UnavailableError{Reason: ErrNotInstalled}
	case err != nil:
		return &UnavailableError{Reason: ErrNotAvailable, Said: errorText(err)}
	case !strings.Contains(string(said), "model available"):
		return &UnavailableError{Reason: ErrNotAvailable, Said: strings.TrimSpace(string(said))}
	}

	said, err = f.run(ctx, "", "license", "--status")
	if err != nil || !strings.HasPrefix(string(said), "Agreed") {
		return &UnavailableError{Reason: ErrTermsNotAgreed, Said: strings.TrimSpace(string(said) + errorText(err))}
	}
	return nil
}

// respond asks the model for an answer that fits schema, and decodes it
// into T. The schema needs fm's title and x-order (LOCAL-MODEL.md). The
// answer is greedy: the same input gives the same output.
func (f fm) respond[T any](ctx context.Context, instructions string, schema string, prompt string) (T, error) {
	var answer T
	out, err := f.run(ctx, prompt, "respond", "--no-stream", "--greedy",
		"--instructions="+instructions, "--schema="+schema)
	if err != nil {
		return answer, err
	}
	reply := strings.TrimSpace(string(out))
	if err := decode(out, &answer); err != nil {
		slog.Warn("localmodel: the answer does not fit expected type", "reply", reply, "err", err)
		return answer, fmt.Errorf("localmodel: the answer does not fit: %w", err)
	}
	slog.Debug("localmodel: answered", "reply", reply)
	return answer, nil
}

// decode reads an answer into answer, which must hold every field it has
// and pass its own Validate, if it has one.
func decode(out []byte, answer any) error {
	decoder := json.NewDecoder(bytes.NewReader(out))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(answer); err != nil {
		return err
	}
	if v, ok := answer.(validator); ok {
		return v.Validate()
	}
	return nil
}

// run runs fm with args and stdin, and gives back what it printed. The
// arguments go to fm directly, never through a shell.
func (f fm) run(ctx context.Context, stdin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, f.path, args...)
	cmd.WaitDelay = waitDelay
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	switch {
	case err == nil:
		return stdout.Bytes(), nil
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, exec.ErrNotFound):
		return nil, ErrNotInstalled
	case ctx.Err() != nil:
		return nil, fmt.Errorf("localmodel: fm %s: %w", args[0], ctx.Err())
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return nil, &RunError{Command: args[0], Code: exit.ExitCode(), Stderr: strings.TrimSpace(stderr.String())}
	}
	return nil, fmt.Errorf("localmodel: fm %s: %w", args[0], err)
}

// errorText is what a failed run said, or nothing.
func errorText(err error) string {
	if runErr, ok := errors.AsType[*RunError](err); ok {
		return runErr.Stderr
	}
	return ""
}

func (e *UnavailableError) Error() string {
	if e.Said == "" {
		return e.Reason.Error()
	}
	return e.Reason.Error() + ": " + e.Said
}

func (e *UnavailableError) Unwrap() error { return e.Reason }

func (e *RunError) Error() string {
	return fmt.Sprintf("localmodel: fm %s exited %d: %s", e.Command, e.Code, e.Stderr)
}

package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

const defaultCaptureDir = "captured-data"

// captureDir is where raw NOTIFY bodies are written, empty when not
// capturing. Accepts both "-capture" and "-capture=<dir>": a deliberate
// capture session wants its own directory, while the bare form
// keeps the default the retired cmd/listening used.
type captureDir string

var _ flag.Value = (*captureDir)(nil)

func (c captureDir) String() string { return string(c) }

func (c *captureDir) Set(value string) error {
	if value == "true" { // the bare -capture form
		value = defaultCaptureDir
	}
	*c = captureDir(value)
	return nil
}

// IsBoolFlag lets "-capture" stand alone without swallowing the next
// argument, while "-capture=<dir>" still passes a directory through.
func (c *captureDir) IsBoolFlag() bool { return true }

var capturePath captureDir

func init() {
	flag.Var(&capturePath, "capture",
		"write every raw NOTIFY body to its own file; optionally -capture=<dir> (default "+defaultCaptureDir+")")
}

// captureOption builds the event-manager option that writes raw bodies to
// disk, or nothing when -capture was not given.
func captureOption() []sonos.EventManagerOption {
	dir := string(capturePath)
	if dir == "" {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Error("creating capture directory — continuing without capture", "dir", dir, "err", err)
		return nil
	}

	absolute, err := filepath.Abs(dir)
	if err != nil {
		absolute = dir
	}
	slog.Info("capturing every raw event", "dir", absolute)

	return []sonos.EventManagerOption{sonos.WithCapture(func(raw sonos.RawEvent) {
		path, err := writeCapture(dir, raw)
		if err != nil {
			// Error, not Warn: a capture session that silently writes nothing
			// wastes the whole session, and -logs=false hides warnings.
			slog.Error("capturing event failed", "dir", dir, "err", err)
			return
		}
		slog.Debug("captured event", "path", path, "speaker", raw.Speaker.IP, "service", raw.Service)
	})}
}

// writeCapture files one body under a name that sorts chronologically and
// says which speaker and service it came from.
func writeCapture(dir string, raw sonos.RawEvent) (string, error) {
	name := fmt.Sprintf("%s-%s-%s.xml",
		time.Now().Format("20060102-150405.000000000"),
		captureField(raw.Speaker.IP, "unrouted"),
		captureField(serviceLabel(raw.Service), "unknown"))

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, raw.Body, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// serviceLabel turns an event path into something that can go in a filename.
// RawEvent.Service is the subscription path — "/MediaRenderer/AVTransport/Event"
// — so using it raw builds a nested path whose directories do not exist, and
// every write fails.
func serviceLabel(service string) string {
	trimmed := strings.TrimSuffix(strings.Trim(service, "/"), "/Event")
	if index := strings.LastIndex(trimmed, "/"); index >= 0 {
		trimmed = trimmed[index+1:]
	}
	return strings.Map(func(r rune) rune {
		if r == os.PathSeparator || r == '\\' {
			return '-'
		}
		return r
	}, trimmed)
}

// captureField keeps a filename readable when the event was never routed and
// its speaker and service are therefore unknown.
func captureField(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

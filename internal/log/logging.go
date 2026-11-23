package log

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

const (
	timeFormat     = "15:04:05.000"
	startFormat    = "2006-01-02 15:04:05 MST"
	DefaultLogFile = "sonos-tui.log" // file name in the start directory when no path is given
)

// Options says where the log goes. With no destination, the log is discarded.
type Options struct {
	Capture   bool         // keep the log in memory and print it to stdout at exit
	File      string       // path of a file to append to as records come; empty for none
	FileColor bool         // keep tint's colour codes in the file
	Level     slog.Leveler // lowest level that is kept
}

// Setup sets the default logger from opts. It returns a function that prints
// the captured log and closes the file; call it when the program ends.
func Setup(opts Options) (func() error, error) {
	start := time.Now()
	var handlers []slog.Handler
	var closers []func() error

	if opts.File != "" {
		f, err := os.OpenFile(opts.File, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
		if err != nil {
			return nil, err
		}
		if _, err := fmt.Fprintf(f, "=== sonos-tui started %s ===\n", start.Format(startFormat)); err != nil {
			f.Close()
			return nil, err
		}
		handlers = append(handlers, newTextHandler(f, opts.Level, !opts.FileColor))
		closers = append(closers, f.Close)
	}

	if opts.Capture {
		buf := NewBuffer()
		handlers = append(handlers, newTextHandler(buf, opts.Level, false))
		closers = append(closers, func() error { return buf.Flush(os.Stdout) })
	}

	if len(handlers) == 0 {
		slog.SetDefault(slog.New(slog.DiscardHandler))
		return func() error { return nil }, nil
	}

	slog.SetDefault(slog.New(NewElapsedHandler(slog.NewMultiHandler(handlers...), start)))
	return func() error {
		var errs []error
		for _, c := range closers {
			errs = append(errs, c())
		}
		return errors.Join(errs...)
	}, nil
}

// newTextHandler makes the tint handler that every log destination uses.
func newTextHandler(w io.Writer, level slog.Leveler, noColor bool) slog.Handler {
	return tint.NewTextHandler(w, &tint.Options{
		TimeFormat: timeFormat,
		Level:      level,
		NoColor:    noColor,
	})
}

package log

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

func CaptureLogs(enable bool) func() {
	if !enable {
		slog.SetDefault(slog.New(slog.DiscardHandler))
		return func() {}
	}

	buf := NewBuffer()
	slog.SetDefault(slog.New(NewElapsedHandler(
		tint.NewTextHandler(buf, &tint.Options{TimeFormat: time.TimeOnly}),
		time.Now(),
	)))

	return func() {
		if err := buf.Flush(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "flushing logs:", err)
		}
	}
}

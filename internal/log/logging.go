package log

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

const timeFormat = "15:04:05.000"

func CaptureLogs(enable bool, level slog.Leveler) func() {
	if !enable {
		slog.SetDefault(slog.New(slog.DiscardHandler))
		return func() {}
	}

	buf := NewBuffer()
	slog.SetDefault(slog.New(NewElapsedHandler(
		tint.NewTextHandler(buf, &tint.Options{
			TimeFormat: timeFormat,
			Level:      level,
		}),
		time.Now(),
	)))

	return func() {
		if err := buf.Flush(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "flushing logs:", err)
		}
	}
}

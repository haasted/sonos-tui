package log

import (
	"context"
	"log/slog"
	"time"
)

type ElapsedHandler struct {
	slog.Handler
	start time.Time
}

func NewElapsedHandler(h slog.Handler, start time.Time) *ElapsedHandler {
	return &ElapsedHandler{Handler: h, start: start}
}

func (h *ElapsedHandler) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(slog.Duration("elapsed", time.Since(h.start).Round(time.Microsecond)))
	return h.Handler.Handle(ctx, r)
}

func (h *ElapsedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ElapsedHandler{Handler: h.Handler.WithAttrs(attrs), start: h.start}
}

func (h *ElapsedHandler) WithGroup(name string) slog.Handler {
	return &ElapsedHandler{Handler: h.Handler.WithGroup(name), start: h.start}
}

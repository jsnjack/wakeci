package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
)

// LevelTrace is the most verbose logging level, surfaced only via --trace.
const LevelTrace = slog.Level(-8)

// L is the application-wide logger. initLogger rewires it from the
// --debug/--trace flags; it defaults to discarding output so code paths that
// run before that (tests, in particular) never see a nil logger.
var L = slog.New(slog.NewTextHandler(io.Discard, nil))

// fanoutHandler dispatches a record to every handler that accepts its level.
// It lets --debug (stderr) and --trace (file) be enabled independently or
// together, each at its own level.
type fanoutHandler struct {
	handlers []slog.Handler
}

func (f *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (f *fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, h := range f.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &fanoutHandler{handlers: next}
}

func (f *fanoutHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithGroup(name)
	}
	return &fanoutHandler{handlers: next}
}

// initLogger wires the global logger L from the --debug/--trace flags.
// tracePath is non-empty only when --trace is set, in which case every
// record is additionally written to that file (truncated on start),
// independently of the stderr level. It returns a cleanup func that closes
// the trace file; callers must invoke it before every process exit path,
// since os.Exit skips deferred calls.
func initLogger(tracePath, level string) func() {
	stderrLevel := slog.LevelWarn
	if level == "debug" {
		stderrLevel = slog.LevelDebug
	}
	handlers := []slog.Handler{
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: stderrLevel}),
	}

	cleanup := func() {}
	if tracePath != "" {
		f, err := os.OpenFile(tracePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err == nil {
			cleanup = func() {
				if err := f.Close(); err != nil {
					L.Error("close trace log", "path", tracePath, "err", err)
				}
			}
			handlers = append(handlers, slog.NewTextHandler(f, &slog.HandlerOptions{Level: LevelTrace}))
		}
	}

	L = slog.New(&fanoutHandler{handlers: handlers})
	slog.SetDefault(L)
	return cleanup
}

// writeBody writes data to an HTTP response, logging failures at trace
// level. A failed write here almost always means the client already
// disconnected, so there is nothing more actionable to do.
func writeBody(logger *slog.Logger, w http.ResponseWriter, data []byte) {
	if _, err := w.Write(data); err != nil {
		logger.Log(context.Background(), LevelTrace, "write response body", "err", err)
	}
}

// writeText writes a plain-text response with status
func writeText(w http.ResponseWriter, logger *slog.Logger, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	writeBody(logger, w, []byte(msg))
}

// writeJSON writes v as a JSON response
func writeJSON(w http.ResponseWriter, logger *slog.Logger, v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		logger.Error("marshal response", "err", err)
		writeText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeBody(logger, w, payload)
}

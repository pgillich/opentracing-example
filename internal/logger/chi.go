package logger

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"emperror.dev/errors"
	"github.com/go-chi/chi/v5/middleware"
)

type ChiLogr struct {
	*slog.Logger
}

type ChiLogrEntry struct {
	*slog.Logger
}

func (e ChiLogrEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra interface{}) {
	if extra == nil {
		extra = "Chi"
	}
	e.With(
		"status", status,
		"bytes", bytes,
		"elapsed", elapsed,
	).Info(fmt.Sprintf("%+v", extra))
}

func (e *ChiLogrEntry) Panic(v interface{}, stack []byte) {
	e.With(
		"panic", v,
	).Error("PANIC", "error", errors.NewPlain("chi panic"))
}

func (l *ChiLogr) NewLogEntry(r *http.Request) middleware.LogEntry {
	reqID := middleware.GetReqID(r.Context())

	return &ChiLogrEntry{l.With(
		"reqID", reqID,
		"method", r.Method,
		"URL", r.URL.String(),
		"RemoteAddr", r.RemoteAddr,
	)}
}

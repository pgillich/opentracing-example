package logger

import (
	"fmt"
	"net/http"
	"time"

	"emperror.dev/errors"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
)

type ChiLogr struct {
	logr.Logger
}

type ChiLogrEntry struct {
	logr.Logger
}

func (e ChiLogrEntry) Write(status, bytes int, header http.Header, elapsed time.Duration, extra interface{}) {
	if extra == nil {
		extra = "Chi"
	}
	e.WithValues(
		"status", status,
		"bytes", bytes,
		"elapsed", elapsed,
	).Info(fmt.Sprintf("%+v", extra))
}

func (e *ChiLogrEntry) Panic(v interface{}, stack []byte) {
	e.WithValues(
		"panic", v,
	).Error(errors.NewPlain("chi panic"), "PANIC")
}

func (l *ChiLogr) NewLogEntry(r *http.Request) middleware.LogEntry {
	reqID := middleware.GetReqID(r.Context())

	return &ChiLogrEntry{l.WithValues(
		"reqID", reqID,
		"method", r.Method,
		"URL", r.URL.String(),
		"RemoteAddr", r.RemoteAddr,
	)}
}

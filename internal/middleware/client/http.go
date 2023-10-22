package client

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/pgillich/opentracing-example/internal/logger"
	"go.opentelemetry.io/otel/trace"
)

// Transport implements the http.RoundTripper interface and wraps
// outbound HTTP(S) requests with logs.
type Transport struct {
	rt http.RoundTripper

	logger     *slog.Logger
	beginLevel slog.Level
	endLevel   slog.Level
}

// NewTransport wraps the provided http.RoundTripper with one that
// logs request and respnse.
//
// If the provided http.RoundTripper is nil, http.DefaultTransport will be used
// as the base http.RoundTripper.
func NewTransport(base http.RoundTripper, beginLevel slog.Level, endLevel slog.Level, log *slog.Logger) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}

	return &Transport{
		rt:         base,
		logger:     log,
		beginLevel: beginLevel,
		endLevel:   endLevel,
	}
}

// RoundTrip logs outgoing request and response.
func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx := r.Context()
	ctx, log := logger.FromContext(ctx,
		"outMethod", r.Method,
		"outUrl", r.URL.String(),
		"spanID", trace.SpanFromContext(ctx).SpanContext().SpanID(),
	)
	var res *http.Response
	var err error

	log.Log(ctx, t.beginLevel, "OUT_REQ")
	beginTS := time.Now()
	defer func() {
		elapsedSec := time.Since(beginTS).Seconds()
		args := []any{
			"outStatusCode", res.StatusCode,
			"outContentLength", res.ContentLength,
			"outDuration", fmt.Sprintf("%.3f", elapsedSec),
		}
		if err != nil {
			args = append(args, logger.KeyError, err)
		}
		log.With(args...).Log(ctx, t.endLevel, "OUT_RESP")
	}()

	r = r.WithContext(ctx)
	res, err = t.rt.RoundTrip(r)

	return res, err //nolint:wrapcheck // should not be changed
}

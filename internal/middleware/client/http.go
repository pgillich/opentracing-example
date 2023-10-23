package client

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	metric_api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/middleware"
)

// LogTransport implements the http.RoundTripper interface and wraps
// outbound HTTP(S) requests with logs.
type LogTransport struct {
	rt http.RoundTripper

	beginLevel slog.Level
	endLevel   slog.Level
}

// NewLogTransport wraps the provided http.RoundTripper with one that
// logs request and respnse.
//
// If the provided http.RoundTripper is nil, http.DefaultTransport will be used
// as the base http.RoundTripper.
func NewLogTransport(base http.RoundTripper, beginLevel slog.Level, endLevel slog.Level) *LogTransport {
	if base == nil {
		base = http.DefaultTransport
	}

	return &LogTransport{
		rt:         base,
		beginLevel: beginLevel,
		endLevel:   endLevel,
	}
}

// RoundTrip logs outgoing request and response.
func (t *LogTransport) RoundTrip(r *http.Request) (*http.Response, error) {
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

	r = r.WithContext(ctx)
	res, err = t.rt.RoundTrip(r)

	elapsedSec := time.Since(beginTS).Seconds()
	var statusCode int
	var contentLength int64
	if res != nil {
		statusCode = res.StatusCode
		contentLength = res.ContentLength
	}
	args := []any{
		"outStatusCode", statusCode,
		"outReqContentLength", r.ContentLength,
		"outRespContentLength", contentLength,
		"outDuration", fmt.Sprintf("%.3f", elapsedSec),
	}
	if err != nil {
		args = append(args, logger.KeyError, err)
	}
	log.With(args...).Log(ctx, t.endLevel, "OUT_RESP")

	return res, err //nolint:wrapcheck // should not be changed
}

// MetricTransport implements the http.RoundTripper interface and wraps
// outbound HTTP(S) requests with metrics.
type MetricTransport struct {
	rt http.RoundTripper

	meter        metric_api.Meter
	name         string
	description  string
	baseAttrs    []attribute.KeyValue
	errFormatter middleware.ErrFormatter
}

// NewMetricTransport wraps the provided http.RoundTripper with one that
// meters metrics.
//
// If the provided http.RoundTripper is nil, http.DefaultTransport will be used
// as the base http.RoundTripper.
func NewMetricTransport(base http.RoundTripper, meter metric_api.Meter, name string,
	description string, attributes map[string]string, errFormatter middleware.ErrFormatter,
) *MetricTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	baseAttrs := make([]attribute.KeyValue, 0, len(attributes))
	for aKey, aVal := range attributes {
		baseAttrs = append(baseAttrs, attribute.Key(aKey).String(aVal))
	}

	return &MetricTransport{
		rt:           base,
		meter:        meter,
		name:         name,
		description:  description,
		baseAttrs:    baseAttrs,
		errFormatter: errFormatter,
	}
}

// RoundTrip meters outgoing request-response pair.
func (t *MetricTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx := r.Context()
	ctx, log := logger.FromContext(ctx)

	attempted, err := middleware.Int64CounterGetInstrument(t.name, metric_api.WithDescription(t.description))
	if err != nil {
		log.Error("unable to instantiate counter", logger.KeyError, err, "metricName", t.name)
		panic(err)
	}
	durationSum, err := middleware.Float64CounterGetInstrument(t.name+"_duration", metric_api.WithDescription(t.description+", duration sum"), metric_api.WithUnit("s"))
	if err != nil {
		log.Error("unable to instantiate time counter", logger.KeyError, err, "metricName", t.name)
		panic(err)
	}
	beginTS := time.Now()
	var res *http.Response

	r = r.WithContext(ctx)
	res, err = t.rt.RoundTrip(r)

	elapsedSec := time.Since(beginTS).Seconds()
	attrs := make([]attribute.KeyValue, len(t.baseAttrs), len(t.baseAttrs)+6)
	copy(attrs, t.baseAttrs)
	var statusCode int
	if res != nil {
		statusCode = res.StatusCode
	}
	host := middleware.GetHost(r)
	attrs = append(attrs,
		attribute.Key(middleware.MetrAttrMethod).String(r.Method),
		attribute.Key(middleware.MetrAttrUrl).String(r.URL.String()),
		attribute.Key(middleware.MetrAttrHost).String(host),
		attribute.Key(middleware.MetrAttrPath).String(r.URL.Path),
		attribute.Key(middleware.MetrAttrStatus).Int(statusCode),
		attribute.Key(middleware.MetrAttrErr).String(t.errFormatter(err)),
	)
	opt := metric_api.WithAttributes(attrs...)
	attempted.Add(ctx, 1, opt)
	durationSum.Add(ctx, elapsedSec, opt)

	return res, err //nolint:wrapcheck // should not be changed
}

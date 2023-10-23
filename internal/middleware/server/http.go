package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	metric_api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.11.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/middleware"
	"github.com/pgillich/opentracing-example/internal/tracing"
)

func ChiLoggerBaseMiddleware(log *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			log = log.With(
				"inMethod", r.Method,
				"inUrl", r.URL.String(),
			)
			ctx := logger.NewContext(r.Context(), log)

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}

func ChiTracerMiddleware(tr trace.Tracer, instance string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ctx, log := logger.FromContext(r.Context())
			routePath := getRoutePath(ctx, r)

			ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(r.Header))
			span := trace.SpanFromContext(ctx)
			clientCommand := ""
			if span.SpanContext().IsValid() {
				spanValues, spanValuesErr := span.SpanContext().MarshalJSON()
				log.With(
					"span", spanValues,
					"spanErr", spanValuesErr,
					"bag", baggage.FromContext(ctx).String(),
					"traceID", span.SpanContext().TraceID().String(),
					"spanID", span.SpanContext().SpanID().String(),
				).Debug("SPAN_IN")
				clientCommand = span.SpanContext().TraceState().Get(tracing.StateKeyClientCommand)
			} else {
				command := r.Method + " " + r.URL.String()

				traceState := trace.TraceState{}
				traceState, err := traceState.Insert(tracing.StateKeyClientCommand, tracing.EncodeTracestateValue(command))
				if err != nil {
					log.Error("unable to set command in state", logger.KeyError, err)
				}
				ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
					TraceState: traceState,
				}))
				bag, err := tracing.NewBaggage(instance, command)
				if err != nil {
					log.Error("unable to set command in baggage", logger.KeyError, err)
				}
				ctx = baggage.ContextWithBaggage(ctx, bag)

				span = trace.SpanFromContext(ctx)
				spanValues, spanValuesErr := span.SpanContext().MarshalJSON()
				log.With(
					"span", spanValues,
					"spanErr", spanValuesErr,
					"bag", bag,
				).Debug("SPAN_NEW")
			}
			ctx = logger.NewContext(ctx, log)

			spanParent := span
			spanKind := trace.SpanKindServer
			ctx, span = tr.Start(ctx, "IN HTTP "+r.Method+" "+r.URL.String(),
				trace.WithAttributes(semconv.NetAttributesFromHTTPRequest("tcp", r)...),
				trace.WithAttributes(semconv.HTTPClientAttributesFromHTTPRequest(r)...),
				trace.WithAttributes(semconv.HTTPServerAttributesFromHTTPRequest(instance, routePath, r)...),
				trace.WithSpanKind(spanKind),
				trace.WithAttributes(
					attribute.String(tracing.StateKeyClientCommand, clientCommand),
					attribute.String(tracing.SpanKeyComponent, tracing.SpanKeyComponentValue),
				),
			)
			spanLogValues := []interface{}{"traceID", span.SpanContext().TraceID().String()}
			if spanParent.SpanContext().IsValid() {
				spanLogValues = append(spanLogValues, "spanID", spanParent.SpanContext().SpanID().String())
			}
			spanLogValues = append(spanLogValues, "spanID", span.SpanContext().SpanID().String())
			ctx, log = logger.FromContext(ctx, spanLogValues...)
			log.With("spanKind", spanKind).Debug("SPAN_START")

			uk := attribute.Key("username") // from HTTP header
			span.AddEvent("IN req from user", trace.WithAttributes(append(append(
				semconv.HTTPServerAttributesFromHTTPRequest(instance, routePath, r),
				semconv.HTTPClientAttributesFromHTTPRequest(r)...),
				uk.String("testUser"),
			)...))

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}

func ChiLoggerMiddleware(beginLevel slog.Level, endLevel slog.Level) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ctx, log := logger.FromContext(r.Context())
			routePath := getRoutePath(ctx, r)
			lrw := NewLoggingResponseWriter(w)

			log.Log(ctx, beginLevel, "IN_HTTP")
			beginTS := time.Now()

			r = r.WithContext(ctx)
			next.ServeHTTP(lrw, r)

			elapsedSec := time.Since(beginTS).Seconds()
			log.With(
				"inPathPattern", routePath,
				"inStatusCode", lrw.statusCode,
				"inReqContentLength", r.ContentLength,
				"inRespContentLength", w.Header().Get("Content-Length"),
				"inDuration", fmt.Sprintf("%.3f", elapsedSec),
			).Log(ctx, endLevel, "IN_HTTP_RESP")
		}

		return http.HandlerFunc(fn)
	}
}

func ChiMetricMiddleware(meter metric_api.Meter, name string,
	description string, attributes map[string]string, log *slog.Logger,
) func(next http.Handler) http.Handler {
	baseAttrs := make([]attribute.KeyValue, 0, len(attributes))
	for aKey, aVal := range attributes {
		baseAttrs = append(baseAttrs, attribute.Key(aKey).String(aVal))
	}
	attempted, err := middleware.Int64CounterGetInstrument(name, metric_api.WithDescription(description))
	if err != nil {
		log.Error("unable to instantiate counter", logger.KeyError, err, "metricName", name)
		panic(err)
	}
	durationSum, err := middleware.Float64CounterGetInstrument(name+"_duration", metric_api.WithDescription(description+", duration sum"), metric_api.WithUnit("s"))
	if err != nil {
		log.Error("unable to instantiate time counter", logger.KeyError, err, "metricName", name)
		panic(err)
	}

	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ctx, _ := logger.FromContext(r.Context())
			routePath := getRoutePath(ctx, r)
			lrw := NewLoggingResponseWriter(w)

			beginTS := time.Now()

			next.ServeHTTP(lrw, r)

			elapsedSec := time.Since(beginTS).Seconds()
			attrs := make([]attribute.KeyValue, len(baseAttrs), len(baseAttrs)+6)
			copy(attrs, baseAttrs)
			host := middleware.GetHost(r)
			attrs = append(attrs,
				attribute.Key(middleware.MetrAttrMethod).String(r.Method),
				attribute.Key(middleware.MetrAttrUrl).String(r.URL.String()),
				attribute.Key(middleware.MetrAttrHost).String(host),
				attribute.Key(middleware.MetrAttrPath).String(r.URL.Path),
				attribute.Key(middleware.MetrAttrPathPattern).String(routePath),
				attribute.Key(middleware.MetrAttrStatus).Int(lrw.statusCode),
			)
			opt := metric_api.WithAttributes(attrs...)
			attempted.Add(ctx, 1, opt)
			durationSum.Add(ctx, elapsedSec, opt)
		}

		return http.HandlerFunc(fn)
	}
}

func getRoutePath(ctx context.Context, r *http.Request) string {
	routePath := chi.RouteContext(ctx).RoutePath
	if routePath == "" {
		if r.URL.RawPath != "" {
			routePath = r.URL.RawPath
		} else {
			routePath = r.URL.Path
		}
	}

	return routePath
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewLoggingResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{w, http.StatusOK}
}

func (lrw *loggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

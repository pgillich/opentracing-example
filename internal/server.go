package internal

import (
	"context"
	"net/http"

	"emperror.dev/errors"
	"github.com/go-logr/logr"
	"github.com/pgillich/opentracing-example/internal/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.11.0"
	"go.opentelemetry.io/otel/trace"
)

var ErrInvalidServerRunner = errors.NewPlain("invalid server runner")

func RunServer(h http.Handler, shutdown <-chan struct{}, addr string, log logr.Logger) {
	server := &http.Server{ // nolint:gosec // not secure
		Handler: h,
		Addr:    addr,
	}

	go func() {
		<-shutdown
		if err := server.Shutdown(context.Background()); !errors.Is(err, http.ErrServerClosed) {
			log.Error(err, "Server shutdown error")
		}
	}()

	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Error(err, "Server exit error")
	} else {
		log.Info("Server exit")
	}
}

func (s *Frontend) writeErr(w http.ResponseWriter, statusCode int, err error) {
	w.WriteHeader(statusCode)
	if _, err := w.Write([]byte(err.Error())); err != nil { //nolint:govet // err shadow
		s.log.Error(err, "unable to write response")
	}
}

func chiSpan(tp *sdktrace.TracerProvider, tracerName string, route string, instance string, r *http.Request, l logr.Logger) (context.Context, trace.Span) {
	tr := tp.Tracer(tracerName, trace.WithInstrumentationVersion(tracing.SemVersion()))
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		spanValues, spanValuesErr := span.SpanContext().MarshalJSON()
		l.WithValues(
			"span", spanValues,
			"spanErr", spanValuesErr,
			"bag", baggage.FromContext(ctx),
		).Info("IN Span")
	} else {
		command := r.Method + " " + r.URL.String()

		traceState := trace.TraceState{}
		traceState, err := traceState.Insert("state_command", tracing.EncodeTracestateValue(command))
		if err != nil {
			l.Error(err, "unable to set command in state")
		}
		ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
			TraceState: traceState,
		}))
		bag, err := tracing.NewBaggage(instance, command)
		if err != nil {
			l.Error(err, "unable to set command in baggage")
		}
		ctx = baggage.ContextWithBaggage(ctx, bag)

		spanValues, spanValuesErr := trace.SpanFromContext(ctx).SpanContext().MarshalJSON()
		l.WithValues(
			"span", spanValues,
			"spanErr", spanValuesErr,
			"bag", bag,
		).Info("NEW Span")
	}

	ctx, span = tr.Start(ctx, "IN HTTP",
		trace.WithAttributes(semconv.NetAttributesFromHTTPRequest("tcp", r)...),
		trace.WithAttributes(semconv.HTTPClientAttributesFromHTTPRequest(r)...),
		trace.WithAttributes(semconv.HTTPServerAttributesFromHTTPRequest(instance, route, r)...),
		trace.WithSpanKind(trace.SpanKindServer),
	)

	uk := attribute.Key("username") // from HTTP header
	span.AddEvent("IN req from user", trace.WithAttributes(append(append(
		semconv.HTTPServerAttributesFromHTTPRequest(instance, route, r),
		semconv.HTTPClientAttributesFromHTTPRequest(r)...),
		uk.String("testUser"),
	)...))

	return ctx, span
}

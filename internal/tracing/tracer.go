package tracing

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.11.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/pgillich/opentracing-example/internal/buildinfo"
)

type ErrorHandler struct {
	log *logr.Logger
}

func (e *ErrorHandler) Handle(err error) {
	e.log.Error(err, "OTEL ERROR")
}

var errorHandler = &ErrorHandler{}
var onceSetOtel sync.Once      //nolint:gochecknoglobals // local once
var onceBodySetOtel = func() { //nolint:gochecknoglobals // local once
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	otel.SetErrorHandler(errorHandler)
	otel.SetLogger(*errorHandler.log)
}

func SetErrorHandlerLogger(log *logr.Logger) {
	errorHandler.log = log
}

const (
	StateKeyClientCommand = "client_command"
	SpanKeyComponent      = "component"
	SpanKeyComponentValue = "opentracing-example"
)

func InitTracer(exporter sdktrace.SpanExporter, sampler sdktrace.Sampler, service string, instance string, command string, log logr.Logger) *sdktrace.TracerProvider {
	// For the demonstration, use sdktrace.AlwaysSample sampler to sample all traces.
	// In a production application, use sdktrace.ProbabilitySampler with a desired probability.
	// semconv keys are defined in https://github.com/open-telemetry/opentelemetry-specification/tree/main/semantic_conventions/trace
	attrs := []attribute.KeyValue{
		semconv.ServiceNamespaceKey.String("opentracing-example"),
		semconv.ServiceNameKey.String(service),
		semconv.ServiceInstanceIDKey.String(instance),
		semconv.ServiceVersionKey.String(buildinfo.Version),
		attribute.Int("attrID", os.Getpid()),
	}
	if command != "" {
		attrs = append(attrs, attribute.String(StateKeyClientCommand, command))
	}
	providerOptions := []sdktrace.TracerProviderOption{
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, attrs...)),
	}
	if exporter != nil {
		providerOptions = append(providerOptions, sdktrace.WithBatcher(exporter))
	}
	tp := sdktrace.NewTracerProvider(providerOptions...)

	if errorHandler.log == nil {
		errorHandler.log = &log
	}
	onceSetOtel.Do(onceBodySetOtel)

	return tp
}

func ChiTracerMiddleware(tr trace.Tracer, instance string, l logr.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			routePath := chi.RouteContext(r.Context()).RoutePath
			if routePath == "" {
				if r.URL.RawPath != "" {
					routePath = r.URL.RawPath
				} else {
					routePath = r.URL.Path
				}
			}

			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			span := trace.SpanFromContext(ctx)
			clientCommand := ""
			if span.SpanContext().IsValid() {
				spanValues, spanValuesErr := span.SpanContext().MarshalJSON()
				l.WithValues(
					"span", spanValues,
					"spanErr", spanValuesErr,
					"bag", baggage.FromContext(ctx).String(),
				).Info("IN Span")
				clientCommand = span.SpanContext().TraceState().Get(StateKeyClientCommand)
			} else {
				command := r.Method + " " + r.URL.String()

				traceState := trace.TraceState{}
				traceState, err := traceState.Insert(StateKeyClientCommand, EncodeTracestateValue(command))
				if err != nil {
					l.Error(err, "unable to set command in state")
				}
				ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
					TraceState: traceState,
				}))
				bag, err := NewBaggage(instance, command)
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

			ctx, span = tr.Start(ctx, "IN HTTP "+r.Method+" "+r.URL.String(),
				trace.WithAttributes(semconv.NetAttributesFromHTTPRequest("tcp", r)...),
				trace.WithAttributes(semconv.HTTPClientAttributesFromHTTPRequest(r)...),
				trace.WithAttributes(semconv.HTTPServerAttributesFromHTTPRequest(instance, routePath, r)...),
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String(StateKeyClientCommand, clientCommand),
					attribute.String(SpanKeyComponent, SpanKeyComponentValue),
				),
			)

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

func JaegerProvider(jUrl string) (sdktrace.SpanExporter, error) {
	if jUrl == "" || jUrl == "-" {
		return nil, nil
	}

	return jaeger.New(jaeger.WithCollectorEndpoint(
		jaeger.WithEndpoint(jUrl),
	))
}

func OtlpProvider(oUrl string) (sdktrace.SpanExporter, error) {
	if oUrl == "" || oUrl == "-" {
		return nil, nil
	}

	otlpUrl, err := url.ParseRequestURI(oUrl)
	if err != nil {
		return nil, err
	}

	return otlptracehttp.New(context.Background(), // otlptracehttp.client.Start does nothing in a HTTP client
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithEndpoint(otlpUrl.Host),
		otlptracehttp.WithURLPath(otlpUrl.Path),
	)
}

func NewBaggage(instance, command string) (baggage.Baggage, error) {
	return baggage.Parse(strings.Join([]string{ //nolint:gocritic // strings.Join is better
		"baggID=" + strconv.Itoa(os.Getpid()),
		"baggCommand=" + encodeBaggageValue(command),
	}, ","))
}

var invalidBaggageValueRe = regexp.MustCompile(`[^\x21\x23-\x2b\x2d-\x3a\x3c-\x5B\x5D-\x7e]`)

func encodeBaggageValue(value string) string {
	return invalidBaggageValueRe.ReplaceAllString(value, "_")
}

var invalidTracestateValueRe = regexp.MustCompile(`[^\x20-\x2b\x2d-\x3c\x3e-\x7e]`)

func EncodeTracestateValue(value string) string {
	return invalidTracestateValueRe.ReplaceAllString(strings.TrimSpace(value), "_")
}

func Version() string {
	return "0.0.1"
}

// SemVersion is the semantic version to be supplied to tracer/meter creation.
func SemVersion() string {
	return "semver:" + Version()
}

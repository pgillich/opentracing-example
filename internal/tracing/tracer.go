package tracing

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"github.com/pgillich/opentracing-example/internal/buildinfo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.11.0"
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

const StateKeyClientCommand = "client_command"

func InitTracer(exporter sdktrace.SpanExporter, sampler sdktrace.Sampler, service string, instance string, command string, log logr.Logger) *sdktrace.TracerProvider {
	// For the demonstration, use sdktrace.AlwaysSample sampler to sample all traces.
	// In a production application, use sdktrace.ProbabilitySampler with a desired probability.
	// semconv keys are defined in https://github.com/open-telemetry/opentelemetry-specification/tree/main/semantic_conventions/trace
	attrs := []attribute.KeyValue{
		semconv.ServiceNamespaceKey.String("demo"),
		semconv.ServiceNameKey.String(service),
		semconv.ServiceInstanceIDKey.String(instance),
		semconv.ServiceVersionKey.String(buildinfo.Version),
		attribute.Int("attrID", os.Getpid()),
	}
	if command != "" {
		attrs = append(attrs, attribute.String(StateKeyClientCommand, command))
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sampler),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, attrs...)),
	)

	if errorHandler.log == nil {
		errorHandler.log = &log
	}
	onceSetOtel.Do(onceBodySetOtel)

	return tp
}

func JaegerProvider(url string) (sdktrace.SpanExporter, error) {
	return jaeger.New(jaeger.WithCollectorEndpoint(
		jaeger.WithEndpoint(url),
	))
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

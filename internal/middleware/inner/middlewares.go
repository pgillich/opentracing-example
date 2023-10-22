// Goroutine middlewares
package inner

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"emperror.dev/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	metric_api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/semaphore"

	"github.com/pgillich/opentracing-example/internal/logger"
)

const (
	// MetrAttrErr is the metric attribute for error
	MetrAttrErr = "error"
)

var (
	// ErrTypeCast is an error for type assertion from interface
	ErrTypeCast = errors.NewPlain("unable to cast interface to type")
)

// SemAcquire is a middleware to acquire semaphore
func SemAcquire(sem *semaphore.Weighted) InternalMiddleware {
	return func(next InternalMiddlewareFn) InternalMiddlewareFn {
		return func(ctx context.Context) (interface{}, error) {
			if err := sem.Acquire(ctx, 1); err != nil {
				err = fmt.Errorf("cannot acquire semaphore: %w", err)

				return nil, err
			}
			defer func() {
				select {
				case <-ctx.Done():
				default:
					sem.Release(1)
				}
			}()

			return next(ctx)
		}
	}
}

// Span is a middleware to start/end a new span, using from context.
// Sets "traceID", "spanParentID" and "spanID" log values.
func Span(tr trace.Tracer, spanName string) InternalMiddleware {
	return func(next InternalMiddlewareFn) InternalMiddlewareFn {
		return func(ctx context.Context) (interface{}, error) {
			//spanParent := trace.SpanFromContext(ctx).SpanContext()
			spanKind := trace.SpanKindInternal
			ctx, spanChild := tr.Start(ctx, spanName,
				trace.WithSpanKind(spanKind),
			)
			ctx, _ = logger.FromContext(ctx,
				/*
					"traceID", spanParent.TraceID().String(),
					"spanParentID", spanParent.SpanID().String(),
				*/
				"spanID", spanChild.SpanContext().SpanID().String(),
			)

			defer spanChild.End()

			return next(ctx)
		}
	}
}

// TryCatch is a middleware for catching Go panic and propagating it as an error
func TryCatch() InternalMiddleware {
	return func(next InternalMiddlewareFn) InternalMiddlewareFn {
		return func(ctx context.Context) (interface{}, error) {
			var retVal interface{}
			var err error
			if errTryCatch := tryCatch(func() {
				retVal, err = next(ctx)
			})(); errTryCatch != nil {
				err = errTryCatch
			}

			return retVal, err
		}
	}
}

// ErrPanic is an error for captured panic
var ErrPanic = errors.NewPlain("captured panic")

// tryCatch captures a Go panic and returns as an error
func tryCatch(f func()) func() error {
	return func() (err error) {
		defer func() {
			if panicInfo := recover(); panicInfo != nil {
				err = fmt.Errorf("%w: %v, %s", ErrPanic, panicInfo, string(debug.Stack()))

				return
			}
		}()

		f() // calling the decorated function

		return err
	}
}

// Logger is a middleware for logging begin and end messages.
// A new logger with values is added to the context.
func Logger(values map[string]string, beginLevel slog.Level, endLevel slog.Level) InternalMiddleware {
	logValues := make([]any, 0, len(values)*2)
	for k, v := range values {
		logValues = append(logValues, k, v)
	}

	return func(next InternalMiddlewareFn) InternalMiddlewareFn {
		return func(ctx context.Context) (interface{}, error) {
			var log *slog.Logger
			var err error
			ctx, log = logger.FromContext(ctx, logValues...)
			log.Log(ctx, beginLevel, "GO_BEGIN")
			beginTS := time.Now()
			defer func() {
				elapsedSec := time.Since(beginTS).Seconds()
				args := []any{"duration", fmt.Sprintf("%.3f", elapsedSec)}
				if err != nil {
					args = append(args, logger.KeyError, err)
				}
				log.With(args...).Log(ctx, endLevel, "GO_END")
			}()

			retVal, err := next(ctx)

			return retVal, err
		}
	}
}

/*
Metrics is a middleware to make count and duration report

	Prometheus-specific implementation:
	The "_total" suffix is appended to the counter name, defined in "counterSuffix", see:
	https://github.com/open-telemetry/opentelemetry-go/blob/main/exporters/prometheus/exporter.go#L100
	The unit "s" is appended as "_seconds" to the metric name (injected before the "_total" suffix),
	defined in "unitSuffixes", see
	https://github.com/open-telemetry/opentelemetry-go/blob/main/exporters/prometheus/exporter.go#L343
*/
func Metrics(ctx context.Context, meter metric_api.Meter, name string,
	description string, attributes map[string]string, errFormatter ErrFormatter,
) InternalMiddleware {
	_, log := logger.FromContext(ctx)
	baseAttrs := make([]attribute.KeyValue, 0, len(attributes))
	for aKey, aVal := range attributes {
		baseAttrs = append(baseAttrs, attribute.Key(aKey).String(aVal))
	}
	attempted, err := regInt64Counter.GetInstrument(name, metric_api.WithDescription(description))
	if err != nil {
		log.Error("unable to instantiate counter", logger.KeyError, err, "metricName", name)
		panic(err)
	}
	durationSum, err := regFloat64Counter.GetInstrument(name+"_duration", metric_api.WithDescription(description+", duration sum"), metric_api.WithUnit("s"))
	if err != nil {
		log.Error("unable to instantiate time counter", logger.KeyError, err, "metricName", name)
		panic(err)
	}

	return func(next InternalMiddlewareFn) InternalMiddlewareFn {
		return func(ctx context.Context) (interface{}, error) {
			beginTS := time.Now()

			retVal, err := next(ctx)

			elapsedSec := time.Since(beginTS).Seconds()
			attrs := make([]attribute.KeyValue, len(baseAttrs), len(baseAttrs)+1)
			copy(attrs, baseAttrs)
			opt := metric_api.WithAttributes(append(attrs, attribute.Key(MetrAttrErr).String(errFormatter(err)))...)
			attempted.Add(ctx, 1, opt)
			durationSum.Add(ctx, elapsedSec, opt)

			return retVal, err
		}
	}
}

// InstrumentReg stores the already registered instruments
//
//nolint:structcheck // generics
type InstrumentReg[T any, O any] struct {
	instruments   map[string]T
	mu            sync.Mutex
	newInstrument func(name string, options ...O) (T, error)
}

// GetInstrument registers a new instrument, otherwise returns the already created.
func (r *InstrumentReg[T, O]) GetInstrument(name string, options ...O) (T, error) {
	var err error
	r.mu.Lock()
	defer r.mu.Unlock()
	instrument, has := r.instruments[name]
	if !has {
		instrument, err = r.newInstrument(name, options...)
		if err != nil {
			return instrument, fmt.Errorf("unable to register metric %T %s: %w", r, name, err)
		}
		r.instruments[name] = instrument
	}

	return instrument, nil
}

var (
	// meter is the default meter
	meter metric_api.Meter //nolint:gochecknoglobals // private
	// meterOnce is used to init meter
	meterOnce sync.Once //nolint:gochecknoglobals // private
	// regInt64Counter stores Int64Counters
	regInt64Counter *InstrumentReg[metric_api.Int64Counter, metric_api.Int64CounterOption] //nolint:gochecknoglobals // private
	// regFloat64Counter stores Float64Counters
	regFloat64Counter *InstrumentReg[metric_api.Float64Counter, metric_api.Float64CounterOption] //nolint:gochecknoglobals // private
)

// GetMeter returns the default meter
// Inits meter and InstrumentRegs (if needed)
func GetMeter(log *slog.Logger) metric_api.Meter {
	meterOnce.Do(func() {
		exporter, err := prometheus.New()
		if err != nil {
			log.Error("unable to instantiate prometheus exporter", logger.KeyError, err)
			panic(err)
		}
		provider := metric.NewMeterProvider(metric.WithReader(exporter))
		meter = provider.Meter("github.com/pgillich/opentracing-example/internal/middleware", metric_api.WithInstrumentationVersion("0.1"))

		regInt64Counter = &InstrumentReg[metric_api.Int64Counter, metric_api.Int64CounterOption]{
			instruments:   map[string]metric_api.Int64Counter{},
			newInstrument: meter.Int64Counter,
		}
		regFloat64Counter = &InstrumentReg[metric_api.Float64Counter, metric_api.Float64CounterOption]{
			instruments:   map[string]metric_api.Float64Counter{},
			newInstrument: meter.Float64Counter,
		}
	})

	return meter
}

// ErrFormatter is a func type to format metric error attribute
type ErrFormatter func(error) string

// NoErr always returns "". Can be used to skip any error stats in the metrics
func NoErr(error) string {
	return ""
}

// FullErr returns the full error text.
// Be careful about the cardinality, if the error text has dynamic part(s) (see: Prometheus label)
func FullErr(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}

// FirstErr returns the first part of error text before ':'
func FirstErr(err error) string {
	if err == nil {
		return ""
	}

	return strings.SplitN(err.Error(), ":", 2)[0]
}

package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"go.opentelemetry.io/otel/exporters/prometheus"
	metric_api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"

	"github.com/pgillich/opentracing-example/internal/logger"
)

func Int64CounterGetInstrument(name string, options ...metric_api.Int64CounterOption) (metric_api.Int64Counter, error) {
	return regInt64Counter.GetInstrument(name, options...)
}

func Float64CounterGetInstrument(name string, options ...metric_api.Float64CounterOption) (metric_api.Float64Counter, error) {
	return regFloat64Counter.GetInstrument(name, options...)
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

// GetMeter returns the default meter.
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

func GetHost(r *http.Request) string {
	if r.Host != "" {
		return r.Host
	}

	return r.URL.Host
}

package internal

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/model"
	"github.com/pgillich/opentracing-example/internal/tracing"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.11.0"
	"go.opentelemetry.io/otel/trace"
)

type ClientConfig struct {
	Server    string
	Instance  string
	Command   string
	JaegerURL string
}

type Client struct {
	config   ClientConfig
	log      logr.Logger
	shutdown <-chan struct{}
}

func NewClientService(ctx context.Context, cfg interface{}, log logr.Logger) model.Service {
	if config, is := cfg.(*ClientConfig); !is {
		log.Error(logger.ErrInvalidConfig, "config type")
		panic(logger.ErrInvalidConfig)
	} else {
		return &Client{
			config:   *config,
			log:      log,
			shutdown: ctx.Done(),
		}
	}
}

func (c *Client) Run(args []string) error {
	c.log.WithValues("config", c.config).Info("Client start")

	traceExporter, err := tracing.JaegerProvider(c.config.JaegerURL)
	if err != nil {
		return err
	}
	if c.config.Instance == "-" {
		c.config.Instance, _ = os.Hostname() //nolint:errcheck // not important
	}
	tp := tracing.InitTracer(traceExporter, sdktrace.AlwaysSample(),
		"client.opentracing-example", c.config.Instance, c.config.Command, c.log,
	)
	defer func() {
		//nolint:govet // local err
		if err := tp.Shutdown(context.Background()); err != nil {
			c.log.Error(err, "Error shutting down tracer provider")
		}
	}()
	httpClient := &http.Client{Transport: otelhttp.NewTransport(
		http.DefaultTransport,
		otelhttp.WithPropagators(otel.GetTextMapPropagator()),
		otelhttp.WithSpanOptions(trace.WithAttributes(
			attribute.String(tracing.SpanKeyComponent, tracing.SpanKeyComponentValue),
		)),
	)}
	tr := tp.Tracer("github.com/pgillich/opentracing-example/client", trace.WithInstrumentationVersion(tracing.SemVersion()))

	ctx := context.Background()

	traceState := trace.TraceState{}
	traceState, err = traceState.Insert(tracing.StateKeyClientCommand, tracing.EncodeTracestateValue(c.config.Command))
	if err != nil {
		c.log.Error(err, "unable to set command in state")
	} else {
		ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
			TraceState: traceState,
		}))
	}

	bag, err := tracing.NewBaggage(c.config.Instance, c.config.Command)
	if err != nil {
		return err
	}
	ctx = baggage.ContextWithBaggage(ctx, bag)

	// 	otel.SetTracerProvider(tp)

	ctx, span := tr.Start(ctx, "Run "+c.config.Command,
		trace.WithAttributes(semconv.PeerServiceKey.String("ExampleClientService")),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer func() {
		spanText, _ := span.SpanContext().MarshalJSON() //nolint:errcheck // not important
		c.log.WithValues(
			"service", "client",
			"span", string(spanText),
		).Info("Span END")
		span.End()
		tp.ForceFlush(context.Background()) //nolint:errcheck,gosec // not important
	}()

	return c.run(ctx, httpClient, strings.Join(args, " "))
}

func (c *Client) run(ctx context.Context, httpClient *http.Client, reqBody string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+c.config.Server+"/proxy", strings.NewReader(reqBody))
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.Body != nil {
		defer resp.Body.Close() //nolint:errcheck,gosec // not needed
	}
	c.log.Info("Client resp", "body", string(body))

	return nil
}

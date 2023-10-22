package internal

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/pgillich/opentracing-example/internal/logger"
	mw_client "github.com/pgillich/opentracing-example/internal/middleware/client"
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
	log      *slog.Logger
	shutdown <-chan struct{}
}

func NewClientService(ctx context.Context, cfg interface{}) model.Service {
	_, log := logger.FromContext(ctx)
	if config, is := cfg.(*ClientConfig); !is {
		log.Error("config type", logger.KeyError, logger.ErrInvalidConfig)
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
	c.log.With("config", c.config).Info("Client start")

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
			c.log.Error("Error shutting down tracer provider", logger.KeyError, err)
		}
	}()
	httpClient := &http.Client{Transport: otelhttp.NewTransport(
		mw_client.NewTransport(http.DefaultTransport, slog.LevelInfo, slog.LevelInfo, c.log),
		otelhttp.WithPropagators(otel.GetTextMapPropagator()),
		otelhttp.WithSpanOptions(trace.WithAttributes(
			attribute.String(tracing.SpanKeyComponent, tracing.SpanKeyComponentValue),
		)),
	)}
	tr := tp.Tracer("github.com/pgillich/opentracing-example/client", trace.WithInstrumentationVersion(tracing.SemVersion()))

	log := c.log
	ctx := logger.NewContext(context.Background(), log)

	traceState := trace.TraceState{}
	traceState, err = traceState.Insert(tracing.StateKeyClientCommand, tracing.EncodeTracestateValue(c.config.Command))
	if err != nil {
		log.Error("unable to set command in state", logger.KeyError, err)
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
	spanKind := trace.SpanKindClient
	ctx, span := tr.Start(ctx, "Run "+c.config.Command,
		trace.WithAttributes(semconv.PeerServiceKey.String("ExampleClientService")),
		trace.WithSpanKind(spanKind),
	)
	ctx, log = logger.FromContext(ctx, "traceID", span.SpanContext().TraceID().String(), "spanID", span.SpanContext().SpanID().String())
	log.With("spanKind", spanKind).Info("SPAN_START")
	defer func() {
		spanText, _ := span.SpanContext().MarshalJSON() //nolint:errcheck // not important
		log.With(
			"span", string(spanText),
		).Info("SPAN_END")
		span.End()
		tp.ForceFlush(context.Background()) //nolint:errcheck,gosec // not important
	}()

	return c.run(ctx, httpClient, strings.Join(args, " "))
}

func (c *Client) run(ctx context.Context, httpClient *http.Client, reqBody string) error {
	_, log := logger.FromContext(ctx)
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
		defer resp.Body.Close() //nolint:errcheck // not needed
	}
	log.Info("Client resp", "body", string(body))

	return nil
}

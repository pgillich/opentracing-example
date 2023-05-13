package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	chi_middleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/pgillich/opentracing-example/internal/htmlmsg"
	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/model"
	"github.com/pgillich/opentracing-example/internal/tracing"
)

type FrontendConfig struct {
	ListenAddr string
	Instance   string
	Command    string
	JaegerURL  string
	NatsURL    string
}

func (c *FrontendConfig) SetListenAddr(addr string) {
	c.ListenAddr = addr
}

func (c *FrontendConfig) SetInstance(instance string) {
	c.Instance = instance
}

func (c *FrontendConfig) SetJaegerURL(url string) {
	c.JaegerURL = url
}

func (c *FrontendConfig) SetNatsURL(url string) {
	c.NatsURL = url
}

func (c *FrontendConfig) SetCommand(command string) {
	c.Command = command
}

func (c *FrontendConfig) GetOptions() []string {
	return []string{"--listenaddr", c.ListenAddr, "--instance", c.Instance}
}

type Frontend struct {
	config       FrontendConfig
	serverRunner model.ServerRunner
	log          logr.Logger
	shutdown     <-chan struct{}
}

func NewFrontendService(ctx context.Context, cfg interface{}, log logr.Logger) model.Service {
	if config, is := cfg.(*FrontendConfig); !is {
		log.Error(logger.ErrInvalidConfig, "config type")
		panic(logger.ErrInvalidConfig)
	} else if serverRunner, is := ctx.Value(model.CtxKeyServerRunner).(model.ServerRunner); !is {
		log.Error(ErrInvalidServerRunner, "server runner config")
		panic(ErrInvalidServerRunner)
	} else {
		return &Frontend{
			config:       *config,
			serverRunner: serverRunner,
			log:          log,
			shutdown:     ctx.Done(),
		}
	}
}

func (s *Frontend) Run(args []string) error {
	s.log = s.log.WithValues("args", args)
	var h http.Handler
	if s.config.Instance == "-" {
		s.config.Instance, _ = os.Hostname() //nolint:errcheck // not important
	}
	s.log.WithValues("config", s.config).Info("Frontend start")

	traceExporter, err := tracing.JaegerProvider(s.config.JaegerURL)
	if err != nil {
		return err
	}
	tp := tracing.InitTracer(traceExporter, sdktrace.AlwaysSample(),
		"frontend.opentracing-example", s.config.Instance, "", s.log,
	)
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			s.log.Error(err, "Error shutting down tracer provider")
		}
	}()
	tr := tp.Tracer(
		"github.com/pgillich/opentracing-example/frontend",
		trace.WithInstrumentationVersion(tracing.SemVersion()),
	)

	// CHI

	r := chi.NewRouter()
	r.Use(chi_middleware.RequestLogger(&logger.ChiLogr{Logger: s.log}))
	r.Use(chi_middleware.Recoverer)
	r.Use(tracing.ChiTracerMiddleware(tr, s.config.Instance, s.log))

	r.Get("/proxy", func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			s.writeErr(w, http.StatusInternalServerError, errors.New("empty response"))

			return
		}
		defer r.Body.Close() //nolint:errcheck,gosec // not important
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.writeErr(w, http.StatusInternalServerError, err)

			return
		}

		ctx := r.Context()
		span := trace.SpanFromContext(ctx)
		defer func() {
			spanText, _ := span.SpanContext().MarshalJSON() //nolint:errcheck // not important
			s.log.WithValues(
				"service", "frontend",
				"span", string(spanText),
			).Info("Span END")
			span.End()
			tp.ForceFlush(context.Background()) //nolint:errcheck,gosec // not important
		}()

		bodies := []string{}
		for _, beURL := range strings.Split(string(body), " ") {
			body, err := s.sendToBackend(ctx, beURL) //nolint:govet // err shadow
			if err != nil {
				s.writeErr(w, http.StatusInternalServerError, err)

				return
			}
			bodies = append(bodies, body)
		}

		if _, err = w.Write([]byte(strings.Join(bodies, " "))); err != nil {
			s.log.Error(err, "unable to write response")
		}
	})
	h = r

	s.serverRunner(h, s.shutdown, s.config.ListenAddr, s.log)
	s.log.Info("Frontend exit")

	return nil
}

func (s *Frontend) sendToBackend(ctx context.Context, beURL string) (string, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, beURL, http.NoBody,
	)
	if err != nil {
		return "", fmt.Errorf("unable to send request: %w", err)
	}
	natsClient, err := htmlmsg.NewNatsReqRespClient(s.config.NatsURL, s.log)
	if err != nil {
		return "", fmt.Errorf("unable to connect to nats: %w", err)
	}
	httpClient := &http.Client{
		Transport: otelhttp.NewTransport(
			&htmlmsg.HttpToMsg{
				DefaultTransport: http.DefaultTransport,
				Client:           natsClient,
			},
			otelhttp.WithPropagators(otel.GetTextMapPropagator()),
			otelhttp.WithSpanOptions(trace.WithAttributes(
				attribute.String(tracing.SpanKeyComponent, tracing.SpanKeyComponentValue),
			)),
		),
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("unable to send request: %w", err)
	}
	if resp.Body == nil {
		return "", errors.New("empty body")
	}
	beBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read response: %w", err)
	}
	resp.Body.Close() //nolint:errcheck,gosec // not important

	s.log.WithValues("Header", resp.Header, "Body", string(beBody), "StatusCode", resp.StatusCode, "Status", resp.Status).Info("sendToBackend RESPONSE")

	return string(beBody), nil
}

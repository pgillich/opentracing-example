package internal

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"emperror.dev/errors"
	"github.com/go-chi/chi/v5"
	chi_middleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/semaphore"

	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/middleware"
	mw_client "github.com/pgillich/opentracing-example/internal/middleware/client"
	mw_inner "github.com/pgillich/opentracing-example/internal/middleware/inner"
	mw_server "github.com/pgillich/opentracing-example/internal/middleware/server"
	"github.com/pgillich/opentracing-example/internal/model"
	"github.com/pgillich/opentracing-example/internal/tracing"
)

type FrontendConfig struct {
	ListenAddr string
	Instance   string
	Command    string
	JaegerURL  string
	MaxReq     int64
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

func (c *FrontendConfig) SetCommand(command string) {
	c.Command = command
}

func (c *FrontendConfig) GetOptions() []string {
	return []string{"--listenaddr", c.ListenAddr, "--instance", c.Instance}
}

type Frontend struct {
	config       FrontendConfig
	serverRunner model.ServerRunner
	log          *slog.Logger
	shutdown     <-chan struct{}
}

func NewFrontendService(ctx context.Context, cfg interface{}) model.Service {
	_, log := logger.FromContext(ctx)
	if config, is := cfg.(*FrontendConfig); !is {
		log.Error("config type", logger.KeyError, logger.ErrInvalidConfig)
		panic(logger.ErrInvalidConfig)
	} else if serverRunner, is := ctx.Value(model.CtxKeyServerRunner).(model.ServerRunner); !is {
		log.Error("server runner config", logger.KeyError, ErrInvalidServerRunner)
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
	var h http.Handler
	if s.config.Instance == "-" {
		s.config.Instance, _ = os.Hostname() //nolint:errcheck // not important
	}
	s.log = s.log.With("instance", s.config.Instance)
	s.log.With(
		logger.KeyCmd, s.config.Command,
		"args", args,
		"config", s.config,
	).Info("Frontend start")

	traceExporter, err := tracing.JaegerProvider(s.config.JaegerURL)
	if err != nil {
		return err
	}
	tp := tracing.InitTracer(traceExporter, sdktrace.AlwaysSample(),
		"frontend.opentracing-example", s.config.Instance, "", s.log,
	)
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			s.log.Error("Error shutting down tracer provider", logger.KeyError, err)
		}
	}()
	tr := tp.Tracer(
		"github.com/pgillich/opentracing-example/frontend",
		trace.WithInstrumentationVersion(tracing.SemVersion()),
	)

	// CHI

	r := chi.NewRouter()
	r.Use(chi_middleware.Recoverer)
	r.Use(mw_server.ChiLoggerBaseMiddleware(s.log))
	r.Use(mw_server.ChiTracerMiddleware(tr, s.config.Instance))
	r.Use(mw_server.ChiLoggerMiddleware(slog.LevelInfo, slog.LevelInfo))
	r.Use(mw_server.ChiMetricMiddleware(middleware.GetMeter(s.log),
		"http_in", "HTTP in response", map[string]string{
			"service": "frontend",
		}, s.log,
	))

	r.Handle("/metrics", promhttp.Handler())

	r.Get("/proxy", s.proxyHandler(tp, tr))
	h = r

	s.serverRunner(h, s.shutdown, s.config.ListenAddr, s.log)
	s.log.Info("Frontend exit")

	return nil
}

func (s *Frontend) proxyHandler(tp *sdktrace.TracerProvider, tr trace.Tracer) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, log := logger.FromContext(r.Context())
		if r.Body == nil {
			s.writeErr(w, http.StatusInternalServerError, errors.New("empty response"))

			return
		}
		defer r.Body.Close() //nolint:errcheck // not important
		inBody, err := io.ReadAll(r.Body)
		if err != nil {
			s.writeErr(w, http.StatusInternalServerError, err)

			return
		}

		inBodies := strings.Split(string(inBody), " ")

		bodies := []string{}
		errs := []error{}
		meter := middleware.GetMeter(log)
		sem := semaphore.NewWeighted(s.config.MaxReq)
		type bodyRespT struct {
			Body string
			Err  error
		}
		bodyRespCh := make(chan bodyRespT)
		for be, beURL := range inBodies {
			go func(be int, beURL string) {
				bodyResp := bodyRespT{}
				var retVal interface{}
				jobID := fmt.Sprintf("GET %s #%d", beURL, be)
				jobName := fmt.Sprintf("GET %s", beURL)
				retVal, bodyResp.Err = mw_inner.InternalMiddlewareChain(
					mw_inner.TryCatch(),
					mw_inner.SemAcquire(sem),
					mw_inner.Span(tr, jobID),
					mw_inner.Logger(map[string]string{
						"job_type": "example",
						"job_name": jobName,
						"job_id":   jobID,
					}, slog.LevelInfo, slog.LevelDebug),
					mw_inner.Metrics(ctx, meter, "example_job", "Example job", map[string]string{
						"job_type": "example",
						"job_name": jobName,
					}, middleware.FirstErr),
					mw_inner.TryCatch(),
				)(func(ctx context.Context) (interface{}, error) {
					return s.sendToBackend(ctx, beURL)
				})(ctx)
				if retVal != nil {
					var is bool
					bodyResp.Body, is = retVal.(string)
					if !is {
						bodyResp.Err = fmt.Errorf("%w: %t", mw_inner.ErrTypeCast, retVal)
					}
				}

				bodyRespCh <- bodyResp
			}(be, beURL)
		}
		for range inBodies {
			resp := <-bodyRespCh
			if resp.Err != nil {
				errs = append(errs, resp.Err)
			} else {
				bodies = append(bodies, resp.Body)
			}
		}
		if len(errs) > 0 {
			s.writeErr(w, http.StatusInternalServerError, errors.Combine(errs...))

			return
		}

		if _, err = w.Write([]byte(strings.Join(bodies, " "))); err != nil {
			log.Error("unable to write response", logger.KeyError, err)
		}
	}
}

func (s *Frontend) sendToBackend(ctx context.Context, beURL string) (string, error) {
	_, log := logger.FromContext(ctx)
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, beURL, http.NoBody,
	)
	if err != nil {
		return "", errors.Wrap(err, "unable to send request")
	}
	httpClient := &http.Client{Transport: otelhttp.NewTransport(
		mw_client.NewMetricTransport(
			mw_client.NewLogTransport(
				http.DefaultTransport,
				slog.LevelInfo,
				slog.LevelInfo,
			),
			middleware.GetMeter(log),
			"http_out", "HTTP out response", map[string]string{
				"service":        "frontend",
				"target_service": "backend",
			},
			middleware.FirstErr,
		),
		otelhttp.WithPropagators(otel.GetTextMapPropagator()),
		otelhttp.WithSpanOptions(trace.WithAttributes(
			attribute.String(tracing.SpanKeyComponent, tracing.SpanKeyComponentValue),
		)),
	)}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "unable to send request")
	}
	if resp.Body == nil {
		return "", errors.New("empty body")
	}
	beBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "unable to read response")
	}
	resp.Body.Close() //nolint:errcheck,gosec // not important

	return string(beBody), nil
}

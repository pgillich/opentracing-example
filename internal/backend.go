package internal

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chi_middleware "github.com/go-chi/chi/v5/middleware"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/pgillich/opentracing-example/internal/logger"
	mw_server "github.com/pgillich/opentracing-example/internal/middleware/server"
	"github.com/pgillich/opentracing-example/internal/model"
	"github.com/pgillich/opentracing-example/internal/tracing"
)

type BackendConfig struct {
	ListenAddr string
	Instance   string
	Command    string
	JaegerURL  string

	Response string
}

func (c *BackendConfig) SetListenAddr(addr string) {
	c.ListenAddr = addr
}

func (c *BackendConfig) SetInstance(instance string) {
	c.Instance = instance
}

func (c *BackendConfig) SetJaegerURL(url string) {
	c.JaegerURL = url
}

func (c *BackendConfig) SetCommand(command string) {
	c.Command = command
}

func (c *BackendConfig) GetOptions() []string {
	return []string{"--listenaddr", c.ListenAddr, "--instance", c.Instance}
}

type Backend struct {
	config       BackendConfig
	serverRunner model.ServerRunner
	log          *slog.Logger
	shutdown     <-chan struct{}
}

func NewBackendService(ctx context.Context, cfg interface{}) model.Service {
	_, log := logger.FromContext(ctx)
	if config, is := cfg.(*BackendConfig); !is {
		log.Error("config type", logger.KeyError, logger.ErrInvalidConfig)
		panic(logger.ErrInvalidConfig)
	} else if serverRunner, is := ctx.Value(model.CtxKeyServerRunner).(model.ServerRunner); !is {
		log.Error("server runner config", logger.KeyError, ErrInvalidServerRunner)
		panic(ErrInvalidServerRunner)
	} else {
		return &Backend{
			config:       *config,
			serverRunner: serverRunner,
			log:          log,
			shutdown:     ctx.Done(),
		}
	}
}

func (s *Backend) Run(args []string) error {
	var h http.Handler
	if len(args) > 0 {
		s.config.Response = args[0]
	}
	hostname, _ := os.Hostname() //nolint:errcheck // not important
	if s.config.Instance == "-" {
		s.config.Instance = hostname
	}
	s.log = s.log.With("instance", s.config.Instance)
	s.log.With(
		logger.KeyCmd, s.config.Command,
		"args", args,
		"config", s.config,
	).Info("Backend start")

	traceExporter, err := tracing.JaegerProvider(s.config.JaegerURL)
	if err != nil {
		return err
	}
	tp := tracing.InitTracer(traceExporter, sdktrace.AlwaysSample(),
		"backend.opentracing-example", s.config.Instance, "", s.log,
	)
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			s.log.Error("Error shutting down tracer provider", logger.KeyError, err)
		}
	}()
	tr := tp.Tracer(
		"github.com/pgillich/opentracing-example/backend",
		trace.WithInstrumentationVersion(tracing.SemVersion()),
	)

	// CHI

	r := chi.NewRouter()
	r.Use(chi_middleware.Recoverer)
	r.Use(mw_server.ChiLoggerBaseMiddleware(s.log))
	r.Use(mw_server.ChiTracerMiddleware(tr, s.config.Instance, s.log))
	r.Use(mw_server.ChiLoggerMiddleware(slog.LevelInfo, slog.LevelInfo, s.log))

	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		_, log := logger.FromContext(r.Context())

		if _, err := w.Write([]byte(s.config.Response + hostname)); err != nil {
			log.Error("unable to send response", logger.KeyError, err)
		}
	})
	h = r

	s.serverRunner(h, s.shutdown, s.config.ListenAddr, s.log)
	s.log.Info("Backend started")

	return nil
}

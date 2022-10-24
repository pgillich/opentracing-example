package internal

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	chi_middleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/pgillich/opentracing-example/internal/logger"
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
	log          logr.Logger
	shutdown     <-chan struct{}
}

func NewBackendService(ctx context.Context, cfg interface{}, log logr.Logger) model.Service {
	if config, is := cfg.(*BackendConfig); !is {
		log.Error(logger.ErrInvalidConfig, "config type")
		panic(logger.ErrInvalidConfig)
	} else if serverRunner, is := ctx.Value(model.CtxKeyServerRunner).(model.ServerRunner); !is {
		log.Error(ErrInvalidServerRunner, "server runner config")
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
	s.log = s.log.WithValues("args", args)
	var h http.Handler
	if len(args) > 0 {
		s.config.Response = args[0]
	}
	s.log.WithValues("config", s.config).Info("Backend start")

	traceExporter, err := tracing.JaegerProvider(s.config.JaegerURL)
	if err != nil {
		return err
	}
	tp := tracing.InitTracer(traceExporter, sdktrace.AlwaysSample(),
		s.config.Instance, s.config.Instance, "", s.log,
	)
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			s.log.Error(err, "Error shutting down tracer provider")
		}
	}()

	// CHI

	r := chi.NewRouter()
	r.Use(chi_middleware.RequestLogger(&logger.ChiLogr{Logger: s.log}))
	r.Use(chi_middleware.Recoverer)

	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		ctx, span := chiSpan(tp, "github.com/pgillich/opentracing-example/backend", "/ping", s.config.Instance, r, s.log)
		defer func() {
			spanText, _ := span.SpanContext().MarshalJSON() //nolint:errcheck // not important
			s.log.WithValues(
				"service", "backend",
				"span", string(spanText),
			).Info("Span END")
			span.End()
			tp.ForceFlush(context.Background()) //nolint:errcheck,gosec // not important
		}()
		_ = ctx

		if _, err := w.Write([]byte(s.config.Response)); err != nil {
			s.log.Error(err, "unable to send response")
		}
	})
	h = r

	s.serverRunner(h, s.shutdown, s.config.ListenAddr, s.log)
	s.log.Info("Backend started")

	return nil
}

/*
	// ECHO

	e := echo.New()
	e.Use(EchoLogr(s.log))
	e.Use(echo_middleware.Recover())
	e.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, s.config.Response) //nolint:wrapcheck // Echo
	})
	h = e
*/

/*
	// GIN

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(ginlogr.Ginlogr(s.log, time.RFC3339, false))
	router.Use(ginlogr.RecoveryWithLogr(s.log, time.RFC3339, false, true))
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, s.config.Response)
	})
	h = router.Handler()
*/

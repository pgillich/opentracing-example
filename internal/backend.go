package internal

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	chi_middleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"

	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/model"
)

type BackendConfig struct {
	ListenAddr string

	Response string
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
	s.log.Info("Backend start")
	var h http.Handler

	// CHI

	r := chi.NewRouter()
	r.Use(chi_middleware.RequestLogger(&logger.ChiLogr{Logger: s.log}))
	r.Use(chi_middleware.Recoverer)

	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(s.config.Response)); err != nil {
			s.log.Error(err, "unable to send response")
		}
	})
	h = r

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

	s.serverRunner(h, s.shutdown, s.config.ListenAddr, s.log)
	s.log.Info("Backend started")

	return nil
}

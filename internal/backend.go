package internal

import (
	"context"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

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

	e := echo.New()
	e.Use(EchoLogr(s.log))
	e.Use(middleware.Recover())
	e.GET("/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, s.config.Response) //nolint:wrapcheck // Echo
	})
	s.serverRunner(e, s.shutdown, s.config.ListenAddr, s.log)
	/*
		gin.SetMode(gin.ReleaseMode)
		router := gin.New()
		router.Use(ginlogr.Ginlogr(s.log, time.RFC3339, false))
		router.Use(ginlogr.RecoveryWithLogr(s.log, time.RFC3339, false, true))
		router.GET("/ping", func(c *gin.Context) {
			c.String(http.StatusOK, s.config.Response)
		})
		s.serverRunner(router.Handler(), s.shutdown, s.config.ListenAddr, s.log)
	*/
	s.log.Info("Backend exit")

	return nil
}

package internal

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/alron/ginlogr"
	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/pgillich/opentracing-example/internal/logger"
	"github.com/pgillich/opentracing-example/internal/model"
)

type FrontendConfig struct {
	ListenAddr string
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
	s.log.Info("Frontend start")
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(ginlogr.Ginlogr(s.log, time.RFC3339, false))
	router.Use(ginlogr.RecoveryWithLogr(s.log, time.RFC3339, false, true))
	router.GET("/proxy", func(c *gin.Context) {
		if c.Request.Body == nil {
			c.String(http.StatusInternalServerError, "empty response")
			return //nolint:nlreturn // no problem
		}
		defer c.Request.Body.Close() //nolint:errcheck // not important
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return //nolint:nlreturn // no problem
		}
		beURL := string(body)

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, beURL, http.NoBody)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return //nolint:nlreturn // no problem
		}
		httpClient := &http.Client{}
		resp, err := httpClient.Do(req)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return //nolint:nlreturn // no problem
		}
		if resp.Body == nil {
			c.String(http.StatusInternalServerError, "empty response")
			return //nolint:nlreturn // no problem
		}
		defer resp.Body.Close() //nolint:errcheck // not important
		beBody, err := io.ReadAll(resp.Body)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return //nolint:nlreturn // no problem
		}

		c.String(http.StatusOK, string(beBody))
	})
	s.serverRunner(router.Handler(), s.shutdown, s.config.ListenAddr, s.log)
	s.log.Info("Frontend exit")

	return nil
}

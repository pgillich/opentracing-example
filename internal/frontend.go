package internal

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
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

	e := echo.New()
	e.Use(EchoLogr(s.log))
	e.Use(middleware.Recover())
	e.GET("/proxy", func(c echo.Context) error {
		if c.Request().Body == nil {
			return c.String(http.StatusInternalServerError, "empty response") //nolint:wrapcheck // Echo
		}
		defer c.Request().Body.Close() //nolint:errcheck // not important
		body, err := io.ReadAll(c.Request().Body)
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error()) //nolint:wrapcheck // Echo
		}
		bodies := []string{}
		for _, beURL := range strings.Split(string(body), " ") {
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, beURL, http.NoBody)
			if err != nil {
				return c.String(http.StatusInternalServerError, err.Error()) //nolint:wrapcheck // Echo
			}
			httpClient := &http.Client{}
			resp, err := httpClient.Do(req)
			if err != nil {
				return c.String(http.StatusInternalServerError, err.Error()) //nolint:wrapcheck // Echo
			}
			if resp.Body == nil {
				return c.String(http.StatusInternalServerError, "empty response") //nolint:wrapcheck // Echo
			}
			beBody, err := io.ReadAll(resp.Body)
			resp.Body.Close() //nolint:errcheck,gosec // not important
			if err != nil {
				return c.String(http.StatusInternalServerError, err.Error()) //nolint:wrapcheck // Echo
			}
			bodies = append(bodies, string(beBody))
		}

		return c.String(http.StatusOK, strings.Join(bodies, " ")) //nolint:wrapcheck // Echo
	})
	s.serverRunner(e, s.shutdown, s.config.ListenAddr, s.log)
	/*
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
			bodies := []string{}
			for _, beURL := range strings.Split(string(body), " ") {
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
				beBody, err := io.ReadAll(resp.Body)
				resp.Body.Close() //nolint:errcheck,gosec // not important
				if err != nil {
					c.String(http.StatusInternalServerError, err.Error())
					return //nolint:nlreturn // no problem
				}
				bodies = append(bodies, string(beBody))
			}

			c.String(http.StatusOK, strings.Join(bodies, " "))
		})
		s.serverRunner(router.Handler(), s.shutdown, s.config.ListenAddr, s.log)
	*/
	s.log.Info("Frontend exit")

	return nil
}

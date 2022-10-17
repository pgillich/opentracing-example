package internal

import (
	"context"
	"net/http"
	"time"

	"emperror.dev/errors"
	"github.com/go-logr/logr"
	"github.com/labstack/echo/v4"
)

var ErrInvalidServerRunner = errors.NewPlain("invalid server runner")

func RunServer(h http.Handler, shutdown <-chan struct{}, addr string, log logr.Logger) {
	server := &http.Server{ // nolint:gosec // not secure
		Handler: h,
		Addr:    addr,
	}

	go func() {
		<-shutdown
		if err := server.Shutdown(context.Background()); !errors.Is(err, http.ErrServerClosed) {
			log.Error(err, "Server shutdown error")
		}
	}()

	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Error(err, "Server exit error")
	} else {
		log.Info("Server exit")
	}
}

func EchoLogr(log logr.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) { //nolint:nonamedreturns // copypasted
			start := time.Now()
			path := c.Request().URL.Path
			query := c.Request().URL.RawQuery
			if err = next(c); err != nil {
				c.Error(err)
			}

			end := time.Now()
			latency := end.Sub(start) / time.Millisecond
			if err != nil {
				log.Error(err, "?", "latency", latency)
			} else {
				log.Info(path,
					"status", c.Response().Status,
					"method", c.Request().Method,
					"path", path,
					"query", query,
					"ip", c.RealIP(),
					"user-agent", c.Request().UserAgent(),
					"latency", latency,
				)
			}

			return
		}
	}
}
